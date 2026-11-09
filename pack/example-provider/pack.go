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

package exampleprovider

import (
	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"
)

type Config struct {
	BinaryURL  string            `json:"binaryURL" jsonschema:"required,format=uri"`
	Env        map[string]string `json:"env,omitempty"`
	Definition map[string]any    `json:"definition"`
	Registry   map[string]any    `json:"registry,omitempty"`
}

func (c *Config) SetDefaults() {
	c.BinaryURL = "https://github.com/ks-tool/vpn-admin/releases/download/agent/v2.0.0/example-provider_2.0.0_linux_amd64"
	c.Env = map[string]string{"VPN_PROVIDER_DB": "/var/lib/example-provider", "VPN_PROVIDER_ADDR": "127.0.0.1:9090"}
	c.Definition = map[string]any{
		"name": "example", "version": "0.1.0",
		"resources": []any{map[string]any{
			"apiVersion": "example/v1alpha1", "kind": "Endpoint",
			"specSchema": map[string]any{
				"type": "object", "required": []any{"title", "address"},
				"properties": map[string]any{
					"title":    map[string]any{"type": "string", "minLength": 1},
					"address":  map[string]any{"type": "string", "minLength": 1},
					"disabled": map[string]any{"type": "boolean"},
				},
			},
			"schemaHash": "sha256:example-endpoint-v1alpha1",
			"ports":      []any{map[string]any{"name": "example", "linkKinds": []any{"example"}, "remotePorts": []any{"example"}}},
			"actions":    []any{map[string]any{"name": "inspect", "title": "Inspect endpoint", "resultType": "json", "argumentsSchema": map[string]any{"type": "object", "properties": map[string]any{}}}},
		}},
	}
	c.Registry = map[string]any{
		"protocolVersion": "v1",
		"providers":       []any{map[string]any{"address": "http://127.0.0.1:9090", "provider": c.Definition}},
	}
}

func (*Config) Meta() sdk.Meta {
	return sdk.Meta{Name: "example-provider", Version: "0.1.0", Description: "External VPN provider example", License: "Apache-2.0"}
}

func (*Config) PackRoot() packschema.Root {
	return packschema.Root{
		Ports:     map[string]int{"provider": 9090},
		Data:      map[string]packschema.Volume{"state": {MountPath: "/var/lib/example-provider", Size: "100Mi"}},
		Readiness: &packschema.Readiness{HTTP: "provider/v1/manifest"},
		Packages: map[string]packschema.OSPackages{
			"debian": {
				Prepare: []string{"ca-certificates", "curl"},
				Content: []packschema.File{
					{Path: "/usr/local/bin/example-provider", URL: "{{ .binaryURL }}", Mode: "0755"},
					{Path: "/etc/systemd/system/example-provider.service", Template: "example-provider.service", Mode: "0644"},
					{Path: "/etc/example-provider/example-provider.env", Values: "env", Format: "env", Mode: "0600"},
					{Path: "/var/lib/vpn-agent/providers.conf", Values: "registry", Format: "json", Mode: "0600"},
				},
				Services: []string{"example-provider"},
			},
		},
	}
}
