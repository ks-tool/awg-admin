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
	"fmt"
	"net/netip"
	"testing"
)

func TestRouteIdentifiersProduceValidAddresses(t *testing.T) {
	for index := range 256 {
		_, _, ipv4, ipv6 := RouteIdentifiers(fmt.Sprintf("link-%d", index))
		prefix4, err := netip.ParsePrefix(ipv4)
		if err != nil {
			t.Fatalf("invalid IPv4 %q: %v", ipv4, err)
		}
		if !prefix4.Addr().Next().IsValid() {
			t.Fatalf("IPv4 has no next address: %q", ipv4)
		}
		prefix6, err := netip.ParsePrefix(ipv6)
		if err != nil {
			t.Fatalf("invalid IPv6 %q: %v", ipv6, err)
		}
		if !prefix6.Addr().Next().IsValid() {
			t.Fatalf("IPv6 has no next address: %q", ipv6)
		}
	}
}

func TestRouteIdentifiersUseExtendedHashSpace(t *testing.T) {
	ports := make(map[string]bool)
	tables := make(map[int]bool)
	for index := range 512 {
		id := fmt.Sprintf("link-%d", index)
		socksAddress, _, _, _ := RouteIdentifiers(id)
		ports[socksAddress] = true
		tables[RouteTable(id)] = true
	}
	if len(ports) <= 256 || len(tables) <= 256 {
		t.Fatalf("route identifiers still have a 256-value space: ports=%d tables=%d", len(ports), len(tables))
	}
}

func TestFindRouteIDReallocatesCollisions(t *testing.T) {
	firstSOCKS, firstTUN, _, _ := RouteIdentifiers("link")
	firstTable := RouteTable("link")
	routeID, err := FindRouteID("link", func(candidate string) bool {
		socksAddress, tunName, _, _ := RouteIdentifiers(candidate)
		return socksAddress == firstSOCKS || tunName == firstTUN || RouteTable(candidate) == firstTable
	})
	if err != nil {
		t.Fatal(err)
	}
	if routeID == "link" {
		t.Fatal("colliding route ID was reused")
	}
	socksAddress, tunName, _, _ := RouteIdentifiers(routeID)
	if socksAddress == firstSOCKS || tunName == firstTUN || RouteTable(routeID) == firstTable {
		t.Fatalf("replacement route ID still collides: %q", routeID)
	}
}
