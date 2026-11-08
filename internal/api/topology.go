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

	"github.com/uptrace/bunrouter"
)

func topologyFields(r bunrouter.Request) map[string]any {
	return map[string]any{"method": r.Method, "path": r.URL.Path}
}

func (h *Handler) topologyGet(w http.ResponseWriter, r bunrouter.Request) error {
	topology, err := h.svc.GetTopology()
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, topology)
}

func (h *Handler) topologyPluginList(w http.ResponseWriter, r bunrouter.Request) error {
	plugins, err := h.svc.ListVPNPlugins()
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, plugins)
}

func (h *Handler) topologyServerCreate(w http.ResponseWriter, r bunrouter.Request) error {
	var input service.TopologyServerInput
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	topology, err := h.svc.CreateTopologyServer(input)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, topology)
}

func (h *Handler) topologyServerUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	var input service.TopologyServerInput
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	topology, err := h.svc.UpdateTopologyServer(r.Param("serverID"), input)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, topology)
}

func (h *Handler) topologyServerDelete(w http.ResponseWriter, r bunrouter.Request) error {
	if err := h.svc.DeleteTopologyServer(r.Param("serverID")); err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) topologyInterfaceCreate(w http.ResponseWriter, r bunrouter.Request) error {
	var input service.NetworkInterfaceInput
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	topology, err := h.svc.CreateNetworkInterface(r.Param("serverID"), input)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, topology)
}

func (h *Handler) topologyInterfaceUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	var input service.NetworkInterfaceInput
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	topology, err := h.svc.UpdateNetworkInterface(r.Param("interfaceID"), input)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, topology)
}

func (h *Handler) topologyInterfaceDelete(w http.ResponseWriter, r bunrouter.Request) error {
	if err := h.svc.DeleteNetworkInterface(r.Param("interfaceID")); err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) topologyServiceCreate(w http.ResponseWriter, r bunrouter.Request) error {
	var input service.VPNServiceInput
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	result, err := h.svc.CreateVPNService(input)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, result)
}

func (h *Handler) topologyServiceUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	var input service.VPNServiceInput
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	result, err := h.svc.UpdateVPNService(r.Param("serviceID"), input)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, result)
}

func (h *Handler) topologyServiceDelete(w http.ResponseWriter, r bunrouter.Request) error {
	if err := h.svc.DeleteVPNService(r.Param("serviceID")); err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) topologyServiceApply(w http.ResponseWriter, r bunrouter.Request) error {
	result, err := h.svc.ApplyVPNService(r.Param("serviceID"))
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, result)
}

func (h *Handler) topologyServiceStatus(w http.ResponseWriter, r bunrouter.Request) error {
	result, err := h.svc.RefreshVPNServiceStatus(r.Param("serviceID"))
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, result)
}

func (h *Handler) topologyConnectionCreate(w http.ResponseWriter, r bunrouter.Request) error {
	var input struct {
		SourceID string `json:"sourceId"`
		TargetID string `json:"targetId"`
	}
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	result, err := h.svc.CreateNetworkConnection(input.SourceID, input.TargetID)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, result)
}

func (h *Handler) topologyConnectionDelete(w http.ResponseWriter, r bunrouter.Request) error {
	if err := h.svc.DeleteNetworkConnection(r.Param("connectionID")); err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) topologyPositionsUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	var input struct {
		Positions []models.NodePosition `json:"positions"`
	}
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	if err := h.svc.MoveTopologyNodes(input.Positions); err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) topologyUserList(w http.ResponseWriter, r bunrouter.Request) error {
	users, err := h.svc.ListVPNServiceUsers(r.Param("serviceID"))
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, users)
}

func (h *Handler) topologyUserCreate(w http.ResponseWriter, r bunrouter.Request) error {
	var input map[string]any
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	user, err := h.svc.CreateVPNServiceUser(r.Param("serviceID"), input)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, user)
}

func (h *Handler) topologyUserUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	var input map[string]any
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	user, err := h.svc.UpdateVPNServiceUser(r.Param("serviceID"), r.Param("userID"), input)
	if err != nil {
		return handleErr(err, topologyFields(r))
	}
	return bunrouter.JSON(w, user)
}

func (h *Handler) topologyUserDelete(w http.ResponseWriter, r bunrouter.Request) error {
	if err := h.svc.DeleteVPNServiceUser(r.Param("serviceID"), r.Param("userID")); err != nil {
		return handleErr(err, topologyFields(r))
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}
