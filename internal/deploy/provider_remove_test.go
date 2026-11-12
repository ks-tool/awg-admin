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
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	packcmd "github.com/horchestra/pack/cmd"
	"github.com/ks-tool/vpn-admin/models"
)

func TestRemoveProviderFromPack(t *testing.T) {
	original := embeddedPackApply
	t.Cleanup(func() { embeddedPackApply = original })
	t.Setenv("HOME", t.TempDir())
	called := false
	embeddedPackApply = func(_ context.Context, options packcmd.ApplyOptions) error {
		called = true
		data, err := os.ReadFile(filepath.Join(options.Dir, "packs", "vpn-provider-awg-remove", "schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, expected := range []string{"/usr/local/bin/vpn-provider-awg", "/var/lib/vpn-agent/providers.d/awg.conf"} {
			if !strings.Contains(text, expected) {
				t.Fatalf("removal pack does not contain %q:\n%s", expected, text)
			}
		}
		if strings.Contains(text, "systemctl") {
			t.Fatalf("provider removal still restarts the agent:\n%s", text)
		}
		return nil
	}
	srv := models.Server{SSH: models.SSHConfig{Host: "192.0.2.1", User: "root", Key: "/tmp/id"}}
	if err := RemoveProviderFromPack(context.Background(), srv, "awg", "", ""); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("embedded pack apply was not called")
	}
	if err := RemoveProviderFromPack(context.Background(), srv, "awg; reboot", "", ""); err == nil {
		t.Fatal("unsafe provider name was accepted")
	}
}
