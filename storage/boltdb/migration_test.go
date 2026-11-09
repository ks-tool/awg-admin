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
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentmodels "github.com/ks-tool/vpn-admin/agent/models"
	"github.com/ks-tool/vpn-admin/models"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

type legacyFixture struct {
	serverA uuid.UUID
	serverB uuid.UUID
	ifaceA  uuid.UUID
	ifaceB  uuid.UUID
	tunnel  uuid.UUID
	user    uuid.UUID
}

func TestLegacyMigrationPreservesDataAndIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	fixture := createLegacyFixture(t, path, false)
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open migrated database: %v", err)
	}
	if version, err := db.SchemaVersion(); err != nil || version != currentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, %v", version, err)
	}
	if _, err := os.Stat(path + ".v1.backup"); err != nil {
		t.Fatalf("migration backup: %v", err)
	}

	server, err := db.Servers().Get(fixture.serverA)
	if err != nil {
		t.Fatal(err)
	}
	if server.Info.Location != "Moscow" || server.SSH.KeyData != "private-ssh-key" || server.Agent.Address != "127.0.0.1:8080" {
		t.Fatalf("server settings changed: %+v", server)
	}
	topology, err := db.Topology().Get()
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Servers) != 1 || topology.Servers[0].Latitude == nil || *topology.Servers[0].Latitude != 55.7558 {
		t.Fatalf("topology coordinates changed: %+v", topology.Servers)
	}
	auth, err := db.Auth().Get()
	if err != nil || auth.PasswordHash != "existing-password-hash" {
		t.Fatalf("auth changed: %+v, %v", auth, err)
	}

	resourcesA, err := db.Resources(fixture.serverA).List()
	if err != nil {
		t.Fatal(err)
	}
	resourcesB, err := db.Resources(fixture.serverB).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(resourcesA) != 2 || len(resourcesB) != 1 {
		t.Fatalf("unexpected migrated resources: A=%d B=%d", len(resourcesA), len(resourcesB))
	}
	var interfaceResource, peerResource *models.Resource
	for index := range resourcesA {
		switch resourcesA[index].Kind {
		case "Interface":
			interfaceResource = &resourcesA[index]
		case "Peer":
			peerResource = &resourcesA[index]
		}
	}
	if interfaceResource == nil || peerResource == nil {
		t.Fatalf("Interface and Peer were not migrated: %+v", resourcesA)
	}
	if interfaceResource.ID != fixture.ifaceA.String() || interfaceResource.ServerID != fixture.serverA.String() {
		t.Fatalf("interface identity changed: %+v", interfaceResource)
	}
	if strings.Contains(string(interfaceResource.Spec), "private-awg-key") || strings.Contains(string(interfaceResource.Spec), `"pk"`) || strings.Contains(string(interfaceResource.Spec), `"peers"`) {
		t.Fatalf("private key or peers leaked into interface spec: %s", interfaceResource.Spec)
	}
	var peerSpec map[string]any
	if err := json.Unmarshal(peerResource.Spec, &peerSpec); err != nil {
		t.Fatal(err)
	}
	if peerSpec["interfaceRef"] != fixture.ifaceA.String() || peerSpec["userRef"] != fixture.user.String() {
		t.Fatalf("peer references are wrong: %+v", peerSpec)
	}
	if !strings.Contains(string(peerResource.Status.Provider), "secretMigrationPending") {
		t.Fatalf("peer secret migration state is missing: %s", peerResource.Status.Provider)
	}
	links, err := db.Links().List()
	if err != nil || len(links) != 1 {
		t.Fatalf("migrated links: %+v, %v", links, err)
	}
	if links[0].ID != fixture.tunnel.String() || links[0].Source.ResourceID == links[0].Target.ResourceID {
		t.Fatalf("unexpected migrated link: %+v", links[0])
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer db.Close()
	resourcesA, _ = db.Resources(fixture.serverA).List()
	resourcesB, _ = db.Resources(fixture.serverB).List()
	links, _ = db.Links().List()
	if len(resourcesA) != 2 || len(resourcesB) != 1 || len(links) != 1 {
		t.Fatalf("second migration duplicated data: A=%d B=%d links=%d", len(resourcesA), len(resourcesB), len(links))
	}
}

