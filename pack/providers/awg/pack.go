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

package awg

import (
	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"
)

type Config struct {
	Artifact string         `json:"artifact" jsonschema:"required,format=uri"`
	Registry map[string]any `json:"registry"`
}

func (c *Config) SetDefaults() {
	c.Artifact = "oci://ghcr.io/ks-tool/vpn-providers/awg:v2.0.0"
	c.Registry = map[string]any{
		"protocolVersion": "v1",
		"providers": []any{map[string]any{
			"address": "http://127.0.0.1:9091", "discover": true,
			"command": "/usr/local/bin/vpn-provider-awg",
			"env": map[string]any{
				"VPN_PROVIDER_AWG_DB":   "/var/lib/vpn-provider-awg",
				"VPN_PROVIDER_AWG_ADDR": "127.0.0.1:9091",
			},
			"provider": map[string]any{"name": "awg"},
		}},
	}
}

func (*Config) Meta() sdk.Meta {
	return sdk.Meta{Name: "vpn-provider-awg", Version: "2.0.0", Description: "AmneziaWG and WireGuard provider", License: "Apache-2.0"}
}

func (*Config) PackRoot() packschema.Root {
	return packschema.Root{
		Ports:     map[string]int{"provider": 9091},
		Data:      map[string]packschema.Volume{"state": {MountPath: "/var/lib/vpn-provider-awg", Size: "100Mi"}},
		Readiness: &packschema.Readiness{HTTP: "provider/v1/manifest"},
		Packages: map[string]packschema.OSPackages{
			"debian": {
				Prepare: []string{"ca-certificates", "iproute2", "iptables"},
				Content: []packschema.File{
					{Path: "/usr/local/bin/vpn-provider-awg", URL: "{{ .artifact }}", Mode: "0755"},
					{Path: "/var/lib/vpn-agent/providers.d/awg.conf", Values: "registry", Format: "json", Mode: "0600"},
				},
				Services: []string{"vpn-agent"},
			},
		},
	}
}
