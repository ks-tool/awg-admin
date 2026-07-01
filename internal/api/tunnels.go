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

	"github.com/ks-tool/awg-admin/models"

	"github.com/rs/zerolog/log"
	"github.com/uptrace/bunrouter"
)

type buildTunnelRequest struct {
	Steps  []models.TunnelStep `json:"steps"`
	Subnet string              `json:"subnet"`
}

func (h *Handler) tunnelList(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}
	log.Debug().Fields(fields).Msg("listing tunnels")

	tunnels, err := h.svc.ListTunnels()
	if err != nil {
		return handleErr(err, fields)
	}
	return bunrouter.JSON(w, tunnels)
}

func (h *Handler) tunnelBuild(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	var req buildTunnelRequest
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Msg("building tunnel")

	tunnel, err := h.svc.BuildTunnel(req.Steps, req.Subnet)
	if err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, tunnel)
}

func (h *Handler) tunnelDelete(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}
	id := r.Param("tunnelID")
	fields["tunnel_id"] = id
	log.Debug().Fields(fields).Msg("removing tunnel")

	if err := h.svc.RemoveTunnel(id); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
