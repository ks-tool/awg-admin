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
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ks-tool/vpn-agent/internal/api"
	agentconfig "github.com/ks-tool/vpn-agent/internal/config"
	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/mux/middleware"
)

type ServeOptions struct {
	Addr        string
	TLSCert     string
	TLSKey      string
	TLSClientCA string
	Store       resource.Store
	AgentID     string
	Provider    resource.Provider
	Start       func() error
	Handler     func(*resource.Runtime) http.Handler
	Close       func() error
}

func Serve(options ServeOptions) error {
	runtime := resource.NewRuntime(options.Store, options.AgentID, time.Second, 30*time.Second)
	defer runtime.Close()
	if err := runtime.Register(options.Provider); err != nil {
		return err
	}
	runtime.Start(context.Background())
	if options.Start != nil {
		if err := options.Start(); err != nil {
			return err
		}
	}
	handler := options.Handler
	if handler == nil {
		handler = func(runtime *resource.Runtime) http.Handler {
			return api.NewResourceHandler(runtime, middleware.RequestID, middleware.Logging)
		}
	}
	srv := &http.Server{Addr: options.Addr, Handler: handler(runtime)}
	var err error
	srv.TLSConfig, err = agentconfig.TLS(options.TLSCert, options.TLSKey, options.TLSClientCA)
	if err != nil {
		return err
	}
	serveErrors := make(chan error, 1)
	go func() {
		if srv.TLSConfig != nil {
			serveErrors <- srv.ListenAndServeTLS(options.TLSCert, options.TLSKey)
			return
		}
		serveErrors <- srv.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	select {
	case err := <-serveErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-signals:
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return err
	}
	if options.Close != nil {
		return options.Close()
	}
	return nil
}
