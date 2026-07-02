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

// Package userspace provides a service.Backend that runs each interface as an
// in-process userspace WireGuard device using the amneziawg-go library — for
// hosts without a (matching) AmneziaWG kernel module. It's wired in by the
// awg-agent-userspace binary via service.SetBackend; the rest of the agent is
// unchanged.
//
// Unlike the kernel backend it needs no netlink library: the interface is a TUN
// the amneziawg-go library creates in-process, and its address/MTU/up are set
// with the `ip` command (the same tool the agent already uses for lifecycle
// hooks, and the way jwg / awg-quick's userspace path do it). So the userspace
// build doesn't pull in vishvananda/netlink at all.
package userspace

import (
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/ks-tool/awg-admin/agent/internal/service"

	"github.com/amnezia-vpn/amneziawg-go/conn"
	"github.com/amnezia-vpn/amneziawg-go/device"
	"github.com/amnezia-vpn/amneziawg-go/ipc"
	"github.com/amnezia-vpn/amneziawg-go/tun"
	"github.com/rs/zerolog/log"
)

// Backend runs interfaces as in-process userspace WireGuard devices via
// amneziawg-go. Add creates the TUN + device + UAPI socket; Delete tears them
// down; the address/MTU/up operations shell out to `ip`. The device config
// itself is still pushed and read back through wgctrl, which finds the device's
// UAPI socket under /var/run/amneziawg (the same path the admin's agent client
// already probes) — so nothing above the backend changes.
type Backend struct {
	mu      sync.Mutex
	devices map[string]*wgDevice
}

// wgDevice is one running userspace interface: its amneziawg-go device and the
// UAPI listener wgctrl configures it through.
type wgDevice struct {
	dev  *device.Device
	uapi net.Listener
}

// New returns a userspace Backend backed by the in-process amneziawg-go library.
func New() *Backend {
	return &Backend{devices: make(map[string]*wgDevice)}
}

// Add creates the interface's userspace TUN, its amneziawg-go device and the
// UAPI socket wgctrl configures it through. The device stays down until the
// caller brings the link up (Up → the TUN's up event) and pushes its config
// over the UAPI socket, matching the kernel backend's lifecycle. The amnezia
// flag is irrelevant here: the amneziawg-go device serves both plain WireGuard
// and AmneziaWG, deciding per the obfuscation params pushed over the UAPI.
func (b *Backend) Add(iface string, amnezia bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.devices[iface]; ok {
		return nil // already running
	}

	tunDev, err := tun.CreateTUN(iface, device.DefaultMTU)
	if err != nil {
		return fmt.Errorf("create TUN %q: %w", iface, err)
	}

	logger := device.NewLogger(device.LogLevelError, "["+iface+"] ")
	dev := device.NewDevice(tunDev, conn.NewDefaultBind(), logger)

	// UAPI socket (under /var/run/amneziawg) so the agent's own wgctrl client
	// configures this device exactly as it does a kernel one.
	fileUAPI, err := ipc.UAPIOpen(iface)
	if err != nil {
		dev.Close()
		return fmt.Errorf("open UAPI socket for %q: %w", iface, err)
	}
	uapiListener, err := ipc.UAPIListen(iface, fileUAPI)
	if err != nil {
		_ = fileUAPI.Close()
		dev.Close()
		return fmt.Errorf("listen UAPI socket for %q: %w", iface, err)
	}

	go func() {
		for {
			c, err := uapiListener.Accept()
			if err != nil {
				return // listener closed by Delete
			}
			go dev.IpcHandle(c)
		}
	}()

	b.devices[iface] = &wgDevice{dev: dev, uapi: uapiListener}
	return nil
}

// Delete tears down the interface's UAPI socket and amneziawg-go device, which
// removes the TUN. Idempotent: an interface this backend didn't create is a
// no-op here — the caller's netlink-free teardown still runs.
func (b *Backend) Delete(iface string) error {
	b.mu.Lock()
	d, ok := b.devices[iface]
	if ok {
		delete(b.devices, iface)
	}
	b.mu.Unlock()

	if !ok {
		return nil
	}
	if err := d.uapi.Close(); err != nil {
		log.Warn().Err(err).Str("interface", iface).Msg("closing UAPI listener failed")
	}
	d.dev.Close()
	return nil
}

// Exists reports whether this backend is running a device for iface. Unlike the
// kernel backend it doesn't probe netlink: a userspace TUN's lifetime is this
// process's, so the in-process map is authoritative (a device from a previous
// run is gone with that process).
func (b *Backend) Exists(iface string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, ok := b.devices[iface]
	return ok
}

func (b *Backend) Up(iface string) error {
	return ipCmd("link", "set", "up", "dev", iface)
}

func (b *Backend) Down(iface string) error {
	return ipCmd("link", "set", "down", "dev", iface)
}

func (b *Backend) SetMTU(iface string, mtu int) error {
	return ipCmd("link", "set", "mtu", strconv.Itoa(mtu), "dev", iface)
}

func (b *Backend) AddrAdd(iface, addr string) error {
	return ipCmd("address", "add", addr, "dev", iface)
}

// SyncAddr makes addr the interface's only address: flush what's there, then
// add it (mirrors the kernel backend's remove-others-then-set, via `ip`).
func (b *Backend) SyncAddr(iface, addr string) error {
	if err := ipCmd("address", "flush", "dev", iface); err != nil {
		return err
	}
	return ipCmd("address", "add", addr, "dev", iface)
}

// Info reports the userspace backend's capabilities. The amneziawg-go device
// serves both plain WireGuard and AmneziaWG in process, needing no kernel
// module, so both kinds are always creatable and KernelModule is always false.
func (b *Backend) Info() service.BackendInfo {
	return service.BackendInfo{
		Kind:           "userspace",
		KernelModule:   false,
		InterfaceKinds: []string{"amneziawg", "wireguard"},
	}
}

// ipCmd runs the `ip` command with args, returning its combined output on error.
func ipCmd(args ...string) error {
	out, err := exec.Command("ip", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("ip %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}
