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
	"time"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/internal/agentclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// ReconcileReport is the result of comparing a server's agent's actual
// interfaces against what the local admin DB has recorded for it — see
// ReconcileServer. Both halves are normally empty; entries appear only
// when one side lost its state (the admin DB was wiped/restored from an
// older backup, or the agent's own storage was lost/reinstalled) — see
// TODO.md's "Удаление интерфейса не убирает его на ОС-уровне" section for
// the full background. Deliberately doesn't try to resolve either side
// automatically — the caller (the UI) presents a choice to a human instead.
type ReconcileReport struct {
	// AgentOnly are interfaces the agent has configured that this DB has
	// no record of for this server at all.
	AgentOnly []agentmodels.InterfaceConfig `json:"agentOnly"`
	// DBOnly are interfaces in this DB that the agent didn't report back.
	DBOnly []models.Interface `json:"dbOnly"`
}

// ReconcileServer fetches serverID's agent's actual interface list and
// diffs it against the local DB's records for that server, by interface
// name. Read-only — it only reports mismatches, see ImportInterface/
// DeleteAgentInterface (and the existing SyncServer/DeleteInterface) for
// the actions a human can take on what it finds.
func (s *Service) ReconcileServer(serverID string) (*ReconcileReport, error) {
	debugOp("ReconcileServer").Str("server_id", serverID).Msg("reconciling server")
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	var agentIfaces []agentmodels.InterfaceConfig
	if err := s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		var lErr error
		agentIfaces, lErr = c.List(ctx)
		return lErr
	}); err != nil {
		return nil, fmt.Errorf("list agent interfaces: %w", err)
	}

	dbIfaces, err := s.store.Servers().Interfaces(sID).List()
	if err != nil {
		return nil, err
	}

	dbByName := make(map[string]models.Interface, len(dbIfaces))
	for _, iface := range dbIfaces {
		dbByName[iface.Interface] = iface
	}
	agentByName := make(map[string]agentmodels.InterfaceConfig, len(agentIfaces))
	for _, cfg := range agentIfaces {
		agentByName[cfg.Interface] = cfg
	}

	// Initialize to empty (non-nil) slices so both halves marshal as JSON
	// `[]` rather than `null` when there's no mismatch — the frontend reads
	// .length off them directly.
	report := &ReconcileReport{
		AgentOnly: []agentmodels.InterfaceConfig{},
		DBOnly:    []models.Interface{},
	}
	for name, cfg := range agentByName {
		if _, ok := dbByName[name]; !ok {
			report.AgentOnly = append(report.AgentOnly, cfg)
		}
	}
	for name, iface := range dbByName {
		if _, ok := agentByName[name]; !ok {
			report.DBOnly = append(report.DBOnly, iface)
		}
	}
	return report, nil
}

// ImportInterface creates a new models.Interface in the local DB from
// ifaceName's current config on serverID's agent — the "agent has it, DB
// doesn't" half of ReconcileServer.
//
// Only the interface shell is recovered (address, keys, AmneziaWG params,
// and whatever peer public-key/AllowedIPs entries the agent reports for
// its InterfaceConfig.Peers). The admin-side association between a peer
// and the models.User it belongs to is NOT recoverable from the agent —
// the agent only ever stores the server-side wire shape
// (agentmodels.InterfacePeer: public key, AllowedIPs, PSK, keepalive), it
// has no idea which awg-admin user a key belongs to. If those user/peer
// records are also gone from the DB, they need to be re-created by hand
// (or the existing peers re-added via AddPeer, which will then need new
// keys — the old client configs/QR codes for them are gone for good,
// since the private keys never lived anywhere but the lost DB).
func (s *Service) ImportInterface(serverID, ifaceName string) (*models.Interface, error) {
	debugOp("ImportInterface").Str("server_id", serverID).Str("interface", ifaceName).Msg("importing interface from agent")
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	var cfg *agentmodels.InterfaceConfig
	if err := s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		var gErr error
		cfg, gErr = c.Get(ctx, ifaceName)
		return gErr
	}); err != nil {
		return nil, fmt.Errorf("get %s from agent: %w", ifaceName, err)
	}

	iface := &models.Interface{
		ID:              uuid.New(),
		InterfaceConfig: *cfg,
		// The agent just confirmed this config is exactly what's live —
		// recording it as synced rather than running it back through a
		// pointless push of the same config we just read.
		InSync:       true,
		LastSyncedAt: time.Now(),
	}
	if err := s.store.Servers().Interfaces(sID).Set(iface); err != nil {
		return nil, err
	}
	return iface, nil
}

// DeleteAgentInterface removes ifaceName directly from serverID's agent,
// without touching the local DB (there's nothing there to remove) — the
// other half of the "agent has it, DB doesn't" reconciliation choice,
// alongside ImportInterface.
func (s *Service) DeleteAgentInterface(serverID, ifaceName string) error {
	debugOp("DeleteAgentInterface").Str("server_id", serverID).Str("interface", ifaceName).Msg("deleting agent-only interface")
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}

	return s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		return c.Delete(ctx, ifaceName)
	})
}
