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
	"errors"
	"time"

	"github.com/ks-tool/awg-admin/internal/agentclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

const pushTimeout = 15 * time.Second

// pushInterface sends iface's current config to its server's agent and
// records the outcome on the interface (InSync/LastSyncError/LastSyncedAt).
// Storage is the source of truth — a push failure (agent unreachable,
// rejected config, ...) is recorded but never returned to the caller, so
// editing interfaces/peers keeps working while the agent is offline.
func (s *Service) pushInterface(serverID uuid.UUID, iface *models.Interface) {
	srv, err := s.store.Servers().Get(serverID)
	if err != nil {
		log.Error().Err(err).Str("server_id", serverID.String()).Msg("push interface: failed to load server")
		return
	}

	s.recordSyncResult(serverID, iface, s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		return c.Set(ctx, iface.InterfaceConfig)
	}))
}

func (s *Service) recordSyncResult(serverID uuid.UUID, iface *models.Interface, pushErr error) {
	iface.LastSyncedAt = time.Now()
	if pushErr != nil {
		iface.InSync = false
		iface.LastSyncError = pushErr.Error()
		log.Warn().Err(pushErr).Str("server_id", serverID.String()).Str("interface", iface.Interface).
			Msg("failed to push interface config to agent")
	} else {
		iface.InSync = true
		iface.LastSyncError = ""
	}

	if err := s.store.Servers().Interfaces(serverID).Set(iface); err != nil {
		log.Error().Err(err).Str("server_id", serverID.String()).Str("interface", iface.Interface).
			Msg("failed to persist sync status")
	}
}

// pushInterfaceDelete tells the agent to tear down the named interface.
// Best-effort like pushInterface: the local record is already gone by the
// time this runs, so there's nowhere to record a failure status — it's
// only logged. A 404 from the agent (already gone there too) is not an
// error worth logging.
func (s *Service) pushInterfaceDelete(serverID uuid.UUID, ifaceName string) {
	srv, err := s.store.Servers().Get(serverID)
	if err != nil {
		log.Error().Err(err).Str("server_id", serverID.String()).Msg("push interface delete: failed to load server")
		return
	}

	var notFound *agentclient.NotFoundError
	if err := s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		return c.Delete(ctx, ifaceName)
	}); err != nil && !errors.As(err, &notFound) {
		log.Warn().Err(err).Str("server_id", serverID.String()).Str("interface", ifaceName).
			Msg("failed to delete interface on agent")
	}
}

// SyncServer resends every stored interface of serverID to that server's
// agent. Useful after the agent was redeployed/reinstalled (its local state
// is gone), after a prolonged outage (it may have missed pushes that failed
// while it was down), or just as a manual "force re-apply" from the UI.
func (s *Service) SyncServer(serverID string) error {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}

	ifaces, err := s.store.Servers().Interfaces(sID).List()
	if err != nil {
		return err
	}

	for i := range ifaces {
		s.pushInterface(sID, &ifaces[i])
	}

	s.syncMonitoringState(sID)
	s.syncProfilingState(sID)
	return nil
}

// syncMonitoringState re-applies the stored monitoring on/off preference to
// the agent. Best-effort like pushInterface: a freshly (re)deployed agent's
// collector always starts enabled, so this only matters when monitoring was
// explicitly disabled and needs to be turned back off again.
func (s *Service) syncMonitoringState(serverID uuid.UUID) {
	srv, err := s.store.Servers().Get(serverID)
	if err != nil {
		log.Error().Err(err).Str("server_id", serverID.String()).Msg("sync monitoring state: failed to load server")
		return
	}
	if !srv.Agent.MonitoringDisabled {
		return
	}

	if err := s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		return c.SetMetricsEnabled(ctx, false)
	}); err != nil {
		log.Warn().Err(err).Str("server_id", serverID.String()).Msg("failed to re-apply monitoring state")
	}
}

// syncProfilingState re-applies the stored profiling on/off preference to the
// agent. Best-effort like syncMonitoringState: a freshly (re)deployed agent
// starts with profiling off, so this only matters when it was explicitly turned
// on and needs to be turned back on again after a redeploy/reconnect.
func (s *Service) syncProfilingState(serverID uuid.UUID) {
	srv, err := s.store.Servers().Get(serverID)
	if err != nil {
		log.Error().Err(err).Str("server_id", serverID.String()).Msg("sync profiling state: failed to load server")
		return
	}
	if !srv.Agent.ProfilingEnabled {
		return
	}

	if err := s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		return c.SetProfilingEnabled(ctx, true)
	}); err != nil {
		log.Warn().Err(err).Str("server_id", serverID.String()).Msg("failed to re-apply profiling state")
	}
}

// syncTunnelledServers re-pushes every interface of each server in servers
// that doesn't have mTLS and isn't in tunnelErrs (i.e. its tunnel just
// opened successfully) — called after Manager.OpenAll so an agent that
// missed pushes while its tunnel was down (or is freshly deployed and has
// no config at all) gets caught up as soon as connectivity is restored.
func (s *Service) syncTunnelledServers(servers []models.Server, tunnelErrs map[uuid.UUID]error) {
	for i := range servers {
		srv := &servers[i]
		if !srv.Agent.TLS.IsZero() {
			continue
		}
		if _, failed := tunnelErrs[srv.ID]; failed {
			continue
		}
		if err := s.SyncServer(srv.ID.String()); err != nil {
			log.Warn().Err(err).Str("server_id", srv.ID.String()).Msg("failed to sync server after opening tunnel")
		}
	}
}

// findServerByInterface scans stored servers for the one that owns ifaceID.
// Peers are added/removed by interface ID without the server ID in hand
// (see internal/service/peers.go), so this is how AddPeer/DeletePeer find
// who to push the updated interface config to.
func (s *Service) findServerByInterface(ifaceID uuid.UUID) (*models.Server, error) {
	servers, err := s.store.Servers().List()
	if err != nil {
		return nil, err
	}
	for i := range servers {
		for _, id := range servers[i].Interfaces {
			if id == ifaceID {
				return &servers[i], nil
			}
		}
	}
	return nil, errors.New("no server owns this interface")
}
