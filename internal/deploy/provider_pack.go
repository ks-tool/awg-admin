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

package deploy

import (
	"fmt"

	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"
	"github.com/ks-tool/vpn-admin/models"
)

type providerPack struct {
	name       string
	contentDir string
	Artifact   string         `json:"artifact,omitempty" jsonschema:"format=uri"`
	Deployment map[string]any `json:"deployment"`
	SelfDeploy bool           `json:"-"`
}

func (p *providerPack) SetDefaults() {}

func (p *providerPack) setContentDir(dir string) { p.contentDir = dir }

func (p *providerPack) Meta() sdk.Meta {
	return sdk.Meta{Name: "vpn-provider-" + p.name, Version: "1.0.0", Description: "Deployable VPN provider", License: "Apache-2.0", Dir: p.contentDir}
}

func (p *providerPack) PackRoot() packschema.Root {
	binary := "/usr/local/bin/vpn-provider-" + p.name
	config := "/etc/vpn-provider-" + p.name + "/deploy.json"
	hooks := make([]packschema.Hook, 0, 2)
	for _, phase := range []packschema.Phase{packschema.PostInstall, packschema.PostUpgrade} {
		hooks = append(hooks, packschema.Hook{Name: "deploy-provider", Phase: phase, Idempotent: true, Command: []string{binary, "deploy", "--config", config}})
	}
	content := packschema.File{Path: binary, URL: "{{ .artifact }}", Mode: "0755"}
	if p.SelfDeploy {
		content.URL = ""
		content.Source = p.Meta().Name
	}
	return packschema.Root{Hooks: hooks, Packages: map[string]packschema.OSInstall{
		"debian": {
			Content: []packschema.File{
				content,
				{Path: config, Values: "deployment", Format: "json", Mode: "0600"},
			},
		},
	}}
}

func newProviderPack(provider models.ProviderDeployment) (sdk.Pack, map[string]any, error) {
	if len(provider.Name) == 0 || (len(provider.Artifact) == 0) == (len(provider.BinaryPath) == 0) {
		return nil, nil, fmt.Errorf("provider name and exactly one OCI artifact or local binary are required")
	}
	values := map[string]any{
		"deployment": map[string]any{"agentDB": remoteDBPath, "options": provider.Options},
	}
	if len(provider.Artifact) > 0 {
		values["artifact"] = provider.Artifact
	}
	return &providerPack{name: provider.Name, SelfDeploy: len(provider.BinaryPath) > 0}, values, nil
}
