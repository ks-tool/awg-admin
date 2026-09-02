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

// Interface kinds as reported by the agent (HostInfo.InterfaceKinds and
// UnmanagedInterface.Kind) — the OS link type of a WireGuard-family device.
const (
	InterfaceKindAmnezia   = "amneziawg"
	InterfaceKindWireGuard = "wireguard"
)

// UnmanagedInterface is a live WireGuard/AmneziaWG device on a server that
// its agent has no stored config for — typically brought up by wg-quick/
// awg-quick before the agent was installed. The admin-side twin of the agent's
// models.UnmanagedInterface (same JSON shape; kept here so the admin builds
// against the published agent module without a local replace). Read-only in
// the UI: the way to take one over is the wg-quick import (ImportPreview).
type UnmanagedInterface struct {
	// Name is the OS interface name (e.g. "wg0", "awg0").
	Name string `json:"name"`
	// Kind is the link type: InterfaceKindAmnezia or InterfaceKindWireGuard.
	Kind string `json:"kind"`
}

// ImportSource is a wg-quick/awg-quick conf file read from a server by its
// agent (GET /interfaces/{name}/import-source) — the admin-side twin of the
// agent's models.ImportSource.
type ImportSource struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// ImportPreview is the dry-run report of importing a wg-quick/awg-quick
// interface from a server (Service.PreviewImport): what the import would
// create, what it translated, and everything worth a second look before
// confirming. It carries no private keys — the peer map the admin typed
// already holds those, and the interface key never needs to round-trip.
type ImportPreview struct {
	// Interface is the OS interface name being imported.
	Interface string `json:"interface"`
	// Source is the conf file path on the server the config was read from.
	Source string `json:"source"`
	// Live reports whether a link with this name is currently up on the server
	// (the import then adopts it in place); false means the conf exists on
	// disk but the interface is down, so the agent will create it.
	Live bool `json:"live"`
	// LiveKind is the live link's type (InterfaceKindAmnezia/InterfaceKindWireGuard)
	// when Live, else empty.
	LiveKind string `json:"liveKind,omitempty"`

	Address    string   `json:"address"`
	ListenPort uint16   `json:"listenPort"`
	Amnezia    bool     `json:"amnezia"`
	MTU        int      `json:"mtu,omitempty"`
	DNS        []string `json:"dns,omitempty"`
	Table      int      `json:"table,omitempty"`

	// Users lists the users that will own imported peers — existing ones matched
	// by exact name, missing ones created — with the peers assigned to each.
	Users []ImportPreviewUser `json:"users"`
	// EmbeddedPeers are the conf's [Peer] sections the peer map didn't cover:
	// imported as interface peers without a user (the client keeps working, but
	// no config/QR can be rendered for it until it's re-issued).
	EmbeddedPeers []ImportPreviewPeer `json:"embeddedPeers"`

	// Hooks are the conf's own PreUp/PostUp/PreDown/PostDown, carried over
	// verbatim (with %i as wg-quick expands it).
	Hooks ImportPreviewHooks `json:"hooks"`
	// GeneratedHooks are the commands the import adds to reproduce what
	// wg-quick did implicitly — routes for AllowedIPs outside the interface
	// subnet, and the Table=N routes — appended after Hooks.
	GeneratedHooks ImportPreviewHooks `json:"generatedHooks"`

	// Warnings are non-blocking findings (ignored keys, non-idempotent hooks,
	// peers without keepalive, link-kind mismatches, …), in plain English.
	Warnings []string `json:"warnings"`
}

// ImportPreviewUser is one user in an ImportPreview.
type ImportPreviewUser struct {
	Name string `json:"name"`
	// Exists is true when a user of this name is already in the database (the
	// peers are added to it), false when the import creates the user.
	Exists bool                `json:"exists"`
	Peers  []ImportPreviewPeer `json:"peers"`
}

// ImportPreviewPeer is one peer in an ImportPreview, identified by public key.
type ImportPreviewPeer struct {
	// Name is the display name from the peer map; empty for an embedded peer.
	Name       string   `json:"name,omitempty"`
	PublicKey  string   `json:"publicKey"`
	AllowedIPs []string `json:"allowedIPs"`
	Endpoint   string   `json:"endpoint,omitempty"`
	// Keepalive is the PersistentKeepalive in seconds, 0 when the conf had none.
	Keepalive int `json:"keepalive"`
	// PresharedKey reports whether the peer has a PSK (the key itself isn't shown).
	PresharedKey bool `json:"presharedKey"`
}

// ImportPreviewHooks groups lifecycle hook commands by phase.
type ImportPreviewHooks struct {
	PreUp    []string `json:"preUp,omitempty"`
	PostUp   []string `json:"postUp,omitempty"`
	PreDown  []string `json:"preDown,omitempty"`
	PostDown []string `json:"postDown,omitempty"`
}
