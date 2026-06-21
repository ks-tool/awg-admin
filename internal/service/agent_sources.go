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
	return s.store.AgentSources().List()
}

// CreateAgentSource saves a new named deploy preset. Exactly one of url or
// path must be set: url is fetched (by the managed server itself, or by
// awg-admin when cacheLocally is set — see models.AgentSource), path reads
// the binary directly from awg-admin's own filesystem. cacheLocally is
// ignored (forced false) when path is set, since there's nothing to cache.
func (s *Service) CreateAgentSource(name, url, path string, cacheLocally bool) (*models.AgentSource, error) {
	if len(name) == 0 {
		return nil, fmt.Errorf("agent source name is required")
	}
	if len(url) == 0 && len(path) == 0 {
		return nil, fmt.Errorf("agent source requires a URL or a local file path")
	}
	if len(url) > 0 && len(path) > 0 {
		return nil, fmt.Errorf("agent source can't have both a URL and a local file path")
	}
	if len(path) > 0 {
		cacheLocally = false
	}

	src := &models.AgentSource{
		ID:           uuid.New(),
		Name:         name,
		URL:          url,
		Path:         path,
		CacheLocally: cacheLocally,
	}
	if err := s.store.AgentSources().Set(src); err != nil {
		return nil, err
	}
	return src, nil
}

// DeleteAgentSource removes a saved deploy preset, and its cached binary
// on disk (if any) — best-effort, a cache cleanup failure doesn't block
// removing the preset itself.
func (s *Service) DeleteAgentSource(id string) error {
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
