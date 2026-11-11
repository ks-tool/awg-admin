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

const systemdUnit = `[Unit]
Description=VPN Agent
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/vpn-agent/vpn-agent.env
ExecStart=/usr/local/bin/vpn-agent
Restart=on-failure
RestartSec=3
StateDirectory=vpn-agent

[Install]
WantedBy=multi-user.target
`

type Config struct {
	Artifact   string            `json:"artifact,omitempty" jsonschema:"format=uri"`
	Env        map[string]string `json:"env,omitempty"`
	TLS        TLS               `json:"tls,omitempty"`
	Unit       string            `json:"unit"`
	SelfDeploy bool              `json:"-"`
}

type TLS struct {
	CA          packschema.Secret `json:"ca" jsonschema:"required"`
	Certificate packschema.Secret `json:"certificate" jsonschema:"required"`
	PrivateKey  packschema.Secret `json:"privateKey" jsonschema:"required"`
}

func (c *Config) SetDefaults() {
	c.Artifact = "oci://ghcr.io/ks-tool/vpn-agent:v2.0.0-alpha.1"
	c.Env = map[string]string{
		"VPN_AGENT_DB":           "/var/lib/vpn-agent",
		"VPN_AGENT_ADDR":         "127.0.0.1:8080",
		"VPN_AGENT_LEGACY_PROXY": "http://127.0.0.1:9091",
	}
	c.Unit = systemdUnit
}

func (*Config) Meta() sdk.Meta {
	return sdk.Meta{Name: "vpn-agent", Version: "2.0.0", Description: "VPN agent for external providers", License: "Apache-2.0"}
}

func (c *Config) PackRoot() packschema.Root {
	binary := packschema.File{Path: "/usr/local/bin/vpn-agent", URL: "{{ .artifact }}", Mode: "0755"}
	if c.SelfDeploy {
		binary.URL = ""
		binary.Source = "vpn-agent"
	}
	return packschema.Root{
		Ports:     map[string]int{"agent": 8080},
		Data:      map[string]packschema.Volume{"state": {MountPath: "/var/lib/vpn-agent", Size: "100Mi"}},
		Readiness: &packschema.Readiness{HTTP: "agent/v1/manifest"},
		Hooks: []packschema.Hook{
			{
				Name: "stop-legacy-agent", Phase: packschema.PreInstall, Idempotent: true, Timeout: "30s",
				Command: []string{"sh", "-c", `if systemctl cat awg-agent.service >/dev/null 2>&1; then systemctl disable --now awg-agent.service; fi`},
			},
			{
				Name: "remove-legacy-agent", Phase: packschema.PostInstall, Idempotent: true, Timeout: "30s",
				Command: []string{"sh", "-c", `legacy_unit=$(systemctl show --property FragmentPath --value awg-agent.service 2>/dev/null || true)
if [ -n "$legacy_unit" ] || [ -e /usr/local/bin/awg-agent ]; then
	systemctl disable --now awg-agent.service 2>/dev/null || true
	if [ -n "$legacy_unit" ]; then rm -f "$legacy_unit"; fi
	rm -f /usr/local/bin/awg-agent
	systemctl daemon-reload
	systemctl reset-failed awg-agent.service 2>/dev/null || true
fi`},
			},
		},
		Packages: map[string]packschema.OSInstall{
			"debian": {
				Prepare: []string{"ca-certificates"},
				Content: []packschema.File{
					binary,
					{Path: "/etc/systemd/system/vpn-agent.service", Values: "unit", Format: "text", Mode: "0644"},
					{Path: "/etc/vpn-agent/vpn-agent.env", Values: "env", Format: "env", Mode: "0600"},
					{Path: "/etc/vpn-agent/ca.pem", Values: "tls/ca", Format: "text", Mode: "0600"},
					{Path: "/etc/vpn-agent/server.pem", Values: "tls/certificate", Format: "text", Mode: "0600"},
					{Path: "/etc/vpn-agent/server-key.pem", Values: "tls/privateKey", Format: "text", Mode: "0600"},
				},
				Services: []string{"vpn-agent"},
			},
		},
	}
}
