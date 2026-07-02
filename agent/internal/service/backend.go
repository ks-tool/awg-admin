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

// Backend abstracts the OS-level lifecycle of a WireGuard/AmneziaWG interface —
// the one part of the agent that differs between the AmneziaWG kernel module
// (amneziawg-dkms, driven over netlink) and the userspace implementation
// (amneziawg-go, driven as a child process). Everything else the agent does is
// backend-agnostic and stays as-is:
//
//   - pushing the device config and reading peer stats via wgctrl, which already
//     speaks both the kernel genl family and the userspace UAPI socket;
//   - running PreUp/PostUp/PreDown/PostDown hooks (plain `sh -c`);
//   - storing interface configs (agent/storage).
//
// The address / MTU / up-down operations are plain netlink on an existing link,
// and a userspace TUN behaves identically to a kernel wg link there — so a
// userspace Backend can embed NetlinkBackend and override only Add/Delete/Exists
// (how the link comes into existence and goes away), reusing the rest.
type Backend interface {
	// Add brings the interface's OS link into existence. Kernel: netlink
	// RTM_NEWLINK (type "amneziawg"/"wireguard"). Userspace: start the
	// amneziawg-go process that creates the TUN.
	Add(iface string) error
	// Delete removes the OS link. Kernel: netlink RTM_DELLINK. Userspace: stop
	// the amneziawg-go process. Idempotent: a link that's already gone is not an
	// error.
	Delete(iface string) error
	// Exists reports whether the interface's OS link is present.
	Exists(iface string) bool
	// Up brings the link administratively up.
	Up(iface string) error
	// Down brings the link administratively down.
	Down(iface string) error
	// SetMTU sets the link MTU.
	SetMTU(iface string, mtu int) error
	// AddrAdd assigns addr (a CIDR like 10.0.0.1/24) to the link.
	AddrAdd(iface, addr string) error
	// SyncAddr makes addr the link's only address, removing any others first.
	SyncAddr(iface, addr string) error
}

// backend is the active link backend for this process. It defaults to the
// kernel (AmneziaWG-dkms) backend; a userspace build swaps it out at startup
// via SetBackend. A process serves exactly one backend for its whole lifetime,
// so a package-level value — set once before interfaces are loaded — is all
// this needs (the agent is a single-purpose, single-process daemon).
var backend Backend = NetlinkBackend{}

// SetBackend selects the link backend for this process. Call it once, early in
// main (before loading interfaces) — e.g. a userspace build wiring in an
// amneziawg-go backend. Not safe to call concurrently with interface
// operations.
func SetBackend(b Backend) { backend = b }
