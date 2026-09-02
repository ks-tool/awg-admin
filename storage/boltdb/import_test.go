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

package boltdb

import (
	"path/filepath"
	"strings"
	"testing"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage"

	"github.com/google/uuid"
)

func openTestDB(t *testing.T) *BoltDB {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func mustKey(t *testing.T) agentmodels.Key {
	t.Helper()
	k, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestImportWritesInterfaceUsersAndPeersAtomically(t *testing.T) {
	db := openTestDB(t)
	srv := &models.Server{ID: uuid.New(), Name: "srv"}
	if err := db.Servers().Set(srv); err != nil {
		t.Fatal(err)
	}
	alice := &models.User{ID: uuid.New(), Name: "alice"}
	if err := db.Users().Set(alice); err != nil {
		t.Fatal(err)
	}

	ifKey, k1, k2, k3 := mustKey(t), mustKey(t), mustKey(t), mustKey(t)
	iface := &models.Interface{ID: uuid.New(), InterfaceConfig: agentmodels.InterfaceConfig{
		Interface: "wg0", PrivateKey: ifKey, Address: "10.0.0.1/24", ListenPort: 51820,
		Peers: []agentmodels.InterfacePeer{
			{Key: k1.PublicKey(), AllowedIPs: []string{"10.0.0.2/32"}},
			{Key: k2.PublicKey(), AllowedIPs: []string{"10.0.0.3/32"}},
			{Key: k3.PublicKey(), AllowedIPs: []string{"10.0.0.4/32"}}, // embedded: no user
		},
	}}
	peers := []storage.ImportPeer{
		{UserName: "alice", Peer: models.Peer{Name: "laptop", PrivateKey: k1}},
		{UserName: "bob", Peer: models.Peer{Name: "pc", PrivateKey: k2}},
	}
	if err := db.Servers().Interfaces(srv.ID).Import(iface, peers); err != nil {
		t.Fatalf("Import: %v", err)
	}

	// The interface keeps its full peer list (Set would have wiped it) and is
	// listed under the server.
	got, err := db.Servers().Interfaces(srv.ID).Get(iface.ID)
	if err != nil {
		t.Fatalf("Get interface: %v", err)
	}
	if len(got.Peers) != 3 || got.Interface != "wg0" {
		t.Fatalf("stored interface = %+v", got)
	}
	list, _ := db.Servers().Interfaces(srv.ID).List()
	if len(list) != 1 || list[0].ID != iface.ID {
		t.Fatalf("server interfaces = %+v", list)
	}

	// alice existed (reused, not duplicated); bob was created; each has its peer
	// tagged with the interface id and looked up by public key.
	users, _ := db.Users().List()
	if len(users) != 2 {
		t.Fatalf("users = %+v, want alice + bob", users)
	}
	for _, u := range users {
		switch u.Name {
		case "alice":
			if u.ID != alice.ID || len(u.Peers) != 1 || u.Peers[0].Name != "laptop" || u.Peers[0].InterfaceId != iface.ID {
				t.Fatalf("alice = %+v", u)
			}
			if _, err := db.Users().Peers(u.ID).Get(k1.PublicKey()); err != nil {
				t.Fatalf("alice peer lookup by pubkey: %v", err)
			}
		case "bob":
			if len(u.Peers) != 1 || u.Peers[0].Name != "pc" || u.Peers[0].PrivateKey != k2 || u.Peers[0].InterfaceId != iface.ID {
				t.Fatalf("bob = %+v", u)
			}
		default:
			t.Fatalf("unexpected user %q", u.Name)
		}
	}
}

func TestImportAmbiguousUserWritesNothing(t *testing.T) {
	db := openTestDB(t)
	srv := &models.Server{ID: uuid.New(), Name: "srv"}
	if err := db.Servers().Set(srv); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := db.Users().Set(&models.User{ID: uuid.New(), Name: "carol"}); err != nil {
			t.Fatal(err)
		}
	}
	k1, k2 := mustKey(t), mustKey(t)
	iface := &models.Interface{ID: uuid.New(), InterfaceConfig: agentmodels.InterfaceConfig{
		Interface: "wg0", PrivateKey: mustKey(t), Address: "10.0.0.1/24", ListenPort: 51820,
		Peers: []agentmodels.InterfacePeer{{Key: k1.PublicKey()}, {Key: k2.PublicKey()}},
	}}
	peers := []storage.ImportPeer{
		{UserName: "dave", Peer: models.Peer{Name: "new", PrivateKey: k1}},  // would be created…
		{UserName: "carol", Peer: models.Peer{Name: "amb", PrivateKey: k2}}, // …but this one is ambiguous
	}
	err := db.Servers().Interfaces(srv.ID).Import(iface, peers)
	if err == nil || !strings.Contains(err.Error(), "matches 2 users") {
		t.Fatalf("err = %v, want ambiguity error", err)
	}

	// All-or-nothing: no interface, no dave, and the server's list untouched.
	if _, err := db.Servers().Interfaces(srv.ID).Get(iface.ID); !storage.IsNotFound(err) {
		t.Fatalf("interface after failed import: err = %v, want not found", err)
	}
	users, _ := db.Users().List()
	if len(users) != 2 {
		t.Fatalf("users after failed import = %+v, want only the two carols", users)
	}
	list, _ := db.Servers().Interfaces(srv.ID).List()
	if len(list) != 0 {
		t.Fatalf("server interfaces after failed import = %+v", list)
	}
}

func TestImportRejectsExistingInterfaceID(t *testing.T) {
	db := openTestDB(t)
	srv := &models.Server{ID: uuid.New(), Name: "srv"}
	if err := db.Servers().Set(srv); err != nil {
		t.Fatal(err)
	}
	iface := &models.Interface{ID: uuid.New(), InterfaceConfig: agentmodels.InterfaceConfig{Interface: "wg0", PrivateKey: mustKey(t)}}
	if err := db.Servers().Interfaces(srv.ID).Set(iface); err != nil {
		t.Fatal(err)
	}
	if err := db.Servers().Interfaces(srv.ID).Import(iface, nil); !storage.IsAlreadyExists(err) {
		t.Fatalf("err = %v, want already exists", err)
	}
}
