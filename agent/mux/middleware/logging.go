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

package middleware

import (
	"cmp"
	"net/http"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type responseWriter struct {
	w      http.ResponseWriter
	status int
	size   int64
}

func (w *responseWriter) Header() http.Header {
	return w.w.Header()
}

func (w *responseWriter) Write(p []byte) (n int, err error) {
	w.size += int64(len(p))
	return w.w.Write(p)
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.w.WriteHeader(status)
}

func (w *responseWriter) StatusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := &responseWriter{w: w}
		next.ServeHTTP(wrapped, r)

		var logger *zerolog.Event
		status := wrapped.StatusCode()
		if between(status, 200, 300) {
			logger = log.Info()
		} else if between(status, 500, 600) {
			logger = log.Error()
		} else {
			logger = log.Debug()
		}

		fields := map[string]any{
			"method": r.Method,
			"path":   r.URL.Path,
			"status": status,
			"size":   wrapped.size,
			"addr":   r.RemoteAddr,
		}

		if reqId := RequestIDFromContext(r.Context()); len(reqId) > 0 {
			fields["request_id"] = reqId
		}

		logger.Fields(fields).Send()
	})
}

func between[T cmp.Ordered](val, min, max T) bool {
	return val >= min && val < max
}
