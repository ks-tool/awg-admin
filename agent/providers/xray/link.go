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

package xray

import (
	"encoding/json"
	"net"
	"strconv"

	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/providers/proxy"
)

func (Adapter) PrepareLink(raw json.RawMessage, link models.Link, endpoint models.LinkEndpoint, secret string) (proxy.Peer, models.RouteDriver, error) {
	spec, err := decodeInterface(raw)
	if err != nil {
		return proxy.Peer{}, models.RouteDriver{}, err
	}
	socksAddress, tunName, ipv4, ipv6 := proxy.RouteIdentifiers(link.ID)
	_, socksPort, err := net.SplitHostPort(socksAddress)
	if err != nil {
		return proxy.Peer{}, models.RouteDriver{}, err
	}
	port, err := strconv.Atoi(socksPort)
	if err != nil {
		return proxy.Peer{}, models.RouteDriver{}, err
	}
	stream := map[string]any{"network": spec.Network, "security": "none"}
	if spec.Network == "ws" {
		stream["wsSettings"] = map[string]any{"path": spec.Path}
		if len(spec.ServerName) > 0 {
			stream["wsSettings"].(map[string]any)["headers"] = map[string]any{"Host": spec.ServerName}
		}
	}
	if spec.TLS {
		stream["security"] = "tls"
		stream["tlsSettings"] = map[string]any{"serverName": spec.ServerName, "allowInsecure": true}
	}
	document := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []map[string]any{
			{
				"tag": "socks", "listen": "127.0.0.1", "port": port, "protocol": "socks",
				"settings": map[string]any{"auth": "noauth", "udp": true},
			},
			{
				"tag": "tun", "port": 0, "protocol": "tun",
				"settings": map[string]any{
					"name": tunName, "mtu": 1380, "gateway": []string{ipv4, ipv6},
					"autoSystemRoutingTable": []string{}, "autoOutboundsInterface": "auto",
				},
			},
		},
		"outbounds": []map[string]any{{
			"protocol": "vless", "tag": "next-hop",
			"settings": map[string]any{"vnext": []map[string]any{{
				"address": endpoint.Address, "port": spec.Port,
				"users": []map[string]any{{"id": secret, "encryption": "none"}},
			}}},
			"streamSettings": stream,
		}},
	}
	data, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return proxy.Peer{}, models.RouteDriver{}, err
	}
	driver := models.RouteDriver{
		Binary: "/usr/local/bin/xray", Args: []string{"run", "-config", "{{config}}"}, Config: data,
		SOCKS5: socksAddress, TUN: tunName,
	}
	return proxy.Peer{Secret: secret}, driver, nil
}
