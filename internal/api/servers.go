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
	"github.com/ks-tool/awg-admin/models"
	"github.com/rs/zerolog/log"
	"github.com/uptrace/bunrouter"
)

func (h *Handler) serverList(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}
	log.Debug().Fields(fields).Msg("listing servers")

	serverList, err := h.svc.ListServers()
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, serverList)
}

func (h *Handler) serverCreate(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	var si service.ServerInput
	if err := decode(r, &si); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Msg("creating new server")

	svc, err := h.svc.CreateServer(si)
	if err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, svc)
}

func (h *Handler) serverUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	var si service.ServerInput
	if err = decode(r, &si); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Msg("updating server")

	srv, err := h.svc.UpdateServer(sID, si)
	if err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusAccepted)
	return bunrouter.JSON(w, srv)
}

func (h *Handler) serverGet(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("getting server")

	srv, err := h.svc.GetServer(sID)
	if err != nil {
		return handleErr(err, fields)
	}
	return bunrouter.JSON(w, srv)
}

func (h *Handler) serverGenerateTLS(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("generating agent TLS certificates")

	srv, err := h.svc.GenerateAgentTLS(sID)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, srv)
}

func (h *Handler) serverDeployAgent(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	var req struct {
		AgentSourceID string `json:"agentSourceId"`
	}
	if err = decode(r, &req); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Msg("deploying agent")

	if err = h.svc.DeployAgent(sID, req.AgentSourceID); err != nil {
		return handleErr(err, fields)
	}

	// The deploy keeps running in the background after this returns — see
	// Service.DeployAgent's doc comment. Poll serverDeployStatus for
	// progress/outcome.
	w.WriteHeader(http.StatusAccepted)
	return nil
}

func (h *Handler) serverDeployStatus(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	status, err := h.svc.GetDeployStatus(sID)
	if err != nil {
		return handleErr(err, fields)
	}
	return bunrouter.JSON(w, status)
}

func (h *Handler) serverSync(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("syncing server")

	if err = h.svc.SyncServer(sID); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) serverReconcile(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("reconciling server interfaces")

	report, err := h.svc.ReconcileServer(sID)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, report)
}

func (h *Handler) serverImportInterface(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	var req struct {
		Interface string `json:"interface"`
	}
	if err = decode(r, &req); err != nil {
		return badRequest(err)
	}
	fields["interface"] = req.Interface
	log.Debug().Fields(fields).Msg("importing interface from agent")

	iface, err := h.svc.ImportInterface(sID, req.Interface)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, iface)
}

func (h *Handler) serverDeleteAgentInterface(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	var req struct {
		Interface string `json:"interface"`
	}
	if err = decode(r, &req); err != nil {
		return badRequest(err)
	}
	fields["interface"] = req.Interface
	log.Debug().Fields(fields).Msg("deleting agent-only interface")

	if err = h.svc.DeleteAgentInterface(sID, req.Interface); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) serverMetrics(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("getting server metrics")

	snap, err := h.svc.GetServerMetrics(sID)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, snap)
}

func (h *Handler) serverAgentStatus(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("getting server agent status")

	status, err := h.svc.ServerAgentStatus(sID)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, map[string]models.AgentStatus{"status": status})
}

func (h *Handler) serverHostInfo(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("getting server host info")

	info, err := h.svc.ServerHostInfo(sID)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, info)
}

func (h *Handler) serverMetricsHistory(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("getting server metrics history")

	hist, err := h.svc.GetServerMetricsHistory(sID)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, hist)
}

func (h *Handler) serverSetMonitoring(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err = decode(r, &req); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Bool("enabled", req.Enabled).Msg("setting server monitoring state")

	srv, err := h.svc.SetServerMonitoring(sID, req.Enabled)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, srv)
}

func (h *Handler) serverUnlockSSH(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	var req struct {
		Passphrase string `json:"passphrase"`
		ApplyToAll bool   `json:"applyToAll"`
	}
	if err = decode(r, &req); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Bool("apply_to_all", req.ApplyToAll).Msg("unlocking SSH key with passphrase")

	if err = h.svc.UnlockServerSSH(sID, req.Passphrase, req.ApplyToAll); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) serverDelete(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("deleting server")

	if err = h.svc.DeleteServer(sID); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
