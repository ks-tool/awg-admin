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
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"uuid"

	awgmodels "github.com/ks-tool/vpn-agent/v2/providers/awg/models"

	"github.com/ks-tool/vpn-admin/models"

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

func TestLegacyRegistrySourceSplitsIntoAgentAndProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	raw, err := bolt.Open(path, 0o600, nil)
	if err != nil {
		t.Fatal(err)
	}
	id := uuid.New()
	err = raw.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bktRegistrySources)
		if err := bucket.ForEach(func(key, value []byte) error { return bucket.Delete(key) }); err != nil {
			return err
		}
		return put(bucket, idKey(id), map[string]any{
			"id": id, "name": "Private", "url": "oci://registry.example:5000",
			"agentRepository":    "oci://registry.example:5000/team/agent",
			"providerRepository": "oci://registry.example:5000/team/providers",
			"username":           "reader", "password": "secret", "plainHttp": true,
		})
	})
	if closeErr := raw.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	sources, err := db.RegistrySources().List()
	if err != nil || len(sources) != 2 {
		t.Fatalf("sources = %+v, %v", sources, err)
	}
	if sources[0].ID != id || sources[0].Type != models.RegistrySourceAgent || sources[0].URL != "oci://registry.example:5000/team/agent" || sources[0].Password != "secret" {
		t.Fatalf("agent source = %+v", sources[0])
	}
	if sources[1].Type != models.RegistrySourceProvider || sources[1].URL != "oci://registry.example:5000/team/providers" || !sources[1].PlainHTTP {
		t.Fatalf("provider source = %+v", sources[1])
	}
}

