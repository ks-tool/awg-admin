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

	"github.com/ks-tool/vpn-admin/agent/internal/api"
	"github.com/ks-tool/vpn-admin/agent/internal/provider/example"
	"github.com/ks-tool/vpn-admin/agent/internal/resource"
	"github.com/ks-tool/vpn-admin/agent/mux/middleware"

	"github.com/rs/zerolog/log"
)

var version = "dev"

func main() {
	addr := os.Getenv("VPN_PROVIDER_ADDR")
	if addr == "" {
		addr = "127.0.0.1:9090"
	}
	db := os.Getenv("VPN_PROVIDER_DB")
	if db == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			log.Fatal().Err(err).Send()
		}
		db = filepath.Join(home, ".vpn-example-provider")
	}
	store, err := resource.NewFileStore(db)
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
	server := &http.Server{Addr: addr, Handler: api.NewResourceHandler(runtime, middleware.RequestID, middleware.Logging)}
	go func() {
		log.Info().Str("addr", addr).Str("version", version).Msg("starting example provider")
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
