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

	"github.com/ks-tool/vpn-admin/models"
	"github.com/uptrace/bunrouter"
)

func (h *Handler) registrySourceList(w http.ResponseWriter, _ bunrouter.Request) error {
	list, err := h.svc.ListRegistrySources()
	if err != nil {
		return err
	}
	return bunrouter.JSON(w, list)
}

func (h *Handler) registrySourceCreate(w http.ResponseWriter, r bunrouter.Request) error {
	var req struct {
		Name               string `json:"name"`
		URL                string `json:"url"`
		AgentRepository    string `json:"agentRepository"`
		ProviderRepository string `json:"providerRepository"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		PlainHTTP          bool   `json:"plainHttp"`
	}
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}
	source, err := h.svc.CreateRegistrySource(req.Name, req.URL, req.AgentRepository, req.ProviderRepository, req.Username, req.Password, req.PlainHTTP)
	if err != nil {
		return badRequest(err)
	}
	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, source)
}

func (h *Handler) registrySourceUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	var req struct {
		Name               string `json:"name"`
		URL                string `json:"url"`
		AgentRepository    string `json:"agentRepository"`
		ProviderRepository string `json:"providerRepository"`
		Username           string `json:"username"`
		Password           string `json:"password"`
		PlainHTTP          bool   `json:"plainHttp"`
	}
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}
	source, err := h.svc.UpdateRegistrySource(r.Param("sourceID"), req.Name, req.URL, req.AgentRepository, req.ProviderRepository, req.Username, req.Password, req.PlainHTTP)
	if err != nil {
		return handleErr(err, map[string]any{"method": r.Method, "path": r.URL.Path})
	}
	return bunrouter.JSON(w, source)
}

func (h *Handler) registrySourceDelete(w http.ResponseWriter, r bunrouter.Request) error {
	if err := h.svc.DeleteRegistrySource(r.Param("sourceID")); err != nil {
		return handleErr(err, map[string]any{"method": r.Method, "path": r.URL.Path})
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) artifactTagList(w http.ResponseWriter, r bunrouter.Request) error {
	var req models.RegistryRequest
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}
	tags, err := h.svc.ListArtifactTags(req.Repository, req.Registry)
	if err != nil {
		return handleErr(err, map[string]any{"method": r.Method, "path": r.URL.Path, "repository": req.Repository})
	}
	return bunrouter.JSON(w, tags)
}

func (h *Handler) artifactProviderList(w http.ResponseWriter, r bunrouter.Request) error {
	var req models.RegistryRequest
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}
	providers, err := h.svc.ListProviderRepositories(req.Repository, req.Registry)
	if err != nil {
		return handleErr(err, map[string]any{"method": r.Method, "path": r.URL.Path, "repository": req.Repository})
	}
	return bunrouter.JSON(w, providers)
}
