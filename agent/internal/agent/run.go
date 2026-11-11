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

package agent

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/ks-tool/vpn-agent/internal/api"
	agentconfig "github.com/ks-tool/vpn-agent/internal/config"
	"github.com/ks-tool/vpn-agent/internal/metrics"
	providerremote "github.com/ks-tool/vpn-agent/internal/provider/remote"
	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/mux/middleware"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type config struct {
	LogLevel zerolog.Level `env:"LOG_LEVEL" envDefault:"info" flag:"log-level" help:"logging level"`
	Addr     string        `env:"ADDR" envDefault:"127.0.0.1:8080" flag:"addr" help:"HTTP listen address"`
	DB       string        `env:"DB" flag:"db" help:"agent state directory"`

	TLSCert     string `env:"TLS_CERT" flag:"tls-cert" help:"TLS certificate file"`
	TLSKey      string `env:"TLS_KEY" flag:"tls-key" help:"TLS private key file"`
	TLSClientCA string `env:"TLS_CLIENT_CA" flag:"tls-client-ca" help:"client CA certificate file"`

	LegacyProxy     string        `env:"LEGACY_PROXY" flag:"legacy-proxy" help:"optional AWG provider URL for the legacy interface and interface metrics API"`
	MetricsInterval time.Duration `env:"METRICS_INTERVAL" envDefault:"45s" flag:"metrics-interval" help:"system metrics and latency sampling interval"`
}

func Run(version string) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})

	var cfg config
	if err := agentconfig.Load(&cfg, agentEnvironmentPrefix(), os.Args[1:]); err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}
	zerolog.SetGlobalLevel(cfg.LogLevel)
	fmt.Printf("log level set to '%s'\n", cfg.LogLevel)
	log.Info().Str("version", version).Msg("starting vpn-agent")

	if cfg.DB == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			cfg.DB = defaultAgentDBPath(home)
		}
		if cfg.DB == "" {
			log.Fatal().Msg("VPN_AGENT_DB or --db should not be empty")
		}
	}

	resourceStore, err := resource.NewFileStore(filepath.Join(cfg.DB, "resources"))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize resource storage")
	}
	agentID, err := resourceStore.AgentID()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize agent identity")
	}
	resourceRuntime := resource.NewRuntime(resourceStore, agentID, time.Second, 30*time.Second)
	defer resourceRuntime.Close()

	providerDir := filepath.Join(cfg.DB, "providers.d")
	providerConfig := filepath.Join(cfg.DB, "providers.conf")
	registrations, err := providerremote.RegistrationsDir(providerDir, providerConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to read provider registry")
	}
	supervisor := providerremote.NewSupervisor(registrations)
	if err := supervisor.Start(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("failed to start providers")
	}
	defer supervisor.Close()

	remoteProviders, err := providerremote.LoadDir(providerDir, providerConfig, nil)
	if err != nil {
		supervisor.Close()
		log.Fatal().Err(err).Msg("failed to load external providers")
	}
	for _, provider := range remoteProviders {
		if err := resourceRuntime.Register(provider); err != nil {
			supervisor.Close()
			log.Fatal().Err(err).Msg("failed to register external provider")
		}
	}
	resourceRuntime.Start(context.Background())

	systemMetrics := metrics.NewSystemCollector(cfg.MetricsInterval)
	systemMetrics.SetPersistence(filepath.Join(cfg.DB, metrics.HistoryFilename))
	if err := systemMetrics.LoadHistory(); err != nil {
		log.Warn().Err(err).Msg("failed to load persisted system metrics")
	}
	latencyMetrics := metrics.NewLatencyCollector(cfg.LegacyProxy, cfg.MetricsInterval)
	metricsCtx, stopMetrics := context.WithCancel(context.Background())
	defer stopMetrics()
	var metricsWG sync.WaitGroup
	metricsWG.Add(2)
	go func() {
		defer metricsWG.Done()
		systemMetrics.Run(metricsCtx)
	}()
	go func() {
		defer metricsWG.Done()
		latencyMetrics.Run(metricsCtx)
	}()

	hostInfo := GatherHostInfo(version)
	handler, err := api.NewAgent(hostInfo, resourceRuntime, cfg.LegacyProxy, systemMetrics, latencyMetrics, middleware.RequestID, middleware.Logging)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to configure legacy provider proxy")
	}
	srv := &http.Server{Addr: cfg.Addr, Handler: handler}
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
	metricsDone := make(chan struct{})
	go func() {
		metricsWG.Wait()
		close(metricsDone)
	}()
	select {
	case <-metricsDone:
	case <-time.After(5 * time.Second):
		log.Warn().Msg("timed out waiting for metrics collectors")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("failed to shutdown server")
	}
}

func agentEnvironmentPrefix() string {
	const current = "VPN_AGENT_"
	for _, variable := range os.Environ() {
		if strings.HasPrefix(variable, current) {
			return current
		}
	}
	return "AWG_AGENT_"
}

func defaultAgentDBPath(home string) string {
	current := filepath.Join(home, ".vpn-agent")
	if _, err := os.Stat(current); err == nil {
		return current
	}
	legacy := filepath.Join(home, ".awg-agent")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	return current
}
