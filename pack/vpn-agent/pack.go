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

package vpnagent

import (
	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"
)

type Config struct {
	BinaryURL string            `json:"binaryURL" jsonschema:"required,format=uri"`
	Env       map[string]string `json:"env,omitempty"`
	TLS       TLS               `json:"tls,omitempty"`
}

type TLS struct {
	CA          packschema.Secret `json:"ca,omitempty"`
	Certificate packschema.Secret `json:"certificate,omitempty"`
	PrivateKey  packschema.Secret `json:"privateKey,omitempty"`
}

func (c *Config) SetDefaults() {
	c.BinaryURL = "https://github.com/ks-tool/vpn-admin/releases/download/agent/v2.0.0/vpn-agent_2.0.0_linux_amd64"
	c.Env = map[string]string{"VPN_AGENT_DB": "/var/lib/vpn-agent", "VPN_AGENT_ADDR": "127.0.0.1:8080"}
}

func (*Config) Meta() sdk.Meta {
	return sdk.Meta{Name: "vpn-agent", Version: "2.0.0", Description: "VPN agent and built-in providers", License: "Apache-2.0"}
}

func (*Config) PackRoot() packschema.Root {
	return packschema.Root{
		Ports:     map[string]int{"agent": 8080},
		Data:      map[string]packschema.Volume{"state": {MountPath: "/var/lib/vpn-agent", Size: "100Mi"}},
		Readiness: &packschema.Readiness{HTTP: "agent/info"},
		Packages: map[string]packschema.OSPackages{
			"debian": {
				Prepare: []string{"ca-certificates", "curl", "iproute2", "iptables"},
				Content: []packschema.File{
					{Path: "/usr/local/bin/vpn-agent", URL: "{{ .binaryURL }}", Mode: "0755"},
					{Path: "/etc/systemd/system/vpn-agent.service", Template: "vpn-agent.service", Mode: "0644"},
					{Path: "/etc/vpn-agent/vpn-agent.env", Values: "env", Format: "env", Mode: "0600"},
					{Path: "/etc/vpn-agent/ca.pem", Values: "tls.ca", Format: "secret", Mode: "0600"},
					{Path: "/etc/vpn-agent/server.pem", Values: "tls.certificate", Format: "secret", Mode: "0600"},
					{Path: "/etc/vpn-agent/server-key.pem", Values: "tls.privateKey", Format: "secret", Mode: "0600"},
				},
				Services: []string{"vpn-agent"},
			},
		},
	}
}
