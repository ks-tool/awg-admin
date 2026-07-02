/*
  Copyright © 2026 Alexey Shulutkov <github@shulutkov.ru>

  Licensed under the Apache License, Version 2.0 (the "License");
  you may not use this file except in compliance with the License.
  You may obtain a copy of the License at

  	http://www.apache.org/licenses/LICENSE-2.0

  Unless required by applicable law or agreed to in writing, software
  distributed under the License is distributed on an "AS IS" BASIS,
  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
  See the License for the specific language governing permissions and
  limitations under the License.
*/

// Package agent holds the awg-agent daemon's startup/run logic, shared by every
// agent binary regardless of which service.Backend it drives. The kernel build
// (cmd/awg-agent) uses the default netlink/amneziawg-dkms backend; a userspace
// build (cmd/awg-agent-userspace) calls service.SetBackend before Run to swap in
// an amneziawg-go backend. Everything below the backend — config, storage, the
// HTTP API, metrics, the storage watcher and graceful shutdown — is identical,
// so it lives here once instead of being duplicated per main.
package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ks-tool/awg-admin/agent/internal/api"
	"github.com/ks-tool/awg-admin/agent/internal/metrics"
	"github.com/ks-tool/awg-admin/agent/internal/service"
	"github.com/ks-tool/awg-admin/agent/mux/middleware"
	"github.com/ks-tool/awg-admin/agent/storage"
	"github.com/ks-tool/awg-admin/agent/storage/fs"

	"github.com/Jipok/wgctrl-go"
	"github.com/caarlos0/env/v11"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type config struct {
	LogLevel zerolog.Level `env:"LOG_LEVEL" envDefault:"info"`
	Addr     string        `env:"ADDR" envDefault:"127.0.0.1:8080"`
	DB       string        `env:"DB"`

	// TLS, when all three are set, makes the agent listen with mTLS instead
	// of plain HTTP — required when binding to a public ("white") IP instead
	// of going through an SSH tunnel. TLSClientCA is the CA that signed the
	// awg-admin client certificate; any request without a valid client cert
	// signed by it is rejected at the TLS handshake.
	TLSCert     string `env:"TLS_CERT"`
	TLSKey      string `env:"TLS_KEY"`
	TLSClientCA string `env:"TLS_CLIENT_CA"`

	// MetricsInterval controls how often CPU/RAM/load/network/peer stats are
	// sampled into the local metrics store (see internal/metrics), exposed
	// over GET /metrics.
	MetricsInterval time.Duration `env:"METRICS_INTERVAL" envDefault:"45s"`
}

