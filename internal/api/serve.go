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

	"github.com/uptrace/bunrouter"
)

// Serve adapts router into a plain http.Handler that actually writes the
// HTTP status/body for errors returned by route handlers (handleErr's
// *ErrorResponse, badRequest, requireAuth's 401, ...).
//
// (*bunrouter.Router).ServeHTTP discards those errors silently —
// `_ = r.ServeHTTPError(w, req)` — leaving every error response as an empty
// "200 OK" with nothing written. Only bunrouter.HandlerFunc's own
// ServeHTTP (used when a single route is adapted into a raw http.Handler,
// e.g. via bunrouter.HTTPHandler) performs the error-to-http.Error
// conversion; the router itself never does it for the request as a whole.
// Use this instead of the router directly as the top-level http.Server
// Handler.
func Serve(router *bunrouter.Router) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := router.ServeHTTPError(w, r)
		if err == nil {
			return
		}
		code := http.StatusInternalServerError
		if httpErr, ok := err.(bunrouter.HTTPError); ok {
			code = httpErr.StatusCode()
		}
		http.Error(w, err.Error(), code)
	})
}
