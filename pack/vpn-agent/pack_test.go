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
	"testing"

	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"
	sdkschema "github.com/horchestra/pack/sdk/schema"
)

func TestPackGenerates(t *testing.T) {
	config := &Config{}
	config.SetDefaults()
	if _, err := sdk.Schema(config); err != nil {
		t.Fatal(err)
	}
	if err := sdk.Generate(t.TempDir(), config); err != nil {
		t.Fatal(err)
	}
	for _, file := range config.PackRoot().Packages["debian"].Content {
		if file.Path != "/etc/systemd/system/vpn-agent.service" {
			continue
		}
		if file.Values != "unit" || file.Format != "text" || file.Template != "" {
			t.Fatalf("systemd unit must use the embedded value: %+v", file)
		}
		if config.Unit != systemdUnit {
			t.Fatal("systemd unit default is not embedded in the pack config")
		}
		return
	}
	t.Fatal("systemd unit is missing from pack content")
}

func TestTLSIsZero(t *testing.T) {
	if !(TLS{}).IsZero() {
		t.Fatal("empty TLS must be zero")
	}
	secret := packschema.Secret{SecretRef: &packschema.SecretRef{Name: "tls", Key: "ca"}}
	for name, value := range map[string]TLS{
		"ca": {CA: secret}, "certificate": {Certificate: secret}, "privateKey": {PrivateKey: secret},
	} {
		if value.IsZero() {
			t.Errorf("TLS with %s must not be zero", name)
		}
	}
}

func TestSelfDeployValuesDoNotRequireTLS(t *testing.T) {
	config := &Config{SelfDeploy: true}
	config.SetDefaults()
	raw, err := sdk.Schema(config)
	if err != nil {
		t.Fatal(err)
	}
	document, err := sdkschema.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := document.Compile("pack://" + config.Meta().Name)
	if err != nil {
		t.Fatal(err)
	}
	env := make(map[string]any, len(config.Env))
	for key, value := range config.Env {
		env[key] = value
	}
	values := map[string]any{"env": env, "unit": config.Unit}
	if violations := compiled.Validate(values); len(violations) > 0 {
		t.Fatalf("values without TLS do not validate: %+v", violations)
	}
}

func TestSelfDeployPackUsesBinaryContent(t *testing.T) {
	config := &Config{SelfDeploy: true}
	config.SetDefaults()
	var binary *packschema.File
	for i, file := range config.PackRoot().Packages["debian"].Content {
		if file.Path == "/usr/local/bin/vpn-agent" {
			binary = &config.PackRoot().Packages["debian"].Content[i]
		}
	}
	if binary == nil || binary.Source != "vpn-agent" || binary.URL != "" {
		t.Fatalf("unexpected self-deploy binary file: %#v", binary)
	}
	if err := sdk.Generate(t.TempDir(), config); err != nil {
		t.Fatal(err)
	}
}
