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

package models

import "testing"

// TestDisabledPeerIsOmittedFromDeviceConfig is the core of the peer-deactivation
// feature: a Disabled InterfacePeer stays in the stored InterfaceConfig (desired
// state) but must not appear in the device config applied via wgctrl. Combined
// with ReplacePeers=true (see service.Handler.One), that removes it from the live
// interface. Re-enabling it brings it back.
func TestDisabledPeerIsOmittedFromDeviceConfig(t *testing.T) {
	activeKey, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	offKey, err := GeneratePrivateKey()
	if err != nil {
		t.Fatalf("GeneratePrivateKey: %v", err)
	}
	activePub := activeKey.PublicKey()
	offPub := offKey.PublicKey()

	cfg := InterfaceConfig{
		Interface: "wg0",
		Peers: []InterfacePeer{
			{Key: activePub, AllowedIPs: []string{"10.0.0.2/32"}},
			{Key: offPub, AllowedIPs: []string{"10.0.0.3/32"}, Disabled: true},
		},
	}

	got := cfg.ToAmneziaConfig()
	if len(got.Peers) != 1 {
		t.Fatalf("ToAmneziaConfig: expected 1 active peer, got %d", len(got.Peers))
	}
	if got.Peers[0].PublicKey != activePub.WGKey() {
		t.Fatalf("ToAmneziaConfig kept the wrong peer: %v", got.Peers[0].PublicKey)
	}

	// ToWireguardPeers must filter identically.
	if wp := cfg.ToWireguardPeers(); len(wp) != 1 || wp[0].PublicKey != activePub.WGKey() {
		t.Fatalf("ToWireguardPeers did not filter the disabled peer: %+v", wp)
	}

	// Reactivating brings the peer back into the device config.
	cfg.Peers[1].Disabled = false
	if got := cfg.ToAmneziaConfig(); len(got.Peers) != 2 {
		t.Fatalf("expected 2 peers after reactivating, got %d", len(got.Peers))
	}
}
