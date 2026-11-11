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
