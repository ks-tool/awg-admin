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
	"strings"
	"testing"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
)

// newValidationTestServer gives a Service with one server and no reachable agent
// (pushes fail best-effort, which validation runs before), enough to exercise the
// input validation on CreateInterface/UpdateInterfaceConfig/AddPeer.
func newValidationTestServer(t *testing.T) (*Service, *models.Server) {
	t.Helper()
	svc := newTestService(t)
	srv, err := svc.CreateServer(ServerInput{
		Name:  "srv1",
		SSH:   models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
		Agent: models.Agent{Address: "127.0.0.1:1"},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	return svc, srv
}

func TestCreateInterfaceRejectsDuplicates(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	if _, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820,
	}); err != nil {
		t.Fatalf("first CreateInterface: %v", err)
	}

	cases := []struct {
		name    string
		cfg     agentmodels.InterfaceConfig
		wantSub string
	}{
		{"duplicate name", agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.1.0.1/24", ListenPort: 51821}, "name"},
		{"duplicate port", agentmodels.InterfaceConfig{Interface: "wg1", Address: "10.1.0.1/24", ListenPort: 51820}, "port"},
		{"overlapping subnet", agentmodels.InterfaceConfig{Interface: "wg1", Address: "10.0.0.5/24", ListenPort: 51821}, "overlap"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.CreateInterface(srv.ID.String(), tc.cfg)
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected error containing %q, got %v", tc.wantSub, err)
			}
		})
	}
}

func TestCreateInterfaceAllowsDistinct(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	if _, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820,
	}); err != nil {
		t.Fatalf("wg0: %v", err)
	}
	// Distinct name, port and non-overlapping subnet — must be accepted.
	if _, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg1", Address: "10.0.1.1/24", ListenPort: 51821,
	}); err != nil {
		t.Fatalf("wg1 (distinct): %v", err)
	}
}

func TestAddPeerRejectsBadAllowedIPs(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	iface, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}
	user, err := svc.CreateUser(UserInput{Name: "u1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// A valid in-subnet peer succeeds and reserves 10.0.0.5.
	if _, err := svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: iface.ID, AllowedIPs: []string{"10.0.0.5/32"}}); err != nil {
		t.Fatalf("AddPeer(valid): %v", err)
	}

	cases := []struct {
		name    string
		ips     []string
		wantSub string
	}{
		{"duplicate of another peer", []string{"10.0.0.5/32"}, "already in use"},
		{"the interface's own address", []string{"10.0.0.1/32"}, "already in use"},
		{"outside the subnet", []string{"10.99.0.5/32"}, "subnet"},
		{"two identical in one request", []string{"10.0.0.9/32", "10.0.0.9/32"}, "already in use"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: iface.ID, AllowedIPs: tc.ips})
			if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("expected error containing %q, got %v", tc.wantSub, err)
			}
		})
	}

	// A site-to-site peer: an in-subnet host address plus a routed LAN CIDR that
	// legitimately falls outside the interface subnet — must be accepted.
	if _, err := svc.AddPeer(user.ID.String(), AddPeerInput{
		InterfaceID: iface.ID,
		AllowedIPs:  []string{"10.0.0.7/32", "192.168.50.0/24"},
	}); err != nil {
		t.Fatalf("AddPeer(site-to-site route): unexpected error: %v", err)
	}
}

// TestUpdateInterfaceAllowsUnchangedFieldsDespiteConflict verifies that editing
// an interface's unrelated fields still works when a conflicting sibling exists
// (introduced here directly, as the tunnel/import bypass paths can): only changed
// fields are re-validated.
func TestUpdateInterfaceAllowsUnchangedFieldsDespiteConflict(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	wg0, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface wg0: %v", err)
	}
	wg1, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg1", Address: "10.0.1.1/24", ListenPort: 51821,
	})
	if err != nil {
		t.Fatalf("CreateInterface wg1: %v", err)
	}

	// Inject a port conflict directly (bypassing validation, as BuildTunnel /
	// ImportInterface do): give wg1 the same listen port as wg0.
	wg1.ListenPort = 51820
	if err := svc.store.Servers().Interfaces(srv.ID).Set(wg1); err != nil {
		t.Fatalf("inject conflict: %v", err)
	}

	// Editing wg0's unrelated field (its DNS) keeps its name/port/subnet — the
	// unchanged port must not be re-checked against the now-conflicting wg1.
	cfg := wg0.InterfaceConfig
	cfg.DNS = []string{"1.1.1.1"}
	if _, err := svc.UpdateInterfaceConfig(srv.ID.String(), wg0.ID.String(), cfg); err != nil {
		t.Fatalf("UpdateInterfaceConfig(unchanged port, conflicting sibling): %v", err)
	}
}

// TestNextFreeIPHandlesSlash31 verifies auto-assign works on an RFC 3021
// point-to-point /31 (both addresses usable, no network/broadcast to skip).
func TestNextFreeIPHandlesSlash31(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	iface, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.0/31", ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}
	user, err := svc.CreateUser(UserInput{Name: "u1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// The interface uses 10.0.0.0; the peer should auto-get the other address.
	if _, err := svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: iface.ID}); err != nil {
		t.Fatalf("AddPeer(auto-assign on /31): %v", err)
	}
}
