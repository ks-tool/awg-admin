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
	"reflect"
	"testing"

	"github.com/ks-tool/awg-admin/agent/models"

	"github.com/Jipok/wgctrl-go/wgtypes"
)

func TestOrphanInterfacesNoMismatch(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}, {Interface: "wg1"}}
	devices := []*wgtypes.Device{{Name: "wg0"}, {Name: "wg1"}}

	got := orphanInterfaces(configs, devices)
	if len(got) != 0 {
		t.Fatalf("orphanInterfaces() = %v, want none", got)
	}
}

func TestOrphanInterfacesFindsUnknownDevice(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}}
	devices := []*wgtypes.Device{{Name: "wg0"}, {Name: "wg-leftover"}}

	got := orphanInterfaces(configs, devices)
	want := []string{"wg-leftover"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("orphanInterfaces() = %v, want %v", got, want)
	}
}

func TestOrphanInterfacesNoDevices(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}}
	got := orphanInterfaces(configs, nil)
	if len(got) != 0 {
		t.Fatalf("orphanInterfaces() = %v, want none", got)
	}
}

// A config with no matching live device (e.g. the agent's storage still
// has a record InterfaceCreate hasn't been retried for yet) is not this
// function's concern — it only reports devices the host has that storage
// doesn't, not the reverse.
func TestOrphanInterfacesIgnoresMissingDevice(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}, {Interface: "wg1"}}
	devices := []*wgtypes.Device{{Name: "wg0"}}

	got := orphanInterfaces(configs, devices)
	if len(got) != 0 {
		t.Fatalf("orphanInterfaces() = %v, want none", got)
	}
}

func TestUnmanagedInterfacesReportsKindSorted(t *testing.T) {
	configs := []models.InterfaceConfig{{Interface: "wg0"}}
	devices := []*wgtypes.Device{
		{Name: "wg0", IsAmnezia: true},
		{Name: "wg-plain"},
		{Name: "awg1", IsAmnezia: true},
	}

	got := unmanagedInterfaces(configs, devices)
	want := []models.UnmanagedInterface{
		{Name: "awg1", Kind: models.InterfaceKindAmnezia},
		{Name: "wg-plain", Kind: models.InterfaceKindWireGuard},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unmanagedInterfaces() = %v, want %v", got, want)
	}
}

// An empty result must still be a non-nil slice: the admin's UI reads .length
// off the JSON, and a nil slice would marshal as null.
func TestUnmanagedInterfacesEmptyIsNotNil(t *testing.T) {
	got := unmanagedInterfaces([]models.InterfaceConfig{{Interface: "wg0"}}, []*wgtypes.Device{{Name: "wg0"}})
	if got == nil || len(got) != 0 {
		t.Fatalf("unmanagedInterfaces() = %#v, want empty non-nil slice", got)
	}
}

func TestReconcilePeersUpdatesInPlaceAndRemovesStalePeers(t *testing.T) {
	keepKey := wgtypes.Key{1}
	removeKey := wgtypes.Key{2}
	addKey := wgtypes.Key{3}
	psk := wgtypes.Key{4}

	got := reconcilePeers(
		[]wgtypes.Peer{
			{PublicKey: keepKey, PresharedKey: psk},
			{PublicKey: removeKey},
		},
		[]wgtypes.PeerConfig{
			{PublicKey: keepKey},
			{PublicKey: addKey},
		},
	)

	if len(got) != 3 {
		t.Fatalf("reconcilePeers() returned %d configs, want 3: %+v", len(got), got)
	}
	byKey := make(map[wgtypes.Key]wgtypes.PeerConfig, len(got))
	for _, peer := range got {
		byKey[peer.PublicKey] = peer
	}
	removed, exists := byKey[removeKey]
	if !exists || !removed.Remove {
		t.Fatalf("stale peer config = %+v, want removal of %v", removed, removeKey)
	}
	updated, exists := byKey[keepKey]
	if !exists || updated.Remove || !updated.ReplaceAllowedIPs {
		t.Fatalf("existing peer config = %+v, want in-place update with replaced AllowedIPs", updated)
	}
	if updated.PresharedKey == nil || *updated.PresharedKey != (wgtypes.Key{}) {
		t.Fatalf("existing peer PSK = %v, want explicit clear", updated.PresharedKey)
	}
	added, exists := byKey[addKey]
	if !exists || added.Remove || !added.ReplaceAllowedIPs {
		t.Fatalf("new peer config = %+v, want in-place add with replaced AllowedIPs", added)
	}
}
