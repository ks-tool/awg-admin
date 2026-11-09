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
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/ks-tool/vpn-admin/internal/api"
	"github.com/ks-tool/vpn-admin/internal/config"
	"github.com/ks-tool/vpn-admin/internal/service"
	"github.com/ks-tool/vpn-admin/storage/boltdb"

	"github.com/rs/zerolog/log"
	"github.com/uptrace/bunrouter"
	"golang.org/x/crypto/acme/autocert"
)

//go:embed all:dist
var dist embed.FS

// version is set at build time via -ldflags "-X main.version=...". See the
// "admin" build in .goreleaser.yaml.
var version = "dev"

// TLS for standalone mode is configured via environment variables:
//   - VPN_ADMIN_TLS_CERT / VPN_ADMIN_TLS_KEY: serve HTTPS with a static
//     certificate/key pair.
//   - VPN_ADMIN_AUTOCERT_DOMAINS: comma-separated list of domains to serve
//     HTTPS with a certificate obtained automatically from Let's Encrypt
//     (golang.org/x/crypto/acme/autocert). VPN_ADMIN_AUTOCERT_CACHE_DIR
//     selects where issued certificates are cached (default
//     "$HOME/.vpn-admin/pki"). Requires port 80 to be reachable for the HTTP-01
//     challenge.
//
// The two are mutually exclusive; when neither is set, the server falls
// back to plain HTTP.
func main() {
	log.Info().Str("version", version).Msg("starting vpn-admin")

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}

	db, err := boltdb.Open(config.AdminDatabasePath(home))
	if err != nil {
		log.Fatal().Err(err).Send()
	}

	svc := service.New(db, service.WithVersion(version))
	svc.StartTunnels()
	defer svc.StopTunnels()

	distFS, err := fs.Sub(dist, "dist")
	if err != nil {
		log.Fatal().Err(err).Send()
	}
	fileServer := http.FileServer(http.FS(distFS))

	router := bunrouter.New()
	router.GET("/*path", bunrouter.HTTPHandler(fileServer))

	sessions := api.New(svc, router.NewGroup(""))

	addr := config.Env("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	srv := &http.Server{Addr: addr, Handler: api.ContentSecurityPolicyMiddleware(api.CorsMiddleware(api.BasicAuthMiddleware(svc, sessions)(api.Serve(router))))}

	certFile := config.Env("TLS_CERT")
	keyFile := config.Env("TLS_KEY")
	autocertDomains := config.Env("AUTOCERT_DOMAINS")
	if certFile != "" && autocertDomains != "" {
		log.Fatal().Msg("VPN_ADMIN_TLS_CERT/VPN_ADMIN_TLS_KEY and VPN_ADMIN_AUTOCERT_DOMAINS are mutually exclusive")
	}

	var certManager *autocert.Manager
	if autocertDomains != "" {
		cacheDir := config.Env("AUTOCERT_CACHE_DIR")
		if cacheDir == "" {
			cacheDir = config.AdminAutocertPath(home)
		}
		certManager = &autocert.Manager{
			Prompt:     autocert.AcceptTOS,
			Cache:      autocert.DirCache(cacheDir),
			HostPolicy: autocert.HostWhitelist(strings.Split(autocertDomains, ",")...),
		}
		srv.TLSConfig = certManager.TLSConfig()
	}

	go func() {
		var err error
		switch {
		case certManager != nil:
			challengeSrv := &http.Server{Addr: ":80", Handler: certManager.HTTPHandler(nil)}
			go func() {
				if err := challengeSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
					log.Error().Err(err).Msg("ACME challenge server failed")
				}
			}()
			log.Info().Msgf("Starting server on %s (autocert: %s)", addr, autocertDomains)
			err = srv.ListenAndServeTLS("", "")
		case certFile != "" && keyFile != "":
			log.Info().Msgf("Starting server on %s (TLS)", addr)
			err = srv.ListenAndServeTLS(certFile, keyFile)
		default:
			log.Info().Msgf("Starting server on %s", addr)
			err = srv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal().Err(err).Send()
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Send()
	}
}
