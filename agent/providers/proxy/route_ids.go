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
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

const routeIDAttempts = 20000

func RouteIdentifiers(linkID string) (string, string, string, string) {
	sum := sha256.Sum256([]byte(linkID))
	port := 20000 + int(binary.BigEndian.Uint16(sum[0:2]))%20000
	tun := fmt.Sprintf("vpn%x", sum[2:5])
	ipv4 := fmt.Sprintf("100.64.%d.%d/30", sum[5], int(sum[6]%127)*2+2)
	ipv6 := fmt.Sprintf("fd00::%x:%x:%x:1/126", sum[7], sum[8], sum[9])
	return net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)), tun, ipv4, ipv6
}

func RouteTable(linkID string) int {
	sum := sha256.Sum256([]byte(linkID))
	return 100 + int(binary.BigEndian.Uint16(sum[10:12]))%20000
}

func FindRouteID(linkID string, taken func(string) bool) (string, error) {
	for attempt := range routeIDAttempts {
		candidate := linkID
		if attempt > 0 {
			candidate = fmt.Sprintf("%s#route-%d", linkID, attempt)
		}
		if !taken(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("no free route identifiers")
}
