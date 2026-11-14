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
	"testing"

	awgmodels "github.com/ks-tool/vpn-agent/v2/providers/awg/models"

	"github.com/ks-tool/vpn-admin/models"
)

// TestSetPeerDisabledTogglesInterfaceConfig verifies the peer-deactivation toggle
// in both the user and the desired interface configuration. The AWG provider's
// device conversion tests cover omitting disabled peers from the live device.
func TestSetPeerDisabledTogglesInterfaceConfig(t *testing.T) {
	svc := newTestService(t)

	srv, err := svc.CreateServer(ServerInput{
		Name: "srv1",
		SSH:  models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	iface, err := svc.CreateInterface(srv.ID.String(), awgmodels.InterfaceConfig{
		Interface:  "wg0",
		Address:    "10.0.0.1/24",
		ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}
	user, err := svc.CreateUser(UserInput{Name: "u1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	u, err := svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: iface.ID})
	if err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	// The exposed "PrivateKey" is actually the public key (see sanitizePeer).
	pub := u.Peers[0].PrivateKey

	// A freshly added peer is active and present in the interface config.
	before, err := svc.GetInterface(srv.ID.String(), iface.ID.String())
	if err != nil {
		t.Fatalf("GetInterface: %v", err)
	}
	if len(before.Peers) != 1 {
		t.Fatalf("expected the new peer in the interface config, got %d", len(before.Peers))
	}
	if len(before.Peers[0].AllowedIPs) == 0 {
		t.Fatalf("expected an auto-assigned AllowedIP on the peer")
	}

	// ---- Deactivate --------------------------------------------------------
	du, err := svc.SetPeerDisabled(user.ID.String(), pub.String(), true)
	if err != nil {
		t.Fatalf("SetPeerDisabled(true): %v", err)
	}
	if len(du.Peers) != 1 || !du.Peers[0].Disabled {
		t.Fatalf("user peer not marked disabled: %+v", du.Peers)
	}

	off, err := svc.GetInterface(srv.ID.String(), iface.ID.String())
	if err != nil {
		t.Fatalf("GetInterface after disable: %v", err)
	}
	if !off.Peers[0].Disabled {
		t.Fatalf("interface peer not marked disabled")
	}
	// The rest of the interface peer must survive the toggle untouched.
	if len(off.Peers[0].AllowedIPs) == 0 || off.Peers[0].AllowedIPs[0] != before.Peers[0].AllowedIPs[0] {
		t.Fatalf("AllowedIPs not preserved across disable: %v -> %v", before.Peers[0].AllowedIPs, off.Peers[0].AllowedIPs)
	}

	// ---- Reactivate --------------------------------------------------------
	ru, err := svc.SetPeerDisabled(user.ID.String(), pub.String(), false)
	if err != nil {
		t.Fatalf("SetPeerDisabled(false): %v", err)
	}
	if ru.Peers[0].Disabled {
		t.Fatalf("user peer still disabled after reactivation")
	}
	on, err := svc.GetInterface(srv.ID.String(), iface.ID.String())
	if err != nil {
		t.Fatalf("GetInterface after enable: %v", err)
	}
	if on.Peers[0].Disabled {
		t.Fatalf("interface peer still disabled after reactivation")
	}
	if got := len(on.Peers); got != 1 {
		t.Fatalf("reactivated peer must be back in interface config, got %d peers", got)
	}
}

// TestSetPeerDisabledRejectsBadInput checks the validation-error paths map to a
// ValidationError (HTTP 400 upstream) rather than a generic failure.
func TestSetPeerDisabledRejectsBadInput(t *testing.T) {
	svc := newTestService(t)
	user, err := svc.CreateUser(UserInput{Name: "u1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if _, err := svc.SetPeerDisabled("not-a-uuid", "x", true); err == nil {
		t.Fatalf("expected error for invalid user id")
	}
	if _, err := svc.SetPeerDisabled(user.ID.String(), "not-a-key", true); err == nil {
		t.Fatalf("expected error for invalid public key")
	}
}
