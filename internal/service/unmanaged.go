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

	"github.com/ks-tool/awg-admin/internal/agentclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ListUnmanagedInterfaces returns the live WireGuard/AmneziaWG devices on
// serverID's host that its agent has no config for — interfaces brought up
// outside awg-admin (wg-quick/awg-quick, `ip link`), invisible to the normal
// interface list because that reads the database. The agent scans on every
// call, so an interface raised a moment ago shows up. Shown read-only in the
// server's interface list; the way to take one over is PreviewImport /
// ImportInterfaceFromServer. Returns a non-nil slice (marshals as []).
func (s *Service) ListUnmanagedInterfaces(serverID string) ([]models.UnmanagedInterface, error) {
	debugOp("ListUnmanagedInterfaces").Str("server_id", serverID).Msg("listing unmanaged interfaces")
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	list, err := s.listUnmanaged(srv)
	if err != nil {
		return nil, err
	}
	if list == nil {
		list = []models.UnmanagedInterface{}
	}
	return list, nil
}

// listUnmanaged fetches srv's agent's unmanaged-interface list through callAgent.
func (s *Service) listUnmanaged(srv *models.Server) ([]models.UnmanagedInterface, error) {
	var list []models.UnmanagedInterface
	err := s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		var lErr error
		list, lErr = c.ListUnmanaged(ctx)
		return lErr
	})
	return list, err
}

// rejectUnmanagedName refuses to create (or rename to) an interface named like
// a live, unmanaged one on the server: the agent's push would adopt that link
// in place (the update path, see agent service.Handler.One), silently
// replacing a wg-quick interface's keys and peers with a brand-new config. The
// wg-quick import is the one legitimate way to take such a name — it pushes
// the interface's own config. Best-effort by design: when the agent can't be
// asked (unreachable, or an older agent without the endpoint → 404) the check
// is skipped rather than blocking the create; the push that follows would fail
// in the same way anyway and get recorded on the interface's sync status.
func (s *Service) rejectUnmanagedName(sID uuid.UUID, name string) error {
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}
	list, err := s.listUnmanaged(srv)
	if err != nil {
		log.Debug().Err(err).Str("server_id", sID.String()).Str("name", name).
			Msg("could not check the name against the agent's unmanaged interfaces, skipping")
		return nil
	}
	for _, u := range list {
		if u.Name == name {
			return invalidInput("interface name %q is already used on the server by an interface awg-admin doesn't manage (%s); import it instead of creating a new one", name, u.Kind)
		}
	}
	return nil
}
