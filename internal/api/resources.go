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
	"net/http"

	"github.com/ks-tool/vpn-admin/models"
	"github.com/uptrace/bunrouter"
)

func resourceFields(r bunrouter.Request) map[string]any {
	return map[string]any{"method": r.Method, "path": r.URL.Path, "server_id": r.Param("serverID")}
}

func (h *Handler) providerManifest(w http.ResponseWriter, r bunrouter.Request) error {
	var manifest *models.Manifest
	var err error
	if r.URL.Query().Get("refresh") == "true" {
		manifest, err = h.svc.RefreshProviderManifest(r.Param("serverID"))
	} else {
		manifest, err = h.svc.GetProviderManifest(r.Param("serverID"))
	}
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	return bunrouter.JSON(w, manifest)
}

func (h *Handler) providerSchema(w http.ResponseWriter, r bunrouter.Request) error {
	definition, err := h.svc.GetProviderSchema(r.Param("serverID"), r.Param("provider"))
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	return bunrouter.JSON(w, definition)
}

func (h *Handler) providerResourceList(w http.ResponseWriter, r bunrouter.Request) error {
	resources, err := h.svc.ListProviderResources(r.Param("serverID"), r.Param("provider"))
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	return bunrouter.JSON(w, resources)
}

func (h *Handler) providerResourcePut(w http.ResponseWriter, r bunrouter.Request) error {
	var input struct {
		Spec json.RawMessage `json:"spec"`
	}
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	resource, err := h.svc.PutProviderResource(r.Param("serverID"), r.Param("provider"), r.Param("kind"), r.Param("name"), input.Spec)
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	w.WriteHeader(http.StatusAccepted)
	return bunrouter.JSON(w, resource)
}

func (h *Handler) providerResourceDelete(w http.ResponseWriter, r bunrouter.Request) error {
	if err := h.svc.DeleteProviderResource(r.Param("serverID"), r.Param("provider"), r.Param("kind"), r.Param("name")); err != nil {
		return handleErr(err, resourceFields(r))
	}
	w.WriteHeader(http.StatusAccepted)
	return nil
}

func (h *Handler) providerResourceStatus(w http.ResponseWriter, r bunrouter.Request) error {
	status, err := h.svc.GetProviderResourceStatus(r.Param("serverID"), r.Param("provider"), r.Param("kind"), r.Param("name"))
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	return bunrouter.JSON(w, status)
}

func (h *Handler) providerResourceAction(w http.ResponseWriter, r bunrouter.Request) error {
	var input struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := decode(r, &input); err != nil {
		return badRequest(err)
	}
	result, err := h.svc.RunProviderResourceAction(r.Param("serverID"), r.Param("provider"), r.Param("kind"), r.Param("name"), r.Param("action"), input.Arguments)
	if err != nil {
		return handleErr(err, resourceFields(r))
	}
	return bunrouter.JSON(w, result)
}
