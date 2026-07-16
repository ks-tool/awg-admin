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
	"fmt"
	"net"

	"github.com/ks-tool/awg-admin/internal/deploy"
	"github.com/ks-tool/awg-admin/internal/pki"
	"github.com/ks-tool/awg-admin/internal/sshclient"
	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// ensureTunnel (re)opens the SSH tunnel for srv if it doesn't have mTLS
// configured, so a server added or edited after StartTunnels ran at process
// startup still gets one without requiring a restart. The resulting dial
// error (if any) is both logged and returned: most callers treat it as
// best-effort and ignore it (the server record itself was already saved
// successfully), but UpdateServer/UnlockServerSSH specifically check for a
// *sshclient.PassphraseRequiredError to prompt the user instead of letting
// it pass silently as a transient connectivity issue.
func (s *Service) ensureTunnel(srv *models.Server) error {
	servers := []models.Server{*srv}
	errs := s.tunnels.OpenAll(servers)
	s.syncTunnelledServers(servers, errs)
	if err, ok := errs[srv.ID]; ok {
		log.Warn().Err(err).Str("server_id", srv.ID.String()).Msg("failed to open agent tunnel")
		return err
	}
	return nil
}

// ServerInput carries all fields required/optional for server creation/update.
type ServerInput struct {
	Name  string
	Info  models.ServerInfo
	SSH   models.SSHConfig
	Agent models.Agent
}

func (s *Service) ListServers() ([]models.Server, error) {
	debugOp("ListServers").Msg("listing servers")
	return s.store.Servers().List()
}

func (s *Service) GetServer(id string) (*models.Server, error) {
	debugOp("GetServer").Str("server_id", id).Msg("getting server")
	sID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return s.store.Servers().Get(sID)
}

// ServerAgentStatus reports a tri-state health for id's agent, for the
// dashboard (see models.AgentStatus). It combines transport liveness (the SSH
// tunnel, or the direct mTLS path) with an actual probe of the agent's HTTP API
// (a lightweight List /interfaces/ via callAgent, which self-heals a dead SSH
// tunnel by reopening it):
//
//   - AgentStatusOK       (green):  the agent is reachable and answering — which
//     for an SSH server also means its tunnel is up.
//   - AgentStatusDown     (red):    the SSH tunnel could not be brought up, or an
//     mTLS agent is unreachable — the connection is
//     simply down.
//   - AgentStatusDegraded (amber):  the SSH tunnel is up but the agent behind it
//     isn't answering, or the state is indeterminate.
//
// Any non-OK state is logged at error level (with server_id) so problems are
// visible in the logs. This is a passive check: reconnect/passphrase failures
// collapse into the status rather than surfacing as an error (or a passphrase
// prompt) to the caller.
func (s *Service) ServerAgentStatus(id string) (models.AgentStatus, error) {
	sID, err := uuid.Parse(id)
	if err != nil {
		return "", err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return "", err
	}

	mtls := !srv.Agent.TLS.IsZero()

	// For SSH servers the tunnel must be up before the agent can be reached at
	// all; a tunnel that won't open is "down" (red), distinct from a live tunnel
	// with a silent agent (degraded).
	if !mtls {
		if _, err := s.tunnels.Ensure(sID, srv.SSH, srv.Agent.Address); err != nil {
			log.Error().Err(err).Str("server_id", id).Msg("agent status: SSH tunnel to agent is down")
			return models.AgentStatusDown, nil
		}
	}

	// Transport is available (mTLS direct, or a live SSH tunnel) — probe the
	// agent's HTTP API to confirm the daemon actually answers.
	if err := s.pingAgent(srv); err != nil {
		if mtls {
			// mTLS is reached directly; a failed probe means the agent is unreachable.
			log.Error().Err(err).Str("server_id", id).Msg("agent status: mTLS agent unreachable")
			return models.AgentStatusDown, nil
		}
		// SSH: the pre-check saw a live tunnel, but a probe failing with a
		// dropped-connection error (EOF/closed/deadline) means the tunnel died
		// underneath us — Alive() merely lagged. That's "tunnel down" (red), not
		// a silent agent behind a genuinely live tunnel (degraded).
		if tunnelDropped(err) {
			log.Error().Err(err).Str("server_id", id).Msg("agent status: SSH tunnel to agent died")
			return models.AgentStatusDown, nil
		}
		log.Error().Err(err).Str("server_id", id).Msg("agent status: SSH tunnel up but agent not responding")
		return models.AgentStatusDegraded, nil
	}

	return models.AgentStatusOK, nil
}

