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
// the one part of the agent that differs between the two builds:
//
//   - kernel (agent/internal/kernel): the AmneziaWG kernel module, driven over
//     netlink (vishvananda/netlink). Wired in by cmd/awg-agent.
//   - userspace (agent/internal/userspace): an in-process amneziawg-go TUN,
//     with address/up/mtu done via the `ip` command (no netlink dependency, the
//     way jwg / awg-quick's userspace path do it). Wired in by
//     cmd/awg-agent-userspace.
//
// Keeping this an interface (rather than either build importing the other) is
// what lets the userspace agent avoid pulling in vishvananda/netlink at all.
// Everything else the agent does is backend-agnostic:
//
//   - pushing the device config and reading peer stats via wgctrl, which speaks
//     both the kernel genl family and the userspace UAPI socket;
//   - running PreUp/PostUp/PreDown/PostDown hooks (plain `sh -c`);
//   - storing interface configs (agent/storage).
type Backend interface {
	// Add brings the interface's OS link into existence. amnezia selects the
	// interface type: true creates an AmneziaWG link (obfuscation-capable),
	// false a plain WireGuard one — driven by the admin's "Amnezia Interface"
	// toggle (see models.InterfaceConfig.IsAmnezia). Kernel: netlink RTM_NEWLINK
	// with the matching kind. Userspace: the amneziawg-go device serves both, so
	// the TUN is created the same way regardless.
	Add(iface string, amnezia bool) error
	// Delete removes the OS link. Kernel: netlink RTM_DELLINK. Userspace: close
	// the amneziawg-go device (which removes the TUN). Idempotent: a link that's
	// already gone is not an error.
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
	// Info reports what this backend can create on the current host — its kind,
	// the interface variants it can bring up, and (kernel only) whether the
	// AmneziaWG kernel module is present. Probed on each call (it shells out to
	// modinfo for the kernel backend), so the agent calls it once at startup and
	// caches the result (see agent.Run → models.HostInfo).
	Info() BackendInfo
}

// BackendInfo describes what the active link backend can create on the current
// host. Gathered once at startup (Backend.Info) and folded into the agent's
// GET /info response (models.HostInfo). Host-level facts that don't depend on
// the backend — whether Docker is usable, whether the agent runs in a container
// — are discovered separately by the agent package, not here.
type BackendInfo struct {
	// Kind identifies the backend: "kernel" or "userspace".
	Kind string
	// KernelModule reports whether the AmneziaWG kernel module is available on
	// this host. Always false for the userspace backend (it needs no module).
	KernelModule bool
	// InterfaceKinds lists the interface variants creatable here: "amneziawg"
	// and/or "wireguard".
	InterfaceKinds []string
}

// backend is the active link backend for this process. There is no default: a
// process serves exactly one backend, and its main MUST select it via
// SetBackend before loading interfaces (cmd/awg-agent → kernel,
// cmd/awg-agent-userspace → userspace). Kept out of this package so `service`
// itself stays free of any particular backend's dependencies (notably netlink).
var backend Backend

// SetBackend selects the link backend for this process. Call it once, early in
// main, before loading interfaces. Not safe to call concurrently with interface
// operations.
func SetBackend(b Backend) { backend = b }

// ActiveBackendInfo returns the active backend's capabilities (see BackendInfo).
// SetBackend must have been called first.
func ActiveBackendInfo() BackendInfo { return backend.Info() }
