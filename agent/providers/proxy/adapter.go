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

package proxy

import (
	"encoding/json"

	"github.com/ks-tool/vpn-agent/models"
)

type PeerSpec struct {
	Name         string `json:"name" jsonschema:"required,minLength=1"`
	InterfaceRef string `json:"interfaceRef" jsonschema:"required,minLength=1"`
	UserRef      string `json:"userRef,omitempty"`
	PublicKey    string `json:"publicKey,omitempty"`
	Disabled     bool   `json:"disabled,omitempty"`
}

type Peer struct {
	Resource models.Resource
	Spec     PeerSpec
	Secret   string
}

type CoreConfig struct {
	Data      []byte
	Binary    string
	Arguments func(string) []string
}

type Route struct {
	LinkID string
	SOCKS5 string
}

type LinkAdapter interface {
	PrepareLink(json.RawMessage, models.Link, models.LinkEndpoint, string, string) (Peer, models.RouteDriver, error)
}

type Adapter interface {
	Name() string
	APIVersion() string
	FirewallProtocol() string
	InterfaceSchema() any
	InterfaceUISchema() json.RawMessage
	ValidateInterface(json.RawMessage) error
	NewSecret() (string, error)
	ValidateSecret(string) error
	RenderServer(json.RawMessage, []Peer, []Route, string) (CoreConfig, error)
	RenderClient(json.RawMessage, Peer, string) (string, error)
}
