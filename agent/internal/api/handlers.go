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
	"github.com/ks-tool/awg-admin/agent/internal/service"
	"github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/agent/mux"
	"github.com/ks-tool/awg-admin/agent/storage"

	"github.com/Jipok/wgctrl-go"
	"github.com/rs/zerolog/log"
)

type Handler struct {
	awg       *wgctrl.Client
	store     storage.Storage
	collector *metrics.Collector
}

func New(store storage.Storage, collector *metrics.Collector, mws ...mux.Middleware) http.Handler {
	// PUT         /interfaces     		=> set
	// GET         /interfaces/ 		=> list
	// GET, DELETE /interfaces/{name}	=> get, delete

	// GET         /info				=> info
	// GET         /metrics			=> metrics snapshot (CPU/RAM/LA/network, peer rx/tx/handshake)
	// GET         /metrics/history	=> retained system + per-peer metrics history (up to 48h)
	// PATCH       /metrics			=> enable/disable metrics collection

	awg, err := wgctrl.New()
	if err != nil {
		panic(err)
	}

	h := &Handler{awg: awg, store: store, collector: collector}

	router := mux.NewServeMux(mws...)
	router.PUT("/interfaces", h.set)
	router.GET("/interfaces/", h.list)
	router.GET("/interfaces/{name}", h.get)
	router.DELETE("/interfaces/{name}", h.delete)
	router.GET("/metrics", h.metrics)
	router.GET("/metrics/history", h.metricsHistory)
	router.PATCH("/metrics", h.setMetricsState)

	return router
}

func (h *Handler) set(w http.ResponseWriter, r *http.Request) {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	var cfg models.InterfaceConfig
	if err := decode(r, &cfg); err != nil {
		log.Debug().Fields(fields).Err(err).Msg("decoding interface failed")
		badRequest(w, err)
		return
	}

	if len(cfg.Interface) == 0 {
		err := errors.New("interface name is required")
		log.Debug().Fields(fields).Err(err).Send()
		badRequest(w, err)
		return
	}

	fields["interface"] = cfg.Interface
	log.Debug().Fields(fields).Msg("setting interface")

	// Capture the previously-stored config before overwriting it, so the apply
	// path can reconcile hooks (tear down the old rules, set up the new ones).
	// nil for a brand-new interface (Get returns ErrNotFound).
	old, _ := h.store.Get(cfg.Interface)

	if err := h.store.Set(&cfg); err != nil {
		log.Error().Fields(fields).Err(err).Msg("save interface to DB failed")
		handleErr(w, err)
		return
	}

	networkService := service.NewHandler(h.store, h.awg)
	if err := networkService.One(old, cfg); err != nil {
		log.Error().Fields(fields).Err(err).Msg("configure interface failed")
		if e := h.store.Delete(cfg.Interface); e != nil {
			log.Warn().Fields(fields).Err(e).Msg("delete interface from DB failed")
		}
		handleErr(w, err)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}
	log.Debug().Fields(fields).Msg("listing interfaces")

	devices, err := h.store.List()
	if err != nil {
		log.Error().Fields(fields).Err(err).Msg("list interfaces failed")
		handleErr(w, err)
		return
	}

	encode(w, devices)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	iface, err := ifaceName(r)
	if err != nil {
		log.Debug().Fields(fields).Err(err).Send()
		badRequest(w, err)
		return
	}

	fields["interface"] = iface
	log.Debug().Fields(fields).Msg("getting interface")

	cfg, err := h.store.Get(iface)
	if err != nil {
		if storage.IsNotFound(err) {
			log.Debug().Fields(fields).Err(err).Msg("get config failed")
		} else {
			log.Error().Fields(fields).Err(err).Msg("get config failed")
		}
		handleErr(w, err)
		return
	}

	encode(w, cfg)
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	iface, err := ifaceName(r)
	if err != nil {
		log.Debug().Fields(fields).Err(err).Send()
		badRequest(w, err)
		return
	}

	fields["interface"] = iface
	log.Debug().Fields(fields).Msg("deleting interface")

	networkService := service.NewHandler(h.store, h.awg)
	if err := networkService.Delete(iface); err != nil {
		log.Error().Fields(fields).Err(err).Msg("delete interface failed")
		handleErr(w, err)
		return
	}

	if err := h.store.Delete(iface); err != nil {
		log.Error().Fields(fields).Err(err).Msg("delete interface from DB failed")
		handleErr(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	if h.collector == nil {
		http.Error(w, "metrics collection is disabled", http.StatusServiceUnavailable)
		return
	}

	snap := h.collector.Snapshot()

	if r.URL.Query().Get("fmt") == "prom" {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if err := metrics.WritePrometheus(w, snap); err != nil {
			log.Error().Fields(fields).Err(err).Msg("failed to write prometheus metrics")
		}
		return
	}

	encode(w, snap)
}

func (h *Handler) metricsHistory(w http.ResponseWriter, r *http.Request) {
	if h.collector == nil {
		http.Error(w, "metrics collection is disabled", http.StatusServiceUnavailable)
		return
	}

	encode(w, h.collector.SystemHistory())
}

type metricsStateRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *Handler) setMetricsState(w http.ResponseWriter, r *http.Request) {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	if h.collector == nil {
		http.Error(w, "metrics collection is disabled", http.StatusServiceUnavailable)
		return
	}

	var req metricsStateRequest
	if err := decode(r, &req); err != nil {
		log.Debug().Fields(fields).Err(err).Msg("decoding metrics state failed")
		badRequest(w, err)
		return
	}

	h.collector.SetEnabled(req.Enabled)
	log.Debug().Fields(fields).Bool("enabled", req.Enabled).Msg("set metrics collection state")
	encode(w, metricsStateRequest{Enabled: h.collector.Enabled()})
}

// ─────────────────────────────── helpers ───────────────────────────────────

func decode(r *http.Request, v any) error {
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func encode(w http.ResponseWriter, v any) {
	_ = json.NewEncoder(w).Encode(v)
}

func handleErr(w http.ResponseWriter, err error) {
	var status int
	switch {
	case storage.IsNotFound(err):
		status = http.StatusNotFound
	default:
		status = http.StatusInternalServerError
	}
	http.Error(w, err.Error(), status)
}

func badRequest(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusBadRequest)
}

func ifaceName(r *http.Request) (string, error) {
	iface := r.PathValue("name")
	if len(iface) == 0 {
		return "", errors.New("missing interface name")
	}
	return iface, nil
}
