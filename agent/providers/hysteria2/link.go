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

package hysteria2

import (
	"encoding/json"
	"net"
	"strconv"

	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/providers/proxy"
)

func (Adapter) PrepareLink(raw json.RawMessage, link models.Link, endpoint models.LinkEndpoint, secret, routeID string) (proxy.Peer, models.RouteDriver, error) {
	spec, err := decodeInterface(raw)
	if err != nil {
		return proxy.Peer{}, models.RouteDriver{}, err
	}
	socksAddress, tunName, ipv4, ipv6 := proxy.RouteIdentifiers(routeID)
	document := map[string]any{
		"server": net.JoinHostPort(endpoint.Address, strconv.Itoa(spec.Port)),
		"auth":   "link-" + link.ID + ":" + secret,
		"tls":    map[string]any{"sni": spec.ServerName, "insecure": true},
		"socks5": map[string]any{"listen": socksAddress},
		"tun": map[string]any{
			"name": tunName, "mtu": 1380,
			"address": map[string]any{"ipv4": ipv4, "ipv6": ipv6},
		},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return proxy.Peer{}, models.RouteDriver{}, err
	}
	driver := models.RouteDriver{
		Binary: "/usr/local/bin/hysteria", Args: []string{"client", "-c", "{{config}}"}, Config: data,
		SOCKS5: socksAddress, TUN: tunName,
	}
	return proxy.Peer{Secret: secret}, driver, nil
}
