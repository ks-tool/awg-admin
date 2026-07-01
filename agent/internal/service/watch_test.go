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

package service

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ks-tool/awg-admin/agent/models"

	"github.com/fsnotify/fsnotify"
)

func TestIfaceNameFromConfigPath(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"/var/lib/awg-agent/wg0.json", "wg0"},
		{"wg0.json", "wg0"},
		{"/var/lib/awg-agent/wg0.json.tmp", ""}, // not a *.json file
		{"/var/lib/awg-agent/.json", ""},        // empty name
		{"/var/lib/awg-agent/notes.txt", ""},
	}
	for _, c := range cases {
		if got := ifaceNameFromConfigPath(c.path); got != c.want {
			t.Errorf("ifaceNameFromConfigPath(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestHandleStorageEventOnlyActsOnRemoveOfConfigFiles(t *testing.T) {
	var deleted []string
	orig := deleteInterface
	deleteInterface = func(cfg models.InterfaceConfig) error {
		deleted = append(deleted, cfg.Interface)
		return nil
	}
	defer func() { deleteInterface = orig }()

	// A write (config push, not a deletion) must be ignored.
	handleStorageEvent(fsnotify.Event{Name: "/data/wg0.json", Op: fsnotify.Write})
	// A remove of something that isn't one of our config files must be
	// ignored too.
	handleStorageEvent(fsnotify.Event{Name: "/data/notes.txt", Op: fsnotify.Remove})
	// The actual case this exists for: a config file disappearing.
	handleStorageEvent(fsnotify.Event{Name: "/data/wg0.json", Op: fsnotify.Remove})

	if len(deleted) != 1 || deleted[0] != "wg0" {
		t.Fatalf("deleteInterface calls = %v, want exactly one call with \"wg0\"", deleted)
	}
}

// TestWatchStorageReactsToRealFileRemoval exercises the actual fsnotify
// wiring end to end against a real temp directory, rather than just the
// pure event-filtering logic above.
func TestWatchStorageReactsToRealFileRemoval(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "wg0.json")
	if err := os.WriteFile(cfgPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	done := make(chan string, 1)
	orig := deleteInterface
	deleteInterface = func(cfg models.InterfaceConfig) error {
		done <- cfg.Interface
		return nil
	}
	defer func() { deleteInterface = orig }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := WatchStorage(ctx, dir); err != nil {
		t.Fatalf("WatchStorage: %v", err)
	}

	if err := os.Remove(cfgPath); err != nil {
		t.Fatalf("remove config file: %v", err)
	}

	select {
	case iface := <-done:
		if iface != "wg0" {
			t.Fatalf("deleteInterface called with %q, want \"wg0\"", iface)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the watcher to react to the file removal")
	}
}