func TestManualMigrationPreservesSourceAndMarksServersUnmanaged(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "legacy.db")
	destination := filepath.Join(root, ".vpn-admin", "db")
	fixture := createLegacyFixture(t, source, false)
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateV2(source, destination, false); err != nil {
		t.Fatalf("MigrateV2: %v", err)
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("v1 source database was modified")
	}

	db, err := Open(destination)
	if err != nil {
		t.Fatalf("Open migrated database: %v", err)
	}
	if version, err := db.SchemaVersion(); err != nil || version != currentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, %v", version, err)
	}
	server, err := db.Servers().Get(fixture.serverA)
	if err != nil {
		t.Fatal(err)
	}
	if !server.Unmanaged || server.Info.Location != "Moscow" || server.SSH.KeyData != "private-ssh-key" || server.Agent.Address != "127.0.0.1:8080" {
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
	registrySources, err := db.RegistrySources().List()
	if err != nil || len(registrySources) != 2 || registrySources[0].URL != "oci://ghcr.io/ks-tool/vpn-agent" || registrySources[0].Type != models.RegistrySourceAgent || registrySources[1].URL != "oci://ghcr.io/ks-tool/vpn-providers" || registrySources[1].Type != models.RegistrySourceProvider {
		t.Fatalf("default OCI sources were not initialized during migration: %+v, %v", registrySources, err)
	}
	for _, source := range registrySources {
		if err := db.RegistrySources().Delete(source.ID); err != nil {
			t.Fatal(err)
		}
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
	if interfaceResource.Status.Phase != models.ResourcePending || !hasUnmanagedCondition(interfaceResource.Status.Conditions) {
		t.Fatalf("interface was not marked unmanaged: %+v", interfaceResource.Status)
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
	if links[0].ID != fixture.tunnel.String() || links[0].Source.ResourceID == links[0].Target.ResourceID || links[0].Status.Phase != models.LinkPending {
		t.Fatalf("unexpected migrated link: %+v", links[0])
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(destination)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer func() { _ = db.Close() }()
	registrySources, err = db.RegistrySources().List()
	if err != nil || len(registrySources) != 0 {
		t.Fatalf("deleted OCI registry was initialized again: %+v, %v", registrySources, err)
	}
	resourcesA, _ = db.Resources(fixture.serverA).List()
	resourcesB, _ = db.Resources(fixture.serverB).List()
	links, _ = db.Links().List()
	if len(resourcesA) != 2 || len(resourcesB) != 1 || len(links) != 1 {
		t.Fatalf("reopening duplicated data: A=%d B=%d links=%d", len(resourcesA), len(resourcesB), len(links))
	}
}

func TestLegacyDatabaseWithSchemaVersionTwoMigrates(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "legacy.db")
	destination := filepath.Join(root, "v2.db")
	createLegacyFixture(t, source, false)
	raw, err := bolt.Open(source, 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Update(func(tx *bolt.Tx) error {
		if _, err := tx.CreateBucketIfNotExists(bktMeta); err != nil {
			return err
		}
		return tx.Bucket(bktMeta).Put(schemaVersionKey, []byte("2"))
	}); err != nil {
		_ = raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(source); err == nil {
		t.Fatal("legacy database with schemaVersion=2 was opened without migration")
	}
	if err := MigrateV2(source, destination, false); err != nil {
		t.Fatalf("MigrateV2 with legacy schemaVersion=2: %v", err)
	}
	db, err := Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if version, err := db.SchemaVersion(); err != nil || version != currentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, %v", version, err)
	}
}

func TestV1DatabaseRequiresManualMigration(t *testing.T) {
	source := filepath.Join(t.TempDir(), "legacy.db")
	createLegacyFixture(t, source, false)
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(source); err == nil {
		t.Fatal("v1 database was opened without manual migration")
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("opening v1 database modified it")
	}
}

func TestFailedMigrationLeavesSourceAndDestinationUntouched(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "corrupt.db")
	destination := filepath.Join(root, "v2.db")
	createLegacyFixture(t, source, true)
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := MigrateV2(source, destination, false); err == nil {
		t.Fatal("corrupted database migrated without an error")
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("partial destination survived failed migration: %v", err)
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("failed migration modified v1 source")
	}
}

func TestMigrationDoesNotReplaceDestinationWithoutForce(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "legacy.db")
	destination := filepath.Join(root, "v2.db")
	createLegacyFixture(t, source, false)
	if err := os.WriteFile(destination, []byte("existing"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := MigrateV2(source, destination, false); err == nil {
		t.Fatal("existing destination was replaced without force")
	}
	content, err := os.ReadFile(destination)
	if err != nil || string(content) != "existing" {
		t.Fatalf("destination changed: %q, %v", content, err)
	}
	if err := MigrateV2(source, destination, true); err != nil {
		t.Fatalf("forced migration: %v", err)
	}
	db, err := Open(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
}

func TestMigrationRejectsCurrentV2Database(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "v2.db")
	destination := filepath.Join(root, "copy.db")
	db, err := Open(source)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := MigrateV2(source, destination, false); err == nil || !strings.Contains(err.Error(), "already a v2 database") {
		t.Fatalf("current v2 database was not identified: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination was created for an already migrated source: %v", err)
	}
}

func TestNewDatabaseStartsAtCurrentSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if version, err := db.SchemaVersion(); err != nil || version != currentSchemaVersion {
		t.Fatalf("SchemaVersion = %d, %v", version, err)
	}
}

func hasUnmanagedCondition(conditions []models.Condition) bool {
	for _, condition := range conditions {
		if condition.Type == "Managed" && condition.Status == models.ConditionFalse && condition.Reason == "Unmanaged" {
			return true
		}
	}
	return false
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
		ifaceA := models.Interface{ID: fixture.ifaceA, Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, Table: 100, PrivateKey: privateKey, Peers: []awgmodels.InterfacePeer{{Key: peerKey.PublicKey(), AllowedIPs: []string{"10.0.0.2/32"}}}, PreUp: []string{"private-hook"}, InSync: true, LastSyncedAt: time.Now(), Tunnel: &fixture.tunnel}
		ifaceB := models.Interface{ID: fixture.ifaceB, Interface: "awg0", Address: "10.0.0.2/24", PrivateKey: mustKey(t), InSync: true, Tunnel: &fixture.tunnel}
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
		_ = raw.Close()
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}
	return fixture
}
