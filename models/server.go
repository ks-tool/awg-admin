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

package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	agentmodels "github.com/ks-tool/awg-admin/agent/models"
)

type Server struct {
	ID   uuid.UUID  `json:"id"`
	Name string     `json:"name"`
	Info ServerInfo `json:"info"`

	SSH   SSHConfig `json:"ssh"`
	Agent Agent     `json:"agent"`

	Interfaces []uuid.UUID `json:"interfaces,omitempty"`
}

type ServerInfo struct {
	Description string   `json:"description,omitempty"`
	Location    string   `json:"location,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type SSHConfig struct {
	Host string `json:"host"`
	Port uint16 `json:"port,omitempty"`
	User string `json:"user,omitempty"`
	// Key is a path to the private key file on the machine running awg-admin,
	// not the key body itself. Mutually exclusive with KeyData; KeyData takes
	// precedence when both are set (see internal/sshclient).
	Key string `json:"key,omitempty"`
	// KeyData holds an uploaded private key's raw PEM content, stored
	// directly in the database instead of referencing a path on disk.
	KeyData  string `json:"keyData,omitempty"`
	Password string `json:"password,omitempty"`
}

type Agent struct {
	Address string    `json:"addr"`
	TLS     *AgentTLS `json:"tls,omitempty"`

	// MonitoringDisabled, when true, turns off the agent's CPU/RAM/network/
	// peer metrics collection (see Service.SetServerMonitoring). Zero value
	// (false) keeps monitoring enabled, matching the agent's own default.
	MonitoringDisabled bool `json:"monitoringDisabled,omitempty"`
}

// AgentStatus is the tri-state health of a server's agent shown on the
// dashboard (see Service.ServerAgentStatus), color-coded on the frontend:
// green / red / amber.
type AgentStatus string

const (
	// AgentStatusOK (green): the agent is reachable and answering — which for an
	// SSH-tunnelled server also means its tunnel is up.
	AgentStatusOK AgentStatus = "ok"
	// AgentStatusDown (red): the connection to the agent is down — the SSH
	// tunnel could not be brought up, or an mTLS agent (reached directly) is
	// unreachable.
	AgentStatusDown AgentStatus = "down"
	// AgentStatusDegraded (amber): the SSH tunnel is up but the agent behind it
	// isn't responding, or the state is otherwise indeterminate.
	AgentStatusDegraded AgentStatus = "degraded"
)

// AgentTLS holds the mTLS material for direct (non-tunnelled) communication
// with the agent on a public ("white") IP. CA is kept so awg-admin can
// re-issue Server/Client certs later (e.g. on renewal) without invalidating
// the other side. Server is deployed to the agent; Client is used by
// awg-admin itself when dialing the agent directly.
type AgentTLS struct {
	CA     CertKeyPair `json:"ca"`
	Server CertKeyPair `json:"server"`
	Client CertKeyPair `json:"client"`
}

type CertKeyPair struct {
	Certificate string `json:"cert"`
	PrivateKey  string `json:"pk"`
}

func (t *AgentTLS) IsZero() bool {
	return t == nil || len(t.CA.Certificate) == 0
}

type Interface struct {
	ID uuid.UUID `json:"id"`

	// json:",inline" flattens the fields under jsonv2 (this module builds with
	// GOEXPERIMENT=jsonv2), matching the YAML "inline" convention. tstype
	// additionally tells tygo to emit "extends InterfaceConfig" on the
	// generated TS type instead of a nested field — tygo only understands
	// plain v1 json tag semantics and doesn't infer this on its own.
	agentmodels.InterfaceConfig `json:",inline" tstype:",extends"`

	// Sync status of this interface's config against the agent. Updated
	// after every push attempt (see internal/service's agent push wiring);
	// not touched by the agent itself.
	InSync        bool      `json:"inSync"`
	LastSyncError string    `json:"lastSyncError,omitempty"`
	LastSyncedAt  time.Time `json:"lastSyncedAt,omitempty"`

	// Tunnel groups the interfaces that together form one multi-hop tunnel
	// (see Service.BuildTunnel / the tunnel wizard): every interface a tunnel
	// is built from carries the same id, nil means "not part of any tunnel".
	// Admin-only bookkeeping — never pushed to the agent (only the embedded
	// InterfaceConfig is), not shown or editable in the UI. Its only visible
	// effects: it blocks deleting the interface while the tunnel exists, and
	// the dashboard counts distinct tunnel ids.
	Tunnel *uuid.UUID `json:"tunnel,omitempty"`
}

func (cfg SSHConfig) IsZero() bool {
	return len(cfg.Host) == 0
}

func (cfg SSHConfig) Validate() error {
	if len(cfg.Host) == 0 {
		return fmt.Errorf("SSH host is required")
	}
	if len(cfg.User) == 0 {
		return fmt.Errorf("SSH user is required")
	}
	if len(cfg.Key) == 0 && len(cfg.KeyData) == 0 && len(cfg.Password) == 0 {
		return fmt.Errorf("SSH auth required: provide key or password")
	}
	return nil
}
