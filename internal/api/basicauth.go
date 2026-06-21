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

package api

import (
	"net/http"

	"github.com/ks-tool/awg-admin/internal/service"

	"github.com/rs/zerolog/log"
)

// BasicAuthMiddleware wraps the whole standalone server — the login page,
// every API route, the embedded SPA's static assets — behind HTTP Basic
// Auth, checked against the same single admin account used for the
// session-cookie login (internal/api/auth.go). Off by default
// (models.AuthCredentials.BasicAuthEnabled); when off, every request
// passes straight through. This is a coarser gate than the session login:
// it's meant for exposing the standalone server directly on the internet
// without a reverse proxy in front of it, so even the login page/JS bundle
// isn't reachable by an unauthenticated visitor.
//
// A request that already carries a valid session cookie skips the basic
// auth check entirely, regardless of path. Without this, an admin who's
// already logged in gets challenged by the browser's native Basic Auth
// popup for *every* page load/request once this is turned on — including
// the Settings page itself, so turning it back off required first passing
// a second, separate login prompt on top of the session they already had.
// Basic auth here is a perimeter gate against visitors who haven't
// authenticated at all yet (hiding the login page from random scanners);
// it isn't meant to nag someone who already has a valid session.
//
// Must be wrapped *inside* CorsMiddleware (i.e. cmd/awg-admin.go should
// build the handler chain as CorsMiddleware(BasicAuthMiddleware(...))) —
// CorsMiddleware answers cross-origin preflight OPTIONS requests itself
// without calling next, so putting it outside keeps `npm run dev`'s CORS
// preflight working even when basic auth is enabled (browsers don't send
// credentials on preflight requests, so a naive outermost wrap would
// reject every preflight with 401, breaking the dev server in cross-origin
// setups whenever this toggle is on).
func BasicAuthMiddleware(svc *service.Service, sessions *sessionStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if sessions.validRequest(r) {
				next.ServeHTTP(w, r)
				return
			}

			username, password, _ := r.BasicAuth()
			ok, err := svc.VerifyBasicAuth(username, password)
			if err != nil {
				log.Error().Err(err).Msg("basic auth check failed")
				http.Error(w, "internal error", http.StatusInternalServerError)
				return
			}
			if !ok {
				w.Header().Set("WWW-Authenticate", `Basic realm="awg-admin"`)
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
