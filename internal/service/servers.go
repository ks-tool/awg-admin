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

func (s *Service) ListServers() ([]models.Server, error) { return s.store.Servers().List() }

func (s *Service) GetServer(id string) (*models.Server, error) {
	sID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	return s.store.Servers().Get(sID)
}

// ServerTunnelOpen reports whether id currently has a live SSH tunnel open
// (see sshclient.Manager), lazily (re)dialing one first if the cached entry
// turned out to be dead — so polling this from the frontend is enough to
// self-heal after the remote agent/sshd restarts, without requiring a
// manual Sync or an admin process restart. Reconnect failures (e.g. a
// passphrase the user hasn't supplied yet) are reported as "not open"
// rather than as an error — this is a passive status check, not an action,
// so it shouldn't surface a passphrase prompt on its own. Servers
// configured with mTLS never have a tunnel — they're reached directly — so
// this always reports false for them; callers should check
// models.Server.Agent.TLS to tell "no tunnel needed" apart from "tunnel
// down".
func (s *Service) ServerTunnelOpen(id string) (bool, error) {
	sID, err := uuid.Parse(id)
	if err != nil {
		return false, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return false, err
	}
	if !srv.Agent.TLS.IsZero() {
		return false, nil
	}
	_, err = s.tunnels.Ensure(sID, srv.SSH, srv.Agent.Address)
	return err == nil, nil
}

func (s *Service) CreateServer(in ServerInput) (*models.Server, error) {
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

	// The edit form only sends a non-empty Key/KeyData/Password when the user
	// actually wants to change SSH credentials; preserve the existing ones
	// otherwise instead of wiping them out on unrelated edits. A newly
	// uploaded KeyData supersedes any previously stored Key path (and vice
	// versa) rather than leaving both set. This merge must happen before
	// validation, since an edit that only touches an unrelated field (e.g.
	// SSH port) arrives with empty Key/KeyData/Password and would otherwise
	// fail "SSH auth required" despite the server already having valid
	// stored credentials.
	if len(in.SSH.KeyData) > 0 {
		in.SSH.Key = ""
	} else if len(in.SSH.Key) == 0 {
		in.SSH.Key = srv.SSH.Key
		in.SSH.KeyData = srv.SSH.KeyData
	}
	if len(in.SSH.Password) == 0 {
		in.SSH.Password = srv.SSH.Password
	}

	if err = in.SSH.Validate(); err != nil {
		return nil, err
	}

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

// GenerateAgentTLS (re)issues a CA, server and client certificate for the
// server's agent and stores them on the server record. The server cert's
// SAN is derived from Agent.Address so it matches whatever "white" IP or
// hostname the agent will be reached on directly (without an SSH tunnel).
func (s *Service) GenerateAgentTLS(id string) (*models.Server, error) {
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

	s.deployStatus.start(sID)
	go func() {
		err := deploy.ToAgent(context.Background(), *srv, *src, s.tunnels.PassphraseFor(sID), func(step string) {
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
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.store.Servers().Delete(sID)
}
