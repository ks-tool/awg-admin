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

// Package kernel is the service.Backend that drives interfaces through the
// AmneziaWG (or plain WireGuard) kernel module over netlink — RTM_NEWLINK to
// create the link, address/MTU/up on it, RTM_DELLINK to remove it. It's what
// the standard awg-agent binary wires in via service.SetBackend. Keeping it in
// its own package confines the vishvananda/netlink dependency to the kernel
// build, so the userspace agent (agent/internal/userspace) doesn't pull it in.
package kernel

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/vishvananda/netlink"
)

// Backend implements service.Backend against the kernel module via netlink.
type Backend struct{}

// New returns the kernel backend.
func New() Backend { return Backend{} }

// genericLink builds a netlink link handle for iface of the given kind. The
// kind only matters for Add (the kernel uses it to pick the driver at
// creation); every other operation addresses the link by name (netlink
// resolves the index), so the kind passed there is irrelevant.
func genericLink(iface, kind string) *netlink.GenericLink {
	return &netlink.GenericLink{
		LinkAttrs: netlink.LinkAttrs{
			Name: iface,
		},
		LinkType: kind,
	}
}

// Add creates the interface's link with the kind the config asks for: a plain
// WireGuard interface (amnezia false) is created as kind "wireguard" directly.
// An AmneziaWG interface (amnezia true) is created as kind "amneziawg" so the
// obfuscation params are honored, falling back to a plain "wireguard" link on
// hosts that only have the mainline module — or whose amnezia module doesn't
// allow creating a link over netlink (EINVAL/EOPNOTSUPP). That fallback is
// logged at warn level because obfuscation silently won't apply there.
func (Backend) Add(iface string, amnezia bool) error {
	if !amnezia {
		return netlink.LinkAdd(genericLink(iface, "wireguard"))
	}

	err := netlink.LinkAdd(genericLink(iface, "amneziawg"))
	if err == nil {
		return nil
	}

	wgErr := netlink.LinkAdd(genericLink(iface, "wireguard"))
	if wgErr != nil {
		return fmt.Errorf("create link %q: amneziawg kind: %v; wireguard kind: %w", iface, err, wgErr)
	}
	log.Warn().Str("interface", iface).Err(err).Msg(
		"AmneziaWG-kind link creation failed; created a plain WireGuard link instead — traffic obfuscation will NOT be applied on this host")
	return nil
}

func (Backend) Delete(iface string) error {
	return netlink.LinkDel(genericLink(iface, "wireguard"))
}

func (Backend) Exists(iface string) bool {
	_, err := netlink.LinkByName(iface)
	return err == nil
}

func (Backend) Up(iface string) error {
	return netlink.LinkSetUp(genericLink(iface, "wireguard"))
}

func (Backend) Down(iface string) error {
	return netlink.LinkSetDown(genericLink(iface, "wireguard"))
}

func (Backend) SetMTU(iface string, mtu int) error {
	return netlink.LinkSetMTU(genericLink(iface, "wireguard"), mtu)
}

func (Backend) AddrAdd(iface, s string) error {
	addr, err := netlink.ParseAddr(s)
	if err != nil {
		return err
	}
	return netlink.AddrAdd(genericLink(iface, "wireguard"), addr)
}

// SyncAddr makes s the interface's only address, removing every other address
// already assigned to it first. netlink's AddrReplace only adds-or-updates the
// one address it's given — it has nothing to do with whatever *other* addresses
// are already on the link, so calling it alone after an admin edits an
// interface's addr leaves the old one still assigned alongside the new one.
func (Backend) SyncAddr(iface, s string) error {
	wantAddr, err := netlink.ParseAddr(s)
	if err != nil {
		return err
	}

	link := genericLink(iface, "wireguard")
	existing, err := netlink.AddrList(link, 0)
	if err != nil {
		return err
	}
	for _, addr := range existing {
		if addr.IPNet.String() == wantAddr.IPNet.String() {
			continue
		}
		if err := netlink.AddrDel(link, &addr); err != nil {
			return err
		}
	}

	return netlink.AddrReplace(link, wantAddr)
}
