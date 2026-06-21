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
	"github.com/vishvananda/netlink"
)

type Device string

func newGenericLink(dev Device) *netlink.GenericLink {
	return &netlink.GenericLink{
		LinkAttrs: netlink.LinkAttrs{
			Name: string(dev),
		},
		LinkType: "wireguard",
	}
}

func (dev Device) Add() error {
	return netlink.LinkAdd(newGenericLink(dev))
}

func (dev Device) Delete() error {
	return netlink.LinkDel(newGenericLink(dev))
}

func (dev Device) Up() error {
	return netlink.LinkSetUp(newGenericLink(dev))
}

func (dev Device) Down() error {
	return netlink.LinkSetDown(newGenericLink(dev))
}

func (dev Device) Get() (netlink.Link, error) {
	return netlink.LinkByName(string(dev))
}

func (dev Device) SetMTU(mtu int) error {
	return netlink.LinkSetMTU(newGenericLink(dev), mtu)
}

func (dev Device) AddrList() ([]netlink.Addr, error) {
	return netlink.AddrList(newGenericLink(dev), 0)
}

func (dev Device) AddrAdd(s string) error {
	addr, err := netlink.ParseAddr(s)
	if err != nil {
		return err
	}
	return netlink.AddrAdd(newGenericLink(dev), addr)
}

func (dev Device) AddrDel(s string) error {
	addr, err := netlink.ParseAddr(s)
	if err != nil {
		return err
	}
	return netlink.AddrDel(newGenericLink(dev), addr)
}

// SyncAddr makes want dev's only address, removing every other address
// already assigned to it first. netlink's AddrReplace only adds-or-updates
// the one address it's given — it has nothing to do with whatever *other*
// addresses are already on the link, so calling it alone after an admin
// edits an interface's addr leaves the old one still assigned alongside the
// new one (see InterfaceUpdate).
func (dev Device) SyncAddr(want string) error {
	wantAddr, err := netlink.ParseAddr(want)
	if err != nil {
		return err
	}

	existing, err := dev.AddrList()
	if err != nil {
		return err
	}
	link := newGenericLink(dev)
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
