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
	"errors"
	"net/http"

	"github.com/rs/zerolog/log"
	"github.com/uptrace/bunrouter"
)

func (h *Handler) agentSourceList(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	list, err := h.svc.ListAgentSources()
	if err != nil {
		return handleErr(err, fields)
	}
	return bunrouter.JSON(w, list)
}

// agentSourceReleases returns the newest awg-agent binaries published on GitHub
// (tagged agent/v*), for the Add-agent-source UI's "GitHub releases" picker.
func (h *Handler) agentSourceReleases(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}
	log.Debug().Fields(fields).Msg("listing agent releases from github")

	list, err := h.svc.ListAgentReleases()
	if err != nil {
		return handleErr(err, fields)
	}
	return bunrouter.JSON(w, list)
}

func (h *Handler) agentSourceCreate(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	var req struct {
		Name         string `json:"name"`
		URL          string `json:"url"`
		Path         string `json:"path"`
		Image        string `json:"image"`
		CacheLocally bool   `json:"cacheLocally"`
		Userspace    bool   `json:"userspace"`
	}
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Str("name", req.Name).Msg("creating agent source")

	src, err := h.svc.CreateAgentSource(req.Name, req.URL, req.Path, req.Image, req.CacheLocally, req.Userspace)
	if err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusCreated)
	return bunrouter.JSON(w, src)
}

func (h *Handler) agentSourceUpdate(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	id := r.Param("sourceID")
	if len(id) == 0 {
		return badRequest(errors.New("sourceID required"))
	}
	fields["agent_source_id"] = id

	var req struct {
		Name         string `json:"name"`
		URL          string `json:"url"`
		Path         string `json:"path"`
		Image        string `json:"image"`
		CacheLocally bool   `json:"cacheLocally"`
		Userspace    bool   `json:"userspace"`
	}
	if err := decode(r, &req); err != nil {
		return badRequest(err)
	}
	log.Debug().Fields(fields).Str("name", req.Name).Msg("updating agent source")

	src, err := h.svc.UpdateAgentSource(id, req.Name, req.URL, req.Path, req.Image, req.CacheLocally, req.Userspace)
	if err != nil {
		return handleErr(err, fields)
	}

	return bunrouter.JSON(w, src)
}

func (h *Handler) agentSourceRefresh(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	id := r.Param("sourceID")
	if len(id) == 0 {
		return badRequest(errors.New("sourceID required"))
	}
	fields["agent_source_id"] = id
	log.Debug().Fields(fields).Msg("refreshing agent source cache")

	if err := h.svc.RefreshAgentSourceCache(id); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) agentSourceDelete(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}

	id := r.Param("sourceID")
	if len(id) == 0 {
		return badRequest(errors.New("sourceID required"))
	}
	fields["agent_source_id"] = id

	if err := h.svc.DeleteAgentSource(id); err != nil {
		return handleErr(err, fields)
	}

	w.WriteHeader(http.StatusNoContent)
	return nil
}
