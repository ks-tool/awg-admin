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

import (
	"fmt"

	"github.com/rs/zerolog/log"
	"github.com/vishvananda/netlink"
)

// NetlinkBackend is the default Backend: it drives interfaces through the
// AmneziaWG (or plain WireGuard) kernel module over netlink — RTM_NEWLINK to
// create the link, address/MTU/up on it, RTM_DELLINK to remove it. It's what
// the standard `awg-agent` binary uses.
//
// A userspace build can embed NetlinkBackend and override only Add/Delete/Exists
// to manage an amneziawg-go process instead: the address/MTU/up operations work
// unchanged on the TUN that amneziawg-go creates, since it's an ordinary link
// as far as netlink is concerned.
type NetlinkBackend struct{}

// genericLink builds a netlink link handle for iface of the given kind. The
// kind only matters for Add (the kernel uses it to pick the driver at creation);
// every other operation addresses the link by name (netlink resolves the index),
// so the kind passed there is irrelevant.
func genericLink(iface, kind string) *netlink.GenericLink {
	return &netlink.GenericLink{
		LinkAttrs: netlink.LinkAttrs{
			Name: iface,
		},
		LinkType: kind,
	}
}

// Add creates the interface's link, preferring the AmneziaWG kernel module
// (kind "amneziawg") so the obfuscation params are actually honored. Hosts that
// only have the mainline WireGuard module — or whose amnezia module doesn't
// allow creating a link over netlink (returns EINVAL/EOPNOTSUPP) — fall back to
// a plain "wireguard" link so the agent still comes up. The fallback is logged
// at warn level because on such a host obfuscation silently won't apply.
func (NetlinkBackend) Add(iface string) error {
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

func (NetlinkBackend) Delete(iface string) error {
	return netlink.LinkDel(genericLink(iface, "wireguard"))
}

func (NetlinkBackend) Exists(iface string) bool {
	_, err := netlink.LinkByName(iface)
	return err == nil
}

func (NetlinkBackend) Up(iface string) error {
	return netlink.LinkSetUp(genericLink(iface, "wireguard"))
}

func (NetlinkBackend) Down(iface string) error {
	return netlink.LinkSetDown(genericLink(iface, "wireguard"))
}

func (NetlinkBackend) SetMTU(iface string, mtu int) error {
	return netlink.LinkSetMTU(genericLink(iface, "wireguard"), mtu)
}

func (NetlinkBackend) AddrAdd(iface, s string) error {
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
func (NetlinkBackend) SyncAddr(iface, s string) error {
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
