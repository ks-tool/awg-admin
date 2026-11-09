//go:build linux

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

package awg

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ks-tool/vpn-agent/internal/agent"
	"github.com/ks-tool/vpn-agent/internal/api"
	agentconfig "github.com/ks-tool/vpn-agent/internal/config"
	"github.com/ks-tool/vpn-agent/internal/kernel"
	"github.com/ks-tool/vpn-agent/internal/metrics"
	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/internal/service"
	"github.com/ks-tool/vpn-agent/internal/userspace"
	"github.com/ks-tool/vpn-agent/mux/middleware"
	"github.com/ks-tool/vpn-agent/storage"
	"github.com/ks-tool/vpn-agent/storage/fs"

	"github.com/Jipok/wgctrl-go"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type config struct {
	LogLevel  zerolog.Level `env:"LOG_LEVEL" envDefault:"info" flag:"log-level" help:"logging level"`
	Addr      string        `env:"ADDR" envDefault:"127.0.0.1:9091" flag:"addr" help:"HTTP listen address"`
	DB        string        `env:"DB" flag:"db" help:"provider state directory"`
	Userspace bool          `env:"USERSPACE" flag:"userspace" help:"force the amneziawg-go userspace backend"`

	TLSCert     string `env:"TLS_CERT" flag:"tls-cert" help:"TLS certificate file"`
	TLSKey      string `env:"TLS_KEY" flag:"tls-key" help:"TLS private key file"`
	TLSClientCA string `env:"TLS_CLIENT_CA" flag:"tls-client-ca" help:"client CA certificate file"`

	MetricsInterval time.Duration `env:"METRICS_INTERVAL" envDefault:"45s" flag:"metrics-interval" help:"metrics sampling interval"`
}

func Run(version string) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	var cfg config
	if err := agentconfig.Load(&cfg, environmentPrefix(), os.Args[1:]); err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}
	zerolog.SetGlobalLevel(cfg.LogLevel)
	fmt.Printf("log level set to '%s'\n", cfg.LogLevel)

	userspaceMode := cfg.Userspace || !kernel.Available()
	if userspaceMode {
		service.SetBackend(userspace.New())
	} else {
		service.SetBackend(kernel.New())
	}
	backend := service.ActiveBackendInfo()
	log.Info().Str("version", version).Str("backend", backend.Kind).Bool("forced", cfg.Userspace).Msg("starting AWG provider")

	if cfg.DB == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.DB = defaultDBPath(home)
		}
		if cfg.DB == "" {
			log.Fatal().Msg("VPN_PROVIDER_AWG_DB or --db should not be empty")
		}
	}

	store, err := fs.New(cfg.DB)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize storage")
	}
	resourceStore, err := resource.NewFileStore(filepath.Join(cfg.DB, "resources"))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize resource storage")
	}
	agentID, err := resourceStore.AgentID()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize provider identity")
	}
	resourceRuntime := resource.NewRuntime(resourceStore, agentID, time.Second, 30*time.Second)
	defer resourceRuntime.Close()

	if err := loadInterfaces(store); err != nil {
		log.Fatal().Err(err).Msg("failed to load WireGuard interfaces")
	}

	watchCtx, stopWatch := context.WithCancel(context.Background())
	defer stopWatch()
	if err := service.WatchStorage(watchCtx, cfg.DB); err != nil {
		log.Warn().Err(err).Msg("failed to start storage watcher")
	}

	metricsAwg, err := wgctrl.New()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to open wgctrl client for metrics")
	}
	defer func() { _ = metricsAwg.Close() }()
	awgProvider, err := New(store, filepath.Join(cfg.DB, "resources", "secrets"), service.NewHandler(store, metricsAwg), version, resourceStore)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize AWG provider")
	}
	if err := resourceRuntime.Register(awgProvider); err != nil {
		log.Fatal().Err(err).Msg("failed to register AWG provider")
	}
	resourceRuntime.Start(context.Background())

	collector := metrics.NewCollector(metricsAwg, store, cfg.MetricsInterval)
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

	hostInfo := agent.GatherHostInfo(version)
	hostInfo.Backend = backend.Kind
	hostInfo.KernelModule = backend.KernelModule
	hostInfo.InterfaceKinds = backend.InterfaceKinds
	srv := &http.Server{Addr: cfg.Addr, Handler: api.NewWithResources(hostInfo, store, collector, resourceRuntime, middleware.RequestID, middleware.Logging)}
	srv.TLSConfig, err = agentconfig.TLS(cfg.TLSCert, cfg.TLSKey, cfg.TLSClientCA)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure TLS")
	}

	go func() {
		var err error
		if srv.TLSConfig != nil {
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

	stopMetrics()
	select {
	case <-metricsDone:
	case <-time.After(5 * time.Second):
		log.Warn().Msg("timed out waiting for metrics history to flush")
	}

	log.Info().Msg("stopping active interfaces")
	service.NewHandler(store, metricsAwg).StopEnabled()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to shutdown server")
	}
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
	if orphans, err := h.DetectOrphans(); err != nil {
		log.Warn().Err(err).Msg("failed to check for orphaned WireGuard interfaces")
	} else if len(orphans) > 0 {
		log.Warn().Strs("interfaces", orphans).Msg("found WireGuard interfaces with no corresponding stored config")
	}
	return nil
}

func environmentPrefix() string {
	for _, prefix := range []string{"VPN_PROVIDER_AWG_", "VPN_AGENT_"} {
		for _, variable := range os.Environ() {
			if strings.HasPrefix(variable, prefix) {
				return prefix
			}
		}
	}
	return "AWG_AGENT_"
}

func defaultDBPath(home string) string {
	current := filepath.Join(home, ".vpn-provider-awg")
	if _, err := os.Stat(current); err == nil {
		return current
	}
	legacy := filepath.Join(home, ".awg-agent")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}