func TestLegacyMigrationRollsBackCorruptedConversion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.db")
	createLegacyFixture(t, path, true)
	if _, err := Open(path); err == nil {
		t.Fatal("corrupted database migrated without an error")
	}
	if _, err := os.Stat(path + ".v1.backup"); err != nil {
		t.Fatalf("backup was not retained after rollback: %v", err)
	}
	raw, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if err := raw.View(func(tx *bolt.Tx) error {
		if tx.Bucket(bktResources) != nil || tx.Bucket(bktMeta) != nil {
			t.Fatal("new buckets survived rolled back migration")
		}
		if tx.Bucket(bktServers) == nil {
			t.Fatal("legacy servers were removed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestNewDatabaseStartsAtCurrentSchemaWithoutBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if version, err := db.SchemaVersion(); err != nil || version != currentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, %v", version, err)
	}
	if _, err := os.Stat(path + ".v1.backup"); !os.IsNotExist(err) {
		t.Fatalf("new database unexpectedly has a migration backup: %v", err)
	}
}

func createLegacyFixture(t *testing.T, path string, corruptUsers bool) legacyFixture {
	t.Helper()
	raw, err := bolt.Open(path, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	fixture := legacyFixture{serverA: uuid.New(), serverB: uuid.New(), ifaceA: uuid.New(), ifaceB: uuid.New(), tunnel: uuid.New(), user: uuid.New()}
	privateKey := mustKey(t)
	peerKey := mustKey(t)
	latitude := 55.7558
	if err := raw.Update(func(tx *bolt.Tx) error {
		for _, bucket := range [][]byte{bktServers, bktInterfaces, bktUsers, bktAuth, bktTopology} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		serverA := models.Server{ID: fixture.serverA, Name: "a", Info: models.ServerInfo{Location: "Moscow"}, SSH: models.SSHConfig{Host: "192.0.2.1", KeyData: "private-ssh-key"}, Agent: models.Agent{Address: "127.0.0.1:8080"}, Interfaces: []uuid.UUID{fixture.ifaceA}}
		serverB := models.Server{ID: fixture.serverB, Name: "b", Interfaces: []uuid.UUID{fixture.ifaceB}}
		if err := put(tx.Bucket(bktServers), idKey(fixture.serverA), serverA); err != nil {
			return err
		}
		if err := put(tx.Bucket(bktServers), idKey(fixture.serverB), serverB); err != nil {
			return err
		}
		ifaceA := models.Interface{ID: fixture.ifaceA, InterfaceConfig: agentmodels.InterfaceConfig{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, Table: 100, PrivateKey: privateKey, Peers: []agentmodels.InterfacePeer{{Key: peerKey.PublicKey(), AllowedIPs: []string{"10.0.0.2/32"}}}, PreUp: []string{"private-hook"}}, InSync: true, LastSyncedAt: time.Now(), Tunnel: &fixture.tunnel}
		ifaceB := models.Interface{ID: fixture.ifaceB, InterfaceConfig: agentmodels.InterfaceConfig{Interface: "awg0", Address: "10.0.0.2/24", PrivateKey: mustKey(t)}, InSync: true, Tunnel: &fixture.tunnel}
		if err := put(tx.Bucket(bktInterfaces), idKey(fixture.ifaceA), ifaceA); err != nil {
			return err
		}
		if err := put(tx.Bucket(bktInterfaces), idKey(fixture.ifaceB), ifaceB); err != nil {
			return err
		}
		if corruptUsers {
			return tx.Bucket(bktUsers).Put(idKey(fixture.user), []byte(`{"id":"not-a-uuid"}`))
		}
		user := models.User{ID: fixture.user, Name: "alice", Peers: []models.Peer{{Name: "laptop", PrivateKey: peerKey, InterfaceId: fixture.ifaceA, DNS: []string{"1.1.1.1"}}}}
		if err := put(tx.Bucket(bktUsers), idKey(fixture.user), user); err != nil {
			return err
		}
		if err := put(tx.Bucket(bktAuth), authRecordKey, &models.AuthCredentials{Username: "admin", PasswordHash: "existing-password-hash"}); err != nil {
			return err
		}
		topology := models.Topology{Servers: []models.TopologyServer{{ID: fixture.serverA, Location: "Moscow", Latitude: &latitude}}, Connections: []models.NetworkConnection{{ID: uuid.New(), SourceID: fixture.ifaceA, TargetID: fixture.ifaceB}}}
		return put(tx.Bucket(bktTopology), topologyRecordKey, topology)
	}); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	return fixture
}
