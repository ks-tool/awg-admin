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

package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAgentEnvironmentPrefix(t *testing.T) {
	t.Setenv("VPN_AGENT_DB", "current")
	if got := agentEnvironmentPrefix(); got != "VPN_AGENT_" {
		t.Fatalf("current environment prefix was not selected: %q", got)
	}
	t.Setenv("VPN_AGENT_DB", "")
	if got := agentEnvironmentPrefix(); got != "VPN_AGENT_" {
		t.Fatalf("explicit empty current variable must retain precedence: %q", got)
	}
	if err := os.Unsetenv("VPN_AGENT_DB"); err != nil {
		t.Fatal(err)
	}
	if got := agentEnvironmentPrefix(); got != "AWG_AGENT_" {
		t.Fatalf("legacy environment prefix was not selected: %q", got)
	}
}

func TestDefaultAgentDBPathPreservesLegacyInstallation(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".awg-agent")
	if err := os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if got := defaultAgentDBPath(home); got != legacy {
		t.Fatalf("legacy agent directory was not selected: %q", got)
	}
	current := filepath.Join(home, ".vpn-agent")
	if err := os.Mkdir(current, 0700); err != nil {
		t.Fatal(err)
	}
	if got := defaultAgentDBPath(home); got != current {
		t.Fatalf("current agent directory did not take precedence: %q", got)
	}
}
