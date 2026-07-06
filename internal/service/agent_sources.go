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

package service

import (
	"context"
	"fmt"

	"github.com/ks-tool/awg-admin/internal/deploy"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ListAgentSources returns every saved agent-binary deploy preset (see
// models.AgentSource), shown as options in the "Deploy agent" UI.
func (s *Service) ListAgentSources() ([]models.AgentSource, error) {
	debugOp("ListAgentSources").Msg("listing agent sources")
	return s.store.AgentSources().List()
}

// CreateAgentSource saves a new named deploy preset. Exactly one of url, path
// or image must be set: url is fetched (by the managed server itself, or by
// awg-admin when cacheLocally is set — see models.AgentSource), path reads the
// binary directly from awg-admin's own filesystem, and image is a Docker image
// run as a container on the server. cacheLocally is ignored (forced false) for
// path (nothing to cache) and image (Docker pulls it itself).
func (s *Service) CreateAgentSource(name, url, path, image string, cacheLocally, userspace bool) (*models.AgentSource, error) {
	debugOp("CreateAgentSource").Str("name", name).Msg("creating agent source")
	cacheLocally, userspace, err := validateAgentSourceInput(name, url, path, image, cacheLocally, userspace)
	if err != nil {
		return nil, err
	}

	src := &models.AgentSource{
		ID:           uuid.New(),
		Name:         name,
		URL:          url,
		Path:         path,
		Image:        image,
		CacheLocally: cacheLocally,
		Userspace:    userspace,
	}
	if err := s.store.AgentSources().Set(src); err != nil {
		return nil, err
	}
	return src, nil
}

// UpdateAgentSource edits an existing preset in place (same ID), with the same
// validation as CreateAgentSource. If it stops being a cached URL (caching
// turned off, switched to a path/image kind, or the URL changed), its previously
// cached binary is dropped best-effort — the cache is keyed by ID, so otherwise
// a changed URL would keep serving the stale cached binary until a manual
// refresh.
func (s *Service) UpdateAgentSource(id, name, url, path, image string, cacheLocally, userspace bool) (*models.AgentSource, error) {
	debugOp("UpdateAgentSource").Str("agent_source_id", id).Str("name", name).Msg("updating agent source")
	sID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	existing, err := s.store.AgentSources().Get(sID)
	if err != nil {
		return nil, err
	}

	cacheLocally, userspace, err = validateAgentSourceInput(name, url, path, image, cacheLocally, userspace)
	if err != nil {
		return nil, err
	}

	if existing.CacheLocally && (!cacheLocally || existing.URL != url) {
		if err := deploy.DeleteCache(sID); err != nil {
			log.Warn().Err(err).Str("agent_source_id", id).Msg("failed to remove cached agent binary on update")
		}
	}

	updated := &models.AgentSource{
		ID:           sID,
		Name:         name,
		URL:          url,
		Path:         path,
		Image:        image,
		CacheLocally: cacheLocally,
		Userspace:    userspace,
	}
	if err := s.store.AgentSources().Set(updated); err != nil {
		return nil, err
	}
	return updated, nil
}

// validateAgentSourceInput checks a create/update's fields and normalizes the
// two flags that only apply to some source kinds: cacheLocally (URL only) and
// userspace (URL/Path only, not a Docker image). Returns the normalized flags.
func validateAgentSourceInput(name, url, path, image string, cacheLocally, userspace bool) (bool, bool, error) {
	if len(name) == 0 {
		return false, false, fmt.Errorf("agent source name is required")
	}
	set := 0
	for _, v := range []string{url, path, image} {
		if len(v) > 0 {
			set++
		}
	}
	if set == 0 {
		return false, false, fmt.Errorf("agent source requires a URL, a local file path or a Docker image")
	}
	if set > 1 {
		return false, false, fmt.Errorf("agent source must have exactly one of a URL, a local file path or a Docker image")
	}
	if len(path) > 0 || len(image) > 0 {
		cacheLocally = false
	}
	// Userspace only applies to a URL/Path (systemd) source — a Docker image is
	// inherently the userspace agent.
	if len(image) > 0 {
		userspace = false
	}
	return cacheLocally, userspace, nil
}

// DeleteAgentSource removes a saved deploy preset, and its cached binary
// on disk (if any) — best-effort, a cache cleanup failure doesn't block
// removing the preset itself.
func (s *Service) DeleteAgentSource(id string) error {
	debugOp("DeleteAgentSource").Str("agent_source_id", id).Msg("deleting agent source")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}

	src, err := s.store.AgentSources().Get(sID)
	if err != nil {
		return err
	}
	if src.CacheLocally {
		if err := deploy.DeleteCache(sID); err != nil {
			log.Warn().Err(err).Str("agent_source_id", id).Msg("failed to remove cached agent binary")
		}
	}

	return s.store.AgentSources().Delete(sID)
}

// RefreshAgentSourceCache re-downloads a CacheLocally preset's binary,
// replacing whatever was previously cached — for "rolling" URLs whose
// content changes over time, where a deploy would otherwise keep using
// whatever was cached the first time forever.
func (s *Service) RefreshAgentSourceCache(id string) error {
	debugOp("RefreshAgentSourceCache").Str("agent_source_id", id).Msg("refreshing agent source cache")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}

	src, err := s.store.AgentSources().Get(sID)
	if err != nil {
		return err
	}
	if !src.CacheLocally {
		return fmt.Errorf("agent source %q is not cached locally", src.Name)
	}

	_, err = deploy.RefreshCache(context.Background(), *src)
	return err
}