// Run parses the agent's configuration from the environment and runs the daemon
// until it receives SIGINT/SIGTERM, then shuts down gracefully. version is the
// build-stamped version string (see each main's -ldflags). The link backend
// must already be selected (service.SetBackend) before calling Run; the default
// is the kernel/amneziawg-dkms backend.
func Run(version string) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	var cfg config
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: "AWG_AGENT_"}); err != nil {
		log.Fatal().Err(err).Send()
	}
	zerolog.SetGlobalLevel(cfg.LogLevel)
	fmt.Printf("log level set to '%s'\n", cfg.LogLevel)

	log.Info().Str("version", version).Msg("starting awg-agent")

	// Discover what this host supports once, up front (backend/interface kinds,
	// Docker, in-container, kernel module), log it as a startup diagnostic and
	// serve it unchanged over GET /info. The backend is already selected by main
	// (service.SetBackend) before Run is called.
	hostInfo := gatherHostInfo(version)
	log.Info().
		Str("backend", hostInfo.Backend).
		Bool("docker", hostInfo.Docker).
		Bool("inDocker", hostInfo.InDocker).
		Bool("kernelModule", hostInfo.KernelModule).
		Strs("interfaceKinds", hostInfo.InterfaceKinds).
		Msg("host capabilities")

	if len(cfg.DB) == 0 {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.DB = filepath.Join(home, ".awg-agent")
		}
		if len(cfg.DB) == 0 {
			log.Fatal().Msg("environment variable 'AWG_AGENT_DB' should not be empty")
		}
	}

	store, err := fs.New(cfg.DB)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize storage")
	}

	log.Info().Msg("Loading Wireguard interfaces")
	if err := loadInterfaces(store); err != nil {
		log.Fatal().Err(err).Msg("failed to load interfaces")
	}

	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	// Best-effort: if the watch can't start (e.g. inotify limits hit), the
	// agent still works — it just won't react to an interface's config
	// file being removed by hand until the next restart (see
	// DetectOrphans, called above via loadInterfaces).
	if err := service.WatchStorage(watchCtx, cfg.DB); err != nil {
		log.Warn().Err(err).Msg("failed to start storage watcher")
	}

	metricsAwg, err := wgctrl.New()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open wgctrl client for metrics")
	}
	defer func() { _ = metricsAwg.Close() }()

	collector := metrics.NewCollector(metricsAwg, store, cfg.MetricsInterval)
	// Persist metrics history under the agent's data dir so charts survive a
	// restart (the file has no .json extension, so the interface-config storage
	// and its watcher ignore it — see metrics.HistoryFilename).
	collector.SetPersistence(filepath.Join(cfg.DB, metrics.HistoryFilename))
	if err := collector.LoadHistory(); err != nil {
		log.Warn().Err(err).Msg("failed to load persisted metrics history")
	}
	metricsCtx, stopMetrics := context.WithCancel(context.Background())
	defer stopMetrics()
	metricsDone := make(chan struct{})
	go func() {
		collector.Run(metricsCtx)
		close(metricsDone)
	}()

	srv := &http.Server{Addr: cfg.Addr, Handler: api.New(hostInfo, store, collector, middleware.RequestID, middleware.Logging)}

	tlsConfig, err := buildTLSConfig(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure TLS")
	}
	srv.TLSConfig = tlsConfig

	go func() {
		var err error
		if tlsConfig != nil {
			log.Info().Msgf("Starting server on %s (mTLS)", cfg.Addr)
			err = srv.ListenAndServeTLS(cfg.TLSCert, cfg.TLSKey)
		} else {
			log.Info().Msgf("Starting server on %s", cfg.Addr)
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Msg("failed to start server")
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	<-sig

	// Stop sampling and let Run flush the final metrics-history checkpoint to
	// disk before tearing the server down. Bounded so a stuck filesystem can't
	// hang shutdown.
	stopMetrics()
	select {
	case <-metricsDone:
	case <-time.After(5 * time.Second):
		log.Warn().Msg("timed out waiting for metrics history to flush")
	}

	// Bring every active interface down so no tunnel keeps running (and its
	// PreDown/PostDown rules stay applied) while the agent is stopped. The
	// stored configs are untouched, so the next start re-raises the enabled ones.
	log.Info().Msg("stopping active interfaces")
	service.NewHandler(store, metricsAwg).StopEnabled()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to shutdown server")
	}
}

// buildTLSConfig returns nil when TLS is not configured (plain HTTP, the
// default — meant to be reached through an SSH tunnel to the agent's
// loopback address). When TLSCert/TLSKey/TLSClientCA are all set, it builds
// a server TLS config that requires and verifies a client certificate
// signed by TLSClientCA, for direct mTLS access on a public IP.
func buildTLSConfig(cfg config) (*tls.Config, error) {
	if len(cfg.TLSCert) == 0 && len(cfg.TLSKey) == 0 && len(cfg.TLSClientCA) == 0 {
		return nil, nil
	}
	if len(cfg.TLSCert) == 0 || len(cfg.TLSKey) == 0 || len(cfg.TLSClientCA) == 0 {
		return nil, errors.New("TLS_CERT, TLS_KEY and TLS_CLIENT_CA must all be set together")
	}

	caPEM, err := os.ReadFile(cfg.TLSClientCA)
	if err != nil {
		return nil, fmt.Errorf("read client CA: %w", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("client CA file contains no valid certificates")
	}

	return &tls.Config{
		ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs:  pool,
		MinVersion: tls.VersionTLS12,
	}, nil
}

func loadInterfaces(store storage.Storage) error {
	awg, err := wgctrl.New()
	if err != nil {
		return err
	}
	defer func() { _ = awg.Close() }()

	h := service.NewHandler(store, awg)
	if err := h.All(); err != nil {
		return err
	}

	// Best-effort: a failure here shouldn't stop the agent from starting,
	// it's only a diagnostic. See Handler.DetectOrphans for why this only
	// logs rather than acting on what it finds.
	if orphans, err := h.DetectOrphans(); err != nil {
		log.Warn().Err(err).Msg("failed to check for orphaned WireGuard interfaces")
	} else if len(orphans) > 0 {
		log.Warn().Strs("interfaces", orphans).Msg(
			"found WireGuard interface(s) on this host with no corresponding stored config — " +
				"either left over from before a fix to interface deletion, or created outside awg-admin")
	}

	return nil
}