func (s *Service) CreateServer(in ServerInput) (*models.Server, error) {
	debugOp("CreateServer").Str("name", in.Name).Msg("creating server")
	if len(in.Name) == 0 {
		return nil, fmt.Errorf("server name is required")
	}
	if err := in.SSH.Validate(); err != nil {
		return nil, err
	}

	srv := &models.Server{
		ID:    uuid.New(),
		Name:  in.Name,
		Info:  in.Info,
		SSH:   in.SSH,
		Agent: in.Agent,
	}
	if err := s.store.Servers().Set(srv); err != nil {
		return nil, err
	}
	// A freshly created server has nothing on it yet (no interfaces/peers to
	// push, no agent necessarily deployed there), so unlike UpdateServer
	// there's no reason to dial it right away — that only produces a
	// spurious "dial ... i/o timeout" warning when the agent isn't installed
	// yet. The first tunnel attempt happens on the next admin restart
	// (StartTunnels) or once an interface/agent action actually needs it.
	return srv, nil
}

func (s *Service) UpdateServer(id string, in ServerInput) (*models.Server, error) {
	debugOp("UpdateServer").Str("server_id", id).Str("name", in.Name).Msg("updating server")
	sID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	if len(in.Name) == 0 {
		return nil, fmt.Errorf("server name is required")
	}

	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	// The edit form sends only the one credential the user actually entered
	// (Key path, uploaded KeyData, or Password), never re-sending unchanged
	// secrets. Treat *which* one is present as the intended auth method and
	// clear the others, so switching key⇄password actually drops the previous
	// credential instead of leaving both set — with both set, Dial prefers the
	// key (see sshclient.authMethod), so a switch to password would keep
	// failing on the stale key ("attempted methods [publickey]"). When the user
	// entered nothing, this is an unrelated edit (e.g. the SSH port): preserve
	// every stored credential as-is, so validation doesn't fail "SSH auth
	// required" and the auth method stays untouched. Runs before validation.
	switch {
	case len(in.SSH.KeyData) > 0:
		in.SSH.Key, in.SSH.Password = "", ""
	case len(in.SSH.Key) > 0:
		in.SSH.KeyData, in.SSH.Password = "", ""
	case len(in.SSH.Password) > 0:
		in.SSH.Key, in.SSH.KeyData = "", ""
	default:
		in.SSH.Key = srv.SSH.Key
		in.SSH.KeyData = srv.SSH.KeyData
		in.SSH.Password = srv.SSH.Password
	}

	if err = in.SSH.Validate(); err != nil {
		return nil, err
	}

	// MonitoringDisabled and ProfilingEnabled are agent *desired-state* flags
	// owned exclusively by the dedicated toggles (SetServerMonitoring /
	// SetServerProfiling), never the edit form — which doesn't round-trip them,
	// so in.Agent always carries their zero value. Preserve the stored values
	// through this wholesale Agent replace, or an unrelated edit would silently
	// re-enable monitoring / turn profiling back off (and stop SyncServer from
	// re-applying them).
	in.Agent.MonitoringDisabled = srv.Agent.MonitoringDisabled
	in.Agent.ProfilingEnabled = srv.Agent.ProfilingEnabled

	srv.Name = in.Name
	srv.Info = in.Info
	srv.SSH = in.SSH
	srv.Agent = in.Agent
	if err = s.store.Servers().Set(srv); err != nil {
		return nil, err
	}

	if err = s.ensureTunnel(srv); err != nil {
		var passErr *sshclient.PassphraseRequiredError
		if errors.As(err, &passErr) {
			return nil, err
		}
		// any other dial error (host unreachable, etc.) is already logged by
		// ensureTunnel and shouldn't block the save that just succeeded.
	}
	return srv, nil
}

// UnlockServerSSH caches passphrase as the passphrase for the server's SSH
// key for the remainder of the process's lifetime (never written to
// storage — see sshclient.Manager.SetPassphrase) and immediately retries
// opening its tunnel. When applyToAll is true, the passphrase also becomes
// the fallback tried for any other server whose key turns out to need one,
// so a single entry covers every server sharing the same key/passphrase for
// the session.
func (s *Service) UnlockServerSSH(id string, passphrase string, applyToAll bool) error {
	debugOp("UnlockServerSSH").Str("server_id", id).Bool("apply_to_all", applyToAll).Msg("unlocking server SSH")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}
	s.tunnels.SetPassphrase(sID, passphrase, applyToAll)
	return s.ensureTunnel(srv)
}

// UnlockServerSudo caches password as the sudo password for the server's SSH
// user for the remainder of the process's lifetime (never persisted — see
// sshclient.Manager.SetSudoPassword), so the next DeployAgent for this server
// can escalate a non-root user's privileged commands. When applyToAll is true
// the password also becomes the fallback for any other server whose host turns
// out to need a sudo password. Unlike UnlockServerSSH it doesn't reconnect
// anything — the caller simply retries the deploy, which reads the cached
// password (see Service.DeployAgent / the AgentModal retry flow).
func (s *Service) UnlockServerSudo(id string, password string, applyToAll bool) error {
	debugOp("UnlockServerSudo").Str("server_id", id).Bool("apply_to_all", applyToAll).Msg("caching server sudo password")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	if _, err := s.store.Servers().Get(sID); err != nil {
		return err
	}
	s.tunnels.SetSudoPassword(sID, password, applyToAll)
	return nil
}

