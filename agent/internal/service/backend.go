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

// Backend abstracts the OS-level lifecycle of a WireGuard/AmneziaWG interface.
// The AWG provider selects the kernel implementation when a suitable module is
// available and the amneziawg-go userspace implementation otherwise. Userspace
// can also be forced with --userspace.
//
// Device configuration and peer statistics use wgctrl over either the kernel
// genl family or the userspace UAPI socket. Lifecycle hooks and interface
// storage are identical for both backends.
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
	// modinfo for the kernel backend), so the provider calls it once at startup
	// and caches the result in models.HostInfo.
	Info() BackendInfo
}

// BackendInfo describes what the active link backend can create on the current
// host. Gathered once at startup and folded into the provider's GET /info
// response.
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

// backend is the active link backend for this process. The AWG provider selects
// it before loading interfaces. Kept out of this package so service itself stays
// free of either backend's dependencies.
var backend Backend

// SetBackend selects the link backend for this process. Call it once, early in
// main, before loading interfaces. Not safe to call concurrently with interface
// operations.
func SetBackend(b Backend) { backend = b }

// ActiveBackendInfo returns the active backend's capabilities (see BackendInfo).
// SetBackend must have been called first.
func ActiveBackendInfo() BackendInfo { return backend.Info() }
