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

import "github.com/google/uuid"

// TunnelStep identifies one interface a tunnel is built from, given in order
// (entry first, exit last). Input to Service.BuildTunnel.
type TunnelStep struct {
	ServerID uuid.UUID `json:"serverId"`
	IfaceID  uuid.UUID `json:"ifaceId"`
}

// TunnelMember is one interface belonging to a tunnel, for display in the
// tunnel list. Role is derived from the interface's config ("entry" keeps its
// listen port and relays; "exit" has none and NATs).
type TunnelMember struct {
	ServerID   uuid.UUID `json:"serverId"`
	ServerName string    `json:"serverName"`
	IfaceID    uuid.UUID `json:"ifaceId"`
	Interface  string    `json:"interface"`
	Role       string    `json:"role"`
}

// Tunnel is the derived view of the interfaces sharing one Tunnel id. It is
// NOT persisted as its own entity — it's reconstructed from the Tunnel field
// on models.Interface (see Service.ListTunnels).
type Tunnel struct {
	ID      uuid.UUID      `json:"id"`
	Members []TunnelMember `json:"members"`
}
