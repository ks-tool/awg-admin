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

import "testing"

func TestIsIPv6Address(t *testing.T) {
	cases := []struct {
		addr string
		want bool
	}{
		{"", false},
		{"10.0.0.1/24", false},
		{"10.0.0.1", false},
		{"172.23.5.1/16", false},
		{"fd00::1/64", true},
		{"fe80::1", true},
		{"2001:db8::1/48", true},
		{"not-an-ip", false},
	}
	for _, c := range cases {
		if got := isIPv6Address(c.addr); got != c.want {
			t.Errorf("isIPv6Address(%q) = %v, want %v", c.addr, got, c.want)
		}
	}
}

func TestDisableIPv6(t *testing.T) {
	// Best-effort and never panics: an IPv6-addressed interface is skipped, and a
	// nonexistent interface (or a kernel without IPv6 / read-only /proc/sys) is
	// swallowed. Nothing to assert beyond "does not fail the caller".
	disableIPv6("wg-test-ipv6", "fd00::1/64")
	disableIPv6("wg-test-nonexistent-xyz", "10.0.0.1/24")
}
