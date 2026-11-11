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

package providerdeploy

import (
	"fmt"
	"path/filepath"

	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"
	"github.com/ks-tool/vpn-agent/models"
)

type Config struct {
	Artifact         string         `json:"artifact" jsonschema:"required,format=uri"`
	Registry         map[string]any `json:"registry"`
	RegistrationPath string         `json:"registrationPath" jsonschema:"required"`
}

func (c *Config) SetDefaults() {
	c.Artifact = "oci://ghcr.io/ks-tool/vpn-providers/awg:v1.0.0-alpha.1"
	c.RegistrationPath = "/var/lib/vpn-agent/providers.d/awg.conf"
}

func (*Config) Meta() sdk.Meta {
	return sdk.Meta{Name: "vpn-provider-awg", Version: "2.0.0", Description: "AmneziaWG and WireGuard provider", License: "Apache-2.0"}
}

func (*Config) PackRoot() packschema.Root {
	return packschema.Root{
		Ports:     map[string]int{"provider": 9091},
		Data:      map[string]packschema.Volume{"state": {MountPath: "/var/lib/vpn-provider-awg", Size: "100Mi"}},
		Readiness: &packschema.Readiness{HTTP: "provider/v1/manifest"},
		Packages: map[string]packschema.OSInstall{
			"debian": {
				Prepare: []string{"ca-certificates", "iproute2", "procps"},
				Sysctls: map[string]string{"net.ipv4.ip_forward": "1"},
				Content: []packschema.File{
					{Path: "/usr/local/bin/vpn-provider-awg", URL: "{{ .artifact }}", Mode: "0755"},
					{Path: "{{ .registrationPath }}", Values: "registry", Format: "json", Mode: "0600"},
				},
				Services: []string{"vpn-agent"},
			},
		},
	}
}

func New(provider models.ProviderDeployment, agentDB string) (sdk.Pack, map[string]any, error) {
	if provider.Name != "awg" {
		return nil, nil, fmt.Errorf("provider %q is not AWG", provider.Name)
	}
	if provider.Artifact == "" {
		return nil, nil, fmt.Errorf("AWG provider OCI artifact is required")
	}
	userspace := false
	for name, value := range provider.Options {
		if name != "userspace" {
			return nil, nil, fmt.Errorf("unsupported AWG provider option %q", name)
		}
		enabled, ok := value.(bool)
		if !ok {
			return nil, nil, fmt.Errorf("AWG provider option userspace must be a boolean")
		}
		userspace = enabled
	}
	registration := map[string]any{
		"address": "http://127.0.0.1:9091", "discover": true,
		"command": "/usr/local/bin/vpn-provider-awg",
		"env": map[string]any{
			"VPN_PROVIDER_AWG_DB":   "/var/lib/vpn-provider-awg",
			"VPN_PROVIDER_AWG_ADDR": "127.0.0.1:9091",
		},
		"provider": map[string]any{"name": "awg"},
	}
	if userspace {
		registration["args"] = []any{"--userspace"}
	}
	config := &Config{}
	config.SetDefaults()
	values := map[string]any{
		"artifact":         provider.Artifact,
		"registrationPath": filepath.Join(agentDB, "providers.d", "awg.conf"),
		"registry": map[string]any{
			"protocolVersion": models.ProtocolVersion,
			"providers":       []any{registration},
		},
	}
	return config, values, nil
}
