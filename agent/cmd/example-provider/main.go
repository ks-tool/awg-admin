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

package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ks-tool/vpn-agent/internal/api"
	agentconfig "github.com/ks-tool/vpn-agent/internal/config"
	"github.com/ks-tool/vpn-agent/internal/provider/example"
	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/mux/middleware"

	"github.com/rs/zerolog/log"
)

var version = "dev"

type config struct {
	Addr string `env:"ADDR" envDefault:"127.0.0.1:9090" flag:"addr" help:"HTTP listen address"`
	DB   string `env:"DB" flag:"db" help:"provider state directory"`
}

func main() {
	var cfg config
	if err := agentconfig.Load(&cfg, "VPN_PROVIDER_", os.Args[1:]); err != nil {
		log.Fatal().Err(err).Msg("failed to load configuration")
	}
	if cfg.DB == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal().Err(err).Send()
		}
		cfg.DB = filepath.Join(home, ".vpn-example-provider")
	}
	store, err := resource.NewFileStore(cfg.DB)
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	agentID, err := store.AgentID()
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	provider, err := example.New()
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	runtime := resource.NewRuntime(store, agentID, time.Second, 30*time.Second)
	if err := runtime.Register(provider); err != nil {
		log.Fatal().Err(err).Send()
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	server := &http.Server{Addr: cfg.Addr, Handler: api.NewResourceHandler(runtime, middleware.RequestID, middleware.Logging)}
	go func() {
		log.Info().Str("addr", cfg.Addr).Str("version", version).Msg("starting example provider")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Send()
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}
