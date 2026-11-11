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

package proxy

import (
	"context"
	"os"
	"path/filepath"
	"time"

	agentconfig "github.com/ks-tool/vpn-agent/internal/config"
	"github.com/ks-tool/vpn-agent/internal/resource"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Config struct {
	LogLevel    zerolog.Level `env:"LOG_LEVEL" envDefault:"info" flag:"log-level" help:"logging level"`
	Addr        string        `env:"ADDR" envDefault:"127.0.0.1:9092" flag:"addr" help:"HTTP listen address"`
	DB          string        `env:"DB" flag:"db" help:"provider state directory"`
	Core        string        `env:"CORE" flag:"core" help:"core binary path"`
	TLSCert     string        `env:"TLS_CERT" flag:"tls-cert" help:"TLS certificate file"`
	TLSKey      string        `env:"TLS_KEY" flag:"tls-key" help:"TLS private key file"`
	TLSClientCA string        `env:"TLS_CLIENT_CA" flag:"tls-client-ca" help:"client CA certificate file"`
}

func Run(adapter Adapter, version, prefix, defaultCore string) {
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stdout, TimeFormat: time.RFC3339})
	handled, deployErr := DeployCommand(os.Args[1:], func(ctx context.Context, path string) error {
		return DeployProvider(ctx, adapter, path)
	})
	if handled {
		if deployErr != nil {
			log.Fatal().Err(deployErr).Msg("provider deploy failed")
		}
		return
	}
	cfg := Config{Addr: "127.0.0.1:9092", Core: defaultCore}
	if err := agentconfig.Load(&cfg, prefix+"_", os.Args[1:]); err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}
	zerolog.SetGlobalLevel(cfg.LogLevel)
	if len(cfg.DB) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal().Err(err).Msg("failed to locate home directory")
		}
		cfg.DB = filepath.Join(home, ".vpn-provider-"+adapter.Name())
	}
	resourceStore, err := resource.NewFileStore(filepath.Join(cfg.DB, "resources"))
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize resource storage")
	}
	agentID, err := resourceStore.AgentID()
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize provider identity")
	}
	provider, err := New(adapter, version, resourceStore, cfg.DB, cfg.Core)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to initialize provider")
	}
	log.Info().Str("provider", adapter.Name()).Str("version", version).Str("addr", cfg.Addr).Msg("starting provider API")
	if err := Serve(ServeOptions{
		Addr: cfg.Addr, TLSCert: cfg.TLSCert, TLSKey: cfg.TLSKey, TLSClientCA: cfg.TLSClientCA,
		Store: resourceStore, AgentID: agentID, Provider: provider, Start: provider.RestoreLinks, Close: provider.Close,
	}); err != nil {
		log.Fatal().Err(err).Msg("provider API failed")
	}
}

func DefaultCorePath(name string) string {
	return filepath.Join("/usr/local/bin", name)
}
