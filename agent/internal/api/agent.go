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
	"net/http/httputil"
	"net/url"

	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/mux"
)

func NewAgent(hostInfo models.HostInfo, runtime *resource.Runtime, legacyProxy string, mws ...mux.Middleware) (http.Handler, error) {
	router := mux.NewServeMux()
	router.Handle("/v1/", NewResourceHandler(runtime, mws...))
	router.GET("/info", func(w http.ResponseWriter, _ *http.Request) { encode(w, hostInfo) })
	if legacyProxy == "" {
		router.HandleFunc("/", http.NotFound)
		return router, nil
	}
	target, err := url.Parse(legacyProxy)
	if err != nil {
		return nil, err
	}
	router.Handle("/", httputil.NewSingleHostReverseProxy(target))
	return router, nil
}
