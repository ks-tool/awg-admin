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

// Interface kinds as reported in HostInfo.InterfaceKinds and
// UnmanagedInterface.Kind — the OS link type of a WireGuard-family device.
const (
	InterfaceKindAmnezia   = "amneziawg"
	InterfaceKindWireGuard = "wireguard"
)

// UnmanagedInterface is a live WireGuard/AmneziaWG device on the agent's host
// that the agent has no stored config for — typically one brought up by
// wg-quick/awg-quick before the agent was installed (see GET
// /interfaces/unmanaged). Deliberately a separate, minimal type rather than a
// flag on InterfaceConfig: that struct is what the agent persists and the admin
// stores, so a "not managed" marker on it would stick to the record.
type UnmanagedInterface struct {
	// Name is the OS interface name (e.g. "wg0", "awg0").
	Name string `json:"name"`
	// Kind is the link type: InterfaceKindAmnezia when the device answers on the
	// AmneziaWG genetlink family (obfuscation-capable), else InterfaceKindWireGuard.
	Kind string `json:"kind"`
}

// ImportSource is a wg-quick/awg-quick configuration file read from the
// agent's host — the input for adopting an unmanaged interface into awg-admin
// (GET /interfaces/{name}/import-source). Content is the raw file text; Path
// says which of the candidate locations it was found at.
type ImportSource struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
