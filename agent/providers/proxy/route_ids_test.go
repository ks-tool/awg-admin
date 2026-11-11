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
