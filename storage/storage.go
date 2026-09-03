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

package storage

import (
	"io"
	"net"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

type Storage interface {
	io.Closer
	Users() Users
	Servers() Servers
	Auth() Auth
	AgentSources() AgentSources
	// Backup writes a consistent, restorable snapshot of the entire store to
	// w (see storage/boltdb/dump — the same format as the awg-migrate CLI).
	Backup(w io.Writer) error
}

// AgentSources holds the named, reusable agent-binary deploy presets (see
// models.AgentSource) — a flat collection, not scoped to any one server.
type AgentSources interface {
	List() ([]models.AgentSource, error)
	Get(id uuid.UUID) (*models.AgentSource, error)
	Set(src *models.AgentSource) error
	Delete(id uuid.UUID) error
}

// Auth holds the single admin account used by the standalone web-server's
// login flow (see internal/api). Not a collection like Users/Servers —
// there's exactly one record.
type Auth interface {
	Get() (*models.AuthCredentials, error)
	Set(creds *models.AuthCredentials) error
}

type Servers interface {
	List() ([]models.Server, error)
	Get(id uuid.UUID) (*models.Server, error)
	Set(srv *models.Server) error
	Delete(id uuid.UUID) error
	Interfaces(serverID uuid.UUID) Interfaces
}

type Interfaces interface {
	List() ([]models.Interface, error)
	Get(id uuid.UUID) (*models.Interface, error)
	Set(iface *models.Interface) error
	Delete(id uuid.UUID) error
	UsedIPs(ifaceID uuid.UUID) ([]net.IPNet, error)
	// Import creates iface — keeping its Peers list exactly as given, unlike Set
	// which wipes peers on create — together with the user-side peer records in
	// peers, all in ONE transaction: users are matched by exact name (created
	// when missing) and each peer is attached to the matching user with its
	// InterfaceId set to iface.ID. Nothing is written if any step fails. This is
	// the wg-quick import's write path (see internal/service PreviewImport /
	// ImportInterfaceFromServer); the caller has already validated that every
	// peer in peers has a matching entry in iface.Peers.
	Import(iface *models.Interface, peers []ImportPeer) error
}

// ImportPeer is one user-owned peer to create by Interfaces.Import: the name
// of the user it belongs to (matched exactly; created if absent) and the peer
// record itself. Peer.InterfaceId is overwritten with the imported interface's
// ID.
type ImportPeer struct {
	UserName string
	Peer     models.Peer
}

type Users interface {
	List() ([]models.User, error)
	Get(id uuid.UUID) (*models.User, error)
	Set(user *models.User) error
	Delete(id uuid.UUID) error
	Peers(userID uuid.UUID) Peers
}

type Peers interface {
	List() ([]models.Peer, error)
	Get(publicKey agentmodels.Key) (*models.Peer, error)
	Set(peer *models.Peer) error
	Delete(publicKey agentmodels.Key) error
}
