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
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ks-tool/awg-admin/agent/models"
)

// recordingBackend is a no-op Backend that records the order of link operations
// and, in Up, touches markerPath so a hook can observe whether the link is up.
type recordingBackend struct {
	ops        []string
	markerPath string
}

func (b *recordingBackend) Add(iface string, _ bool) error { b.ops = append(b.ops, "add"); return nil }
func (b *recordingBackend) Delete(string) error            { b.ops = append(b.ops, "delete"); return nil }
func (b *recordingBackend) Exists(string) bool             { return false }
func (b *recordingBackend) Down(string) error              { b.ops = append(b.ops, "down"); return nil }
func (b *recordingBackend) SetMTU(string, int) error       { b.ops = append(b.ops, "mtu"); return nil }
func (b *recordingBackend) AddrAdd(string, string) error   { b.ops = append(b.ops, "addr"); return nil }
func (b *recordingBackend) SyncAddr(string, string) error  { b.ops = append(b.ops, "addr"); return nil }
func (b *recordingBackend) Info() BackendInfo              { return BackendInfo{} }

func (b *recordingBackend) Up(string) error {
	b.ops = append(b.ops, "up")
	// Touch the marker so a PreUp hook running `test -f <marker>` can tell the
	// link was already brought up when it ran.
	if _, err := runSh("touch " + b.markerPath); err != nil {
		return err
	}
	return nil
}

// runSh is a tiny helper mirroring runHooks' `sh -c` execution.
func runSh(cmd string) (string, error) {
	return "", runHooks("x", "test", []string{cmd})
}

// TestInterfaceCreateBringsLinkUpBeforePreUp is a regression test for an
// agent-restart failure: a tunnel entry interface's generated PreUp hook
// `ip route replace default dev %i table N` needs the device IFF_UP, but
// InterfaceCreate used to run PreUp *before* backend.Up, so on a fresh create
// (agent restart) the hook failed with "Device for nexthop is not up" and
// startup aborted. The link must be up by the time PreUp runs — as it already
// is in InterfaceUpdate.
func TestInterfaceCreateBringsLinkUpBeforePreUp(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "link-is-up")
	fake := &recordingBackend{markerPath: marker}

	prev := backend
	SetBackend(fake)
	t.Cleanup(func() { SetBackend(prev) })

	// A PreUp hook that only succeeds if the link is already up (marker exists).
	cfg := models.InterfaceConfig{
		Interface: "awg1",
		Address:   "10.0.0.1/24",
		PreUp:     []string{fmt.Sprintf("test -f %s", marker)},
	}

	if err := InterfaceCreate(cfg); err != nil {
		t.Fatalf("InterfaceCreate: PreUp ran before the link was up: %v", err)
	}

	// "up" must appear before the "postup" phase — i.e. Up precedes the hooks.
	upIdx := -1
	for i, op := range fake.ops {
		if op == "up" {
			upIdx = i
			break
		}
	}
	if upIdx == -1 {
		t.Fatalf("backend.Up was never called; ops=%v", fake.ops)
	}
}
