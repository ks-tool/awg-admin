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

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExistingDatabaseIsUsedWithoutMovingIt(t *testing.T) {
	home := t.TempDir()
	legacy := filepath.Join(home, ".awg-admin")
	if err := os.WriteFile(legacy, []byte("legacy"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := AdminDatabasePath(home); got != legacy {
		t.Fatalf("legacy database was not selected: %q", got)
	}
	root := filepath.Join(home, ".vpn-admin")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(root, "db")
	if err := os.WriteFile(current, []byte("current"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := AdminDatabasePath(home); got != current {
		t.Fatalf("current database did not take precedence: %q", got)
	}
}

func TestCurrentDataLayout(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".vpn-admin")
	if got := AdminDatabasePath(home); got != filepath.Join(root, "db") {
		t.Fatalf("unexpected database path: %q", got)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("database directory was not created: %v", err)
	}
	if got := AdminAutocertPath(home); got != filepath.Join(root, "pki") {
		t.Fatalf("unexpected PKI path: %q", got)
	}
	if got := AdminPluginsPath(home); got != filepath.Join(root, "plugins") {
		t.Fatalf("unexpected plugins path: %q", got)
	}
	if got := AdminCachePath(home); got != filepath.Join(root, "cache") {
		t.Fatalf("unexpected cache path: %q", got)
	}
}

func TestEnvironmentFallback(t *testing.T) {
	t.Setenv("AWG_ADMIN_ADDR", "legacy")
	if got := Env("ADDR"); got != "legacy" {
		t.Fatalf("legacy environment was not read: %q", got)
	}
	t.Setenv("VPN_ADMIN_ADDR", "current")
	if got := Env("ADDR"); got != "current" {
		t.Fatalf("current environment did not take precedence: %q", got)
	}
}
