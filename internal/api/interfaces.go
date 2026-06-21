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

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/rs/zerolog/log"

	"github.com/uptrace/bunrouter"
)

func (h *Handler) interfaceList(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}

	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("listing interfaces")

	list, err := h.svc.ListInterfaces(sID)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, list)
}

func (h *Handler) interfaceCreate(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}

	var cfg agentmodels.InterfaceConfig
	if err = decode(r, &cfg); err != nil {
		return badRequest(err)
	}

	fields["server_id"] = sID
	log.Debug().Fields(fields).Msg("creating interfaces")

	iface, err := h.svc.CreateInterface(sID, cfg)
	if err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, iface)
}

func (h *Handler) interfaceGet(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	iID, err := ifaceID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["interface_id"] = iID
	log.Debug().Fields(fields).Msg("getting interfaces")

	iface, err := h.svc.GetInterface(sID, iID)
	if err != nil {
		return handleErr(err, fields)
	}
	return bunrouter.JSON(w, iface)
}

func (h *Handler) interfaceConfigUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	iID, err := ifaceID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["interface_id"] = iID
	log.Debug().Fields(fields).Msg("updating interfaces")

	var cfg agentmodels.InterfaceConfig
	if err = decode(r, &cfg); err != nil {
		return badRequest(err)
	}

	iface, err := h.svc.UpdateInterfaceConfig(sID, iID, cfg)
	if err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusAccepted)
	return bunrouter.JSON(w, iface)
}

func (h *Handler) interfaceDelete(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	sID, err := serverID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["server_id"] = sID

	iID, err := ifaceID(r)
	if err != nil {
		return badRequest(err)
	}
	fields["interface_id"] = iID
	log.Debug().Fields(fields).Msg("deleting interfaces")

	if err = h.svc.DeleteInterface(sID, iID); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
