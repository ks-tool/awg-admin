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
	"encoding/json"
	"reflect"
	"testing"

	awgmodels "github.com/ks-tool/vpn-agent/v2/providers/awg/models"

	"github.com/ks-tool/vpn-admin/models"
)

func TestLegacyPeerSpecAllocatesAddressAndOmitsEmptyDNS(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	iface, err := svc.CreateInterface(srv.ID.String(), awgmodels.InterfaceConfig{
		Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820,
	})
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.CreateUser(UserInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := awgmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	peer := &models.Peer{Name: "laptop", PrivateKey: privateKey, InterfaceId: iface.ID}
	if err := svc.store.Users().Peers(user.ID).Set(peer); err != nil {
		t.Fatal(err)
	}
	iface.Peers = []awgmodels.InterfacePeer{{Key: privateKey.PublicKey(), AllowedIPs: []string{}}}

	raw, configured, err := svc.legacyPeerSpec(user, peer, iface)
	if err != nil {
		t.Fatal(err)
	}
	wantIPs := []string{"10.0.0.2/32"}
	if !reflect.DeepEqual(configured.AllowedIPs, wantIPs) {
		t.Fatalf("configured allowedIPs = %#v, want %#v", configured.AllowedIPs, wantIPs)
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatal(err)
	}
	if _, ok := spec["dns"]; ok {
		t.Fatalf("empty DNS must be omitted: %s", raw)
	}
	var allowedIPs []string
	if err := json.Unmarshal(spec["allowedIPs"], &allowedIPs); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allowedIPs, wantIPs) {
		t.Fatalf("spec allowedIPs = %#v, want %#v", allowedIPs, wantIPs)
	}
}
