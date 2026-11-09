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
	"encoding/json"
	"errors"
	"net/http"

	"github.com/ks-tool/awg-admin/agent/internal/metrics"
	"github.com/ks-tool/awg-admin/agent/internal/resource"
	"github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/agent/mux"
	"github.com/ks-tool/awg-admin/agent/storage"
)

func NewWithResources(hostInfo models.HostInfo, store storage.Storage, collector *metrics.Collector, runtime *resource.Runtime, mws ...mux.Middleware) http.Handler {
	router := mux.NewServeMux()
	router.Handle("/v1/", NewResourceHandler(runtime, mws...))
	router.Handle("/", New(hostInfo, store, collector, mws...))
	return router
}

func NewResourceHandler(runtime *resource.Runtime, mws ...mux.Middleware) http.Handler {
	h := &resourceHandler{runtime: runtime}
	router := mux.NewServeMux(mws...)
	router.GET("/v1/manifest", h.manifest)
	router.GET("/v1/providers/{provider}/schema", h.schema)
	router.GET("/v1/providers/{provider}/resources", h.list)
	router.PUT("/v1/providers/{provider}/resources/{kind}/{name}", h.put)
	router.DELETE("/v1/providers/{provider}/resources/{kind}/{name}", h.delete)
	router.GET("/v1/providers/{provider}/resources/{kind}/{name}/status", h.status)
	router.POST("/v1/providers/{provider}/resources/{kind}/{name}/actions/{action}", h.action)
	router.GET("/v1/events", h.events)
	return router
}

type resourceHandler struct {
	runtime *resource.Runtime
}

func (h *resourceHandler) manifest(w http.ResponseWriter, _ *http.Request) {
	encode(w, h.runtime.Manifest())
}

func (h *resourceHandler) schema(w http.ResponseWriter, r *http.Request) {
	definition, err := h.runtime.Schema(r.PathValue("provider"))
	if err != nil {
		resourceError(w, err)
		return
	}
	encode(w, definition)
}

func (h *resourceHandler) list(w http.ResponseWriter, r *http.Request) {
	resources, err := h.runtime.List(r.PathValue("provider"))
	if err != nil {
		resourceError(w, err)
		return
	}
	encode(w, resources)
}

func (h *resourceHandler) put(w http.ResponseWriter, r *http.Request) {
	var desired models.Resource
	if err := decode(r, &desired); err != nil {
		badRequest(w, err)
		return
	}
	result, err := h.runtime.Put(r.PathValue("provider"), r.PathValue("kind"), r.PathValue("name"), desired)
	if err != nil {
		resourceError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
	encode(w, result)
}

func (h *resourceHandler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.runtime.Delete(r.PathValue("provider"), r.PathValue("kind"), r.PathValue("name")); err != nil {
		resourceError(w, err)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (h *resourceHandler) status(w http.ResponseWriter, r *http.Request) {
	result, err := h.runtime.Get(r.PathValue("provider"), r.PathValue("kind"), r.PathValue("name"))
	if err != nil {
		resourceError(w, err)
		return
	}
	encode(w, result.Status)
}

func (h *resourceHandler) action(w http.ResponseWriter, r *http.Request) {
	var request models.ActionRequest
	if err := decode(r, &request); err != nil {
		badRequest(w, err)
		return
	}
	result, err := h.runtime.Action(r.Context(), r.PathValue("provider"), r.PathValue("kind"), r.PathValue("name"), r.PathValue("action"), request.Arguments)
	if err != nil {
		resourceError(w, err)
		return
	}
	encode(w, result)
}

func (h *resourceHandler) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-cache")
	events, cancel := h.runtime.Subscribe()
	defer cancel()
	encoder := json.NewEncoder(w)
	for {
		select {
		case <-r.Context().Done():
			return
		case event, open := <-events:
			if !open {
				return
			}
			if err := encoder.Encode(event); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func resourceError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case storage.IsNotFound(err), errors.Is(err, resource.ErrUnknownProvider), errors.Is(err, resource.ErrUnknownResource), errors.Is(err, resource.ErrUnknownAction):
		status = http.StatusNotFound
	}
	http.Error(w, err.Error(), status)
}
