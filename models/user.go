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
	agentmodels "github.com/ks-tool/awg-admin/agent/models"

	"github.com/google/uuid"
)

type User struct {
	ID          uuid.UUID `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Disabled    bool      `json:"disabled,omitempty"`
	Peers       []Peer    `json:"peers,omitempty"`
}

type Peer struct {
	Name        string          `json:"name"`
	PrivateKey  agentmodels.Key `json:"pk"`
	InterfaceId uuid.UUID       `json:"interface"`
	Disabled    bool            `json:"disabled,omitempty"`
	// DNS is the client-side DNS for this peer's generated wg-quick config
	// (the `[Interface] DNS = …` line). Admin-only: it's never pushed to the
	// agent (DNS is a client setting). Empty falls back to the owning
	// interface's DNS in GetPeerConfig.
	DNS []string `json:"dns,omitempty"`
}
