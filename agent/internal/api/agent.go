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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"github.com/ks-tool/vpn-agent/internal/metrics"
	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/mux"
)

type agentHandler struct {
	system      *metrics.Collector
	latency     *metrics.LatencyCollector
	proxy       http.Handler
	providerURL string
	client      *http.Client
}

func NewAgent(hostInfo models.HostInfo, runtime *resource.Runtime, legacyProxy string, system *metrics.Collector, latency *metrics.LatencyCollector, mws ...mux.Middleware) (http.Handler, error) {
	h := &agentHandler{system: system, latency: latency, providerURL: strings.TrimRight(legacyProxy, "/"), client: &http.Client{}}
	router := mux.NewServeMux()
	router.Handle("/v1/", NewResourceHandler(runtime, mws...))
	router.GET("/info", func(w http.ResponseWriter, _ *http.Request) { encode(w, hostInfo) })
	router.GET("/metrics", h.systemMetrics)
	router.GET("/metrics/history", h.systemMetricsHistory)
	router.GET("/metrics/{interface}", h.interfaceMetrics)
	router.GET("/metrics/{interface}/history", h.interfaceMetricsHistory)
	router.PATCH("/metrics", h.setMetricsState)
	if len(legacyProxy) > 0 {
		target, err := url.Parse(legacyProxy)
		if err != nil {
			return nil, err
		}
		h.proxy = httputil.NewSingleHostReverseProxy(target)
		router.Handle("/", h.proxy)
	} else {
		router.HandleFunc("/", http.NotFound)
	}
	return router, nil
}

func (h *agentHandler) systemMetrics(w http.ResponseWriter, r *http.Request) {
	if h.system == nil {
		http.Error(w, "metrics collection is disabled", http.StatusServiceUnavailable)
		return
	}
	snapshot := h.system.Snapshot().System
	if r.URL.Query().Get("fmt") == "prom" {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = metrics.WritePrometheus(w, models.MetricsSnapshot{System: snapshot})
		return
	}
	encode(w, snapshot)
}

func (h *agentHandler) systemMetricsHistory(w http.ResponseWriter, _ *http.Request) {
	if h.system == nil {
		http.Error(w, "metrics collection is disabled", http.StatusServiceUnavailable)
		return
	}
	history := h.system.SystemHistory()
	history.Interfaces = nil
	encode(w, history)
}

func (h *agentHandler) interfaceMetrics(w http.ResponseWriter, r *http.Request) {
	var snapshot models.MetricsSnapshot
	if err := h.providerGet(r.Context(), "/metrics", &snapshot); err != nil {
		providerMetricsError(w, err)
		return
	}
	name := r.PathValue("interface")
	for _, iface := range snapshot.Interfaces {
		if iface.Interface != name {
			continue
		}
		if h.latency != nil {
			h.latency.Apply(&iface)
		}
		encode(w, iface)
		return
	}
	http.Error(w, fmt.Sprintf("interface %q metrics not found", name), http.StatusNotFound)
}

func (h *agentHandler) interfaceMetricsHistory(w http.ResponseWriter, r *http.Request) {
	var history models.SystemHistory
	if err := h.providerGet(r.Context(), "/metrics/history", &history); err != nil {
		providerMetricsError(w, err)
		return
	}
	name := r.PathValue("interface")
	for _, iface := range history.Interfaces {
		if iface.Interface == name {
			encode(w, iface)
			return
		}
	}
	http.Error(w, fmt.Sprintf("interface %q metrics history not found", name), http.StatusNotFound)
}

func (h *agentHandler) setMetricsState(w http.ResponseWriter, r *http.Request) {
	var state metricsStateRequest
	if err := decode(r, &state); err != nil {
		badRequest(w, err)
		return
	}
	if h.system != nil {
		h.system.SetEnabled(state.Enabled)
	}
	if h.latency != nil {
		h.latency.SetEnabled(state.Enabled)
	}
	if len(h.providerURL) == 0 {
		encode(w, state)
		return
	}
	body, _ := json.Marshal(state)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPatch, h.providerURL+"/metrics", bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := h.client.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (h *agentHandler) providerGet(ctx context.Context, path string, output any) error {
	if len(h.providerURL) == 0 {
		return errors.New("interface metrics provider is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.providerURL+path, nil)
	if err != nil {
		return err
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("provider returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(resp.Body).Decode(output)
}

func providerMetricsError(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadGateway)
}
