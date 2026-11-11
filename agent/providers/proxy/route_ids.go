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
	"fmt"
	"net"
)

func RouteIdentifiers(linkID string) (string, string, string, string) {
	sum := sha256.Sum256([]byte(linkID))
	port := 20000 + int(sum[0])%20000
	tun := fmt.Sprintf("vpn%x", sum[1:4])
	ipv4 := fmt.Sprintf("100.64.%d.%d/30", sum[4], int(sum[5]%127)*2+2)
	ipv6 := fmt.Sprintf("fd00::%x:%x:%x:1/126", sum[6], sum[7], sum[8])
	return net.JoinHostPort("127.0.0.1", fmt.Sprintf("%d", port)), tun, ipv4, ipv6
}

func RouteTable(linkID string) int {
	sum := sha256.Sum256([]byte(linkID))
	return 100 + int(sum[0])%20000
}