// GenerateAgentTLS (re)issues a CA, server and client certificate for the
// server's agent and stores them on the server record. The server cert's
// SAN is derived from Agent.Address so it matches whatever "white" IP or
// hostname the agent will be reached on directly (without an SSH tunnel).
func (s *Service) GenerateAgentTLS(id string) (*models.Server, error) {
	debugOp("GenerateAgentTLS").Str("server_id", id).Msg("generating agent TLS material")
	sID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	host := srv.Agent.Address
	if h, _, splitErr := net.SplitHostPort(host); splitErr == nil {
		host = h
	}
	if len(host) == 0 {
		return nil, fmt.Errorf("agent address is required to issue TLS certificates")
	}

	var ips []net.IP
	var dnsNames []string
	if ip := net.ParseIP(host); ip != nil {
		ips = append(ips, ip)
	} else {
		dnsNames = append(dnsNames, host)
	}

	tlsMaterial, err := pki.IssueAgentTLS(srv.Name, ips, dnsNames)
	if err != nil {
		return nil, fmt.Errorf("issue agent TLS material: %w", err)
	}

	srv.Agent.TLS = tlsMaterial
	return srv, s.store.Servers().Set(srv)
}

// DeployAgent starts installing and starting the awg-agent service on the
// server's host over SSH, using its currently stored SSH credentials,
// agent address and (if generated) mTLS certificates, returning as soon as
// the deploy has started rather than waiting for it to finish — it can
// take a while (e.g. downloading a large binary), so the actual work runs
// in the background; poll GetDeployStatus(id) for progress and the
// outcome. agentSourceID picks which saved models.AgentSource preset to
// fetch the binary from (see Service.CreateAgentSource). Only the
// up-front validation (server/source must exist) is synchronous — any
// error returned here means the deploy never started at all.
func (s *Service) DeployAgent(id string, agentSourceID string) error {
	debugOp("DeployAgent").Str("server_id", id).Str("agent_source_id", agentSourceID).Msg("deploying agent")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}

	asID, err := uuid.Parse(agentSourceID)
	if err != nil {
		return fmt.Errorf("invalid agent source id: %w", err)
	}
	src, err := s.store.AgentSources().Get(asID)
	if err != nil {
		return fmt.Errorf("agent source: %w", err)
	}

	// The docker deploy publishes each interface's WireGuard ListenPort as a UDP
	// port on the container; gather them from the desired state we already have.
	// (Ignored by the systemd deploy, which runs on the host network directly.)
	var udpPorts []uint16
	if ifaces, e := s.store.Servers().Interfaces(sID).List(); e == nil {
		for i := range ifaces {
			if ifaces[i].ListenPort > 0 {
				udpPorts = append(udpPorts, ifaces[i].ListenPort)
			}
		}
	}

	s.deployStatus.start(sID)
	go func() {
		err := deploy.ToAgent(context.Background(), *srv, *src, udpPorts, s.tunnels.PassphraseFor(sID), s.tunnels.SudoPasswordFor(sID), func(step string) {
			s.deployStatus.setStep(sID, step)
		})
		s.deployStatus.finish(sID, err)
		if err != nil {
			return
		}

		// A freshly (re)deployed agent starts with no interfaces of its
		// own — push the desired state we already have for this server
		// right away instead of waiting for the next unrelated edit to
		// trigger a push.
		if err := s.SyncServer(id); err != nil {
			log.Warn().Err(err).Str("server_id", id).Msg("failed to sync server after deploying agent")
		}
	}()
	return nil
}

// GetDeployStatus returns the most recent DeployAgent run's progress for a
// server, polled by the frontend while its "Deploy agent" modal is open.
// Returns storage.ErrNotFound if no deploy has been started for this
// server since the process started (status is in-memory only, see
// deployStatusStore).
func (s *Service) GetDeployStatus(id string) (*models.DeployStatus, error) {
	sID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	st, ok := s.deployStatus.get(sID)
	if !ok {
		return nil, storage.ErrNotFound
	}
	return &st, nil
}

func (s *Service) DeleteServer(id string) error {
	debugOp("DeleteServer").Str("server_id", id).Msg("deleting server")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	// Refuse if any of the server's interfaces is part of a tunnel — the
	// tunnel's other end lives on a different server, so it must be removed
	// first (mirrors the per-interface delete guard in DeleteInterface).
	if ifaces, e := s.store.Servers().Interfaces(sID).List(); e == nil {
		for i := range ifaces {
			if ifaces[i].Tunnel != nil {
				return fmt.Errorf("server has interface %q that is part of a tunnel; remove the tunnel first", ifaces[i].Interface)
			}
		}
	}
	return s.store.Servers().Delete(sID)
}
