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

package sshclient

import (
	"fmt"
	"sync"

	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// Manager holds one TunnelClient per server that doesn't have mTLS
// configured — those reach their agent exclusively through an SSH tunnel,
// so awg-admin keeps the underlying SSH connection open for the lifetime of
// the process instead of dialing per request. Servers with mTLS configured
// are not tracked here; they're reached directly (see models.AgentTLS).
type Manager struct {
	mu      sync.RWMutex
	clients map[uuid.UUID]*TunnelClient

	// passphrases caches SSH key passphrases in memory for the lifetime of
	// the process ("for the duration of the session") — never persisted to
	// storage. globalPassphrase, when set, is the fallback tried for any
	// server whose key needs a passphrase but has none cached yet, so one
	// "use for all connections" entry can cover several servers that share
	// the same key/passphrase.
	passphrases      map[uuid.UUID]string
	globalPassphrase string
	hasGlobal        bool
}

// NewManager returns an empty Manager. Call Open for each server to
// populate it (typically done once at startup via OpenAll).
func NewManager() *Manager {
	return &Manager{
		clients:     make(map[uuid.UUID]*TunnelClient),
		passphrases: make(map[uuid.UUID]string),
	}
}

// SetPassphrase caches passphrase as serverID's SSH key passphrase, used by
// every future (re)connect attempt for that server without prompting the
// user again. When applyToAll is true, passphrase also becomes the fallback
// tried for any other server's key that turns out to need one.
func (m *Manager) SetPassphrase(serverID uuid.UUID, passphrase string, applyToAll bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.passphrases[serverID] = passphrase
	if applyToAll {
		m.globalPassphrase = passphrase
		m.hasGlobal = true
	}
}

// PassphraseFor returns the cached passphrase for serverID, falling back to
// the "use for all connections" passphrase (if any) when none was cached
// specifically for this server.
func (m *Manager) PassphraseFor(serverID uuid.UUID) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if p, ok := m.passphrases[serverID]; ok {
		return p
	}
	if m.hasGlobal {
		return m.globalPassphrase
	}
	return ""
}

// OpenAll dials an SSH tunnel for every server in servers that has a valid
// SSH config and no mTLS configured, replacing any existing tunnel for that
// server ID. It returns the per-server dial errors (if any) keyed by server
// ID rather than failing outright, since one unreachable server shouldn't
// block startup for the rest.
func (m *Manager) OpenAll(servers []models.Server) map[uuid.UUID]error {
	errs := make(map[uuid.UUID]error)
	for _, srv := range servers {
		if !srv.Agent.TLS.IsZero() {
			continue // reached directly over mTLS, no tunnel needed
		}
		if err := srv.SSH.Validate(); err != nil {
			errs[srv.ID] = fmt.Errorf("skipping tunnel: %w", err)
			continue
		}
		if err := m.Open(srv.ID, srv.SSH, srv.Agent.Address); err != nil {
			errs[srv.ID] = err
		}
	}
	return errs
}

// Open dials sshCfg and stores the resulting tunnel client under serverID,
// closing any previously open tunnel for that server first.
func (m *Manager) Open(serverID uuid.UUID, sshCfg models.SSHConfig, agentAddr string) error {
	client, err := Dial(sshCfg, m.PassphraseFor(serverID))
	if err != nil {
		return fmt.Errorf("server %s: %w", serverID, err)
	}
	tunnel := NewTunnelClient(client, agentAddr)

	m.mu.Lock()
	if old, ok := m.clients[serverID]; ok {
		_ = old.Close()
	}
	m.clients[serverID] = tunnel
	m.mu.Unlock()
	return nil
}

// Get returns the tunnel client for serverID, if one is open.
func (m *Manager) Get(serverID uuid.UUID) (*TunnelClient, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	t, ok := m.clients[serverID]
	return t, ok
}

// IsOpen reports whether serverID currently has a live SSH tunnel. Servers
// reached directly via mTLS are never tracked here (see OpenAll), so this
// always reports false for them regardless of their actual reachability.
func (m *Manager) IsOpen(serverID uuid.UUID) bool {
	_, ok := m.Get(serverID)
	return ok
}

// Ensure returns a working tunnel client for serverID, lazily (re)dialing
// one if none is cached yet or the cached one's underlying SSH connection
// has actually died — e.g. because the remote agent's host rebooted, sshd
// restarted, or the connection was silently dropped by the network. Open
// only ever replaces an entry on a successful *new* dial (see Open), so
// without this, a tunnel that died after being opened would stay cached
// forever and every call through it would keep failing the same way.
func (m *Manager) Ensure(serverID uuid.UUID, sshCfg models.SSHConfig, agentAddr string) (*TunnelClient, error) {
	if t, ok := m.Get(serverID); ok && t.Alive() {
		return t, nil
	}
	if err := m.Open(serverID, sshCfg, agentAddr); err != nil {
		return nil, err
	}
	t, _ := m.Get(serverID)
	return t, nil
}

// Close tears down every open tunnel.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.clients {
		if err := t.Close(); err != nil {
			return fmt.Errorf("close tunnel for server %s: %w", id, err)
		}
	}
	m.clients = make(map[uuid.UUID]*TunnelClient)
	return nil
}
