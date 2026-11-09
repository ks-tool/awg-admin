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

package remote

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ks-tool/vpn-agent/models"
)

func TestSupervisorRestartsAndStopsProvider(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "starts")
	script := filepath.Join(dir, "provider.sh")
	content := "#!/bin/sh\necho started >> " + logPath + "\nif [ \"$(wc -l < " + logPath + ")\" -lt 2 ]; then exit 1; fi\ntrap 'exit 0' TERM\nwhile :; do sleep 1; done\n"
	if err := os.WriteFile(script, []byte(content), 0700); err != nil {
		t.Fatal(err)
	}
	supervisor := NewSupervisor([]models.ProviderRegistration{{
		Command: script, Provider: models.ProviderDefinition{Name: "test"},
	}})
	if err := supervisor.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		data, _ := os.ReadFile(logPath)
		if strings.Count(string(data), "started") >= 2 {
			break
		}
		if time.Now().After(deadline) {
			supervisor.Close()
			t.Fatalf("provider was not restarted: %s", data)
		}
		time.Sleep(20 * time.Millisecond)
	}
	supervisor.Close()
}

func TestSupervisorRequiresAbsoluteCommand(t *testing.T) {
	supervisor := NewSupervisor([]models.ProviderRegistration{{
		Command: "provider", Provider: models.ProviderDefinition{Name: "test"},
	}})
	if err := supervisor.Start(context.Background()); err == nil {
		supervisor.Close()
		t.Fatal("relative provider command was accepted")
	}
}
