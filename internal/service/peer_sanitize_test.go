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

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
)

// TestPeerEndpointsExposeOnlyThePublicKey is a regression test for a bug
// where models.Peer.PrivateKey (json:"pk") was returned as-is to callers
// outside this package, so the frontend's "pk" field actually held the
// peer's *private* key. Operations that look a peer up by what the
// frontend believed was its public key (QR code, delete) then always
// failed with "not found", since the value sent never matched the peer's
// real public key.
func TestPeerEndpointsExposeOnlyThePublicKey(t *testing.T) {
	svc := newTestService(t)

	srv, err := svc.CreateServer(ServerInput{
		Name: "srv1",
		SSH:  models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	iface, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
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

	updatedUser, err := svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: iface.ID})
	if err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	if len(updatedUser.Peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(updatedUser.Peers))
	}
	exposedKey := updatedUser.Peers[0].PrivateKey // field name is historical, value must be public

	// The exposed key must be usable wherever a public key is expected —
	// it must NOT be a private key, and it must NOT successfully act as a
	// private key signer for the interface's own peer entry.
	ifaceAfter, err := svc.GetInterface(srv.ID.String(), iface.ID.String())
	if err != nil {
		t.Fatalf("GetInterface: %v", err)
	}
	if len(ifaceAfter.Peers) != 1 {
		t.Fatalf("expected 1 InterfacePeer, got %d", len(ifaceAfter.Peers))
	}
	if exposedKey != ifaceAfter.Peers[0].Key {
		t.Fatalf("exposed peer key %q does not match the interface's real public key %q — frontend would send the wrong key", exposedKey, ifaceAfter.Peers[0].Key)
	}

	// GetPeer/ListPeers must expose the same public key, not the private one.
	got, err := svc.GetPeer(user.ID.String(), exposedKey.String())
	if err != nil {
		t.Fatalf("GetPeer with exposed key: %v", err)
	}
	if got.PrivateKey != exposedKey {
		t.Fatalf("GetPeer returned a different key than ListPeers/AddPeer exposed: %q vs %q", got.PrivateKey, exposedKey)
	}

	list, err := svc.ListPeers(user.ID.String())
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(list) != 1 || list[0].PrivateKey != exposedKey {
		t.Fatalf("ListPeers exposed a different key: %+v", list)
	}

	// The exact bug: fetching the QR code/config with the key the frontend
	// was given must succeed, not fail with "not found".
	if _, err := svc.GetPeerConfig(user.ID.String(), exposedKey.String()); err != nil {
		t.Fatalf("GetPeerConfig with the exposed key failed (this is the reported bug): %v", err)
	}
	if _, err := svc.GetPeerQRCode(user.ID.String(), exposedKey.String()); err != nil {
		t.Fatalf("GetPeerQRCode with the exposed key failed: %v", err)
	}

	// DeletePeer must also accept the exposed key.
	if _, err := svc.DeletePeer(user.ID.String(), exposedKey.String()); err != nil {
		t.Fatalf("DeletePeer with the exposed key failed: %v", err)
	}
}
