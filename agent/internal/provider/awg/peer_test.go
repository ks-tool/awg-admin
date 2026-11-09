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

package awg

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/ks-tool/vpn-admin/agent/internal/resource"
	"github.com/ks-tool/vpn-admin/agent/models"
	"github.com/ks-tool/vpn-admin/agent/storage"
	"github.com/ks-tool/vpn-admin/agent/storage/fs"
)

func TestPeerLifecycleActionsAndSecretHandling(t *testing.T) {
	store, err := fs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	interfaceKey := mustPeerKey(t)
	legacy := models.InterfaceConfig{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, PrivateKey: interfaceKey}
	if err := store.Set(&legacy); err != nil {
		t.Fatal(err)
	}
	inventory, err := resource.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	network := &fakeNetwork{}
	provider, err := New(store, t.TempDir(), network, "test", inventory)
	if err != nil {
		t.Fatal(err)
	}
	runtime := resource.NewRuntime(inventory, "agent", time.Millisecond, time.Hour)
	if err := runtime.Register(provider); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	if _, err := runtime.Put("awg", "Interface", "office", models.Resource{APIVersion: APIVersion, Spec: json.RawMessage(`{"iface":"awg0","listen":51820,"addr":"10.0.0.1/24"}`)}); err != nil {
		t.Fatal(err)
	}
	waitForProviderStatus(t, runtime, models.ResourceReady)
	events, unsubscribe := runtime.Subscribe()
	defer unsubscribe()
	created, err := runtime.Put("awg", "Peer", "alice-laptop", models.Resource{
		APIVersion: APIVersion,
		Spec:       json.RawMessage(`{"name":"laptop","interfaceRef":"office","userRef":"alice","allowedIPs":["10.0.0.2/32"],"dns":["1.1.1.1"],"keepaliveSeconds":25}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForPeerGeneration(t, runtime, 2)
	current, err := runtime.Get("awg", "Peer", "alice-laptop")
	if err != nil {
		t.Fatal(err)
	}
	var spec PeerSpec
	if err := json.Unmarshal(current.Spec, &spec); err != nil {
		t.Fatal(err)
	}
	if spec.PublicKey == "" || current.Generation != 2 || current.Status.Phase != models.ResourceReady {
		t.Fatalf("peer key was not published safely: %+v", current)
	}
	if strings.Contains(string(current.Spec), `"privateKey"`) || strings.Contains(string(current.Status.Provider), `"privateKey"`) {
		t.Fatal("private key field leaked into resource")
	}
	select {
	case event := <-events:
		if strings.Contains(string(mustPeerJSON(t, event)), "privateKey") {
			t.Fatal("private key field leaked into event")
		}
	default:
	}

	result, err := runtime.Action(context.Background(), "awg", "Peer", "alice-laptop", "renderClientConfig", json.RawMessage(`{"endpoint":"192.0.2.1:51820"}`))
	if err != nil {
		t.Fatal(err)
	}
	var config string
	if err := json.Unmarshal(result.Data, &config); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(config, "[Interface]") || !strings.Contains(config, "Endpoint = 192.0.2.1:51820") || !strings.Contains(config, publicKeyString(interfaceKey)) {
		t.Fatalf("unexpected client config: %s", config)
	}

	if _, err := runtime.Action(context.Background(), "awg", "Peer", "alice-laptop", "rotateKeys", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	waitForPeerGeneration(t, runtime, 3)
	rotated, _ := runtime.Get("awg", "Peer", "alice-laptop")
	var rotatedSpec PeerSpec
	_ = json.Unmarshal(rotated.Spec, &rotatedSpec)
	if rotatedSpec.PublicKey == spec.PublicKey {
		t.Fatal("peer key did not rotate")
	}
	if _, err := runtime.Action(context.Background(), "awg", "Peer", "alice-laptop", "revoke", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	waitForPeerGeneration(t, runtime, 4)
	revoked, _ := runtime.Get("awg", "Peer", "alice-laptop")
	var revokedSpec PeerSpec
	_ = json.Unmarshal(revoked.Spec, &revokedSpec)
	if !revokedSpec.Disabled {
		t.Fatal("revoke did not update desired state")
	}

	if err := runtime.Delete("awg", "Peer", "alice-laptop"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var storedInterface *models.InterfaceConfig
	for time.Now().Before(deadline) {
		storedInterface, err = store.Get("awg0")
		if err == nil && len(storedInterface.Peers) == 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := runtime.Get("awg", "Peer", "alice-laptop"); !storage.IsNotFound(err) {
		t.Fatalf("peer was not deleted: %v", err)
	}
	if err != nil {
		t.Fatalf("interface was removed with peer: %v", err)
	}
	if len(storedInterface.Peers) != 0 {
		t.Fatalf("peer survived deletion: %+v", storedInterface.Peers)
	}
	_ = created
}

func TestPeerSecretMigration(t *testing.T) {
	store, err := fs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	interfaceKey := mustPeerKey(t)
	peerKey := mustPeerKey(t)
	legacy := models.InterfaceConfig{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, PrivateKey: interfaceKey, Peers: []models.InterfacePeer{{Key: peerKey.PublicKey(), AllowedIPs: []string{"10.0.0.2/32"}}}}
	if err := store.Set(&legacy); err != nil {
		t.Fatal(err)
	}
	inventory, err := resource.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	interfaceResource := models.Resource{ID: "interface-id", APIVersion: APIVersion, Kind: "Interface", Name: "office", Generation: 1, Spec: json.RawMessage(`{"iface":"awg0","listen":51820,"addr":"10.0.0.1/24"}`)}
	if err := inventory.Put(resource.StoredResource{Provider: "awg", Resource: interfaceResource}); err != nil {
		t.Fatal(err)
	}
	peerSpec := PeerSpec{Name: "laptop", InterfaceRef: "interface-id", UserRef: "alice", PublicKey: publicKeyString(peerKey), AllowedIPs: []string{"10.0.0.2/32"}}
	rawSpec, _ := json.Marshal(peerSpec)
	peerResource := models.Resource{ID: "peer-id", APIVersion: APIVersion, Kind: "Peer", Name: "alice-laptop", Generation: 1, Spec: rawSpec}
	if err := inventory.Put(resource.StoredResource{Provider: "awg", Resource: peerResource}); err != nil {
		t.Fatal(err)
	}
	provider, err := New(store, t.TempDir(), &fakeNetwork{}, "test", inventory)
	if err != nil {
		t.Fatal(err)
	}
	status, err := provider.Reconcile(context.Background(), peerResource)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), `"secretMigrationPending":true`) {
		t.Fatalf("migration should remain pending: %s", status)
	}
	if _, err := provider.Action(context.Background(), peerResource, "renderClientConfig", json.RawMessage(`{"endpoint":"192.0.2.1:51820"}`)); err == nil {
		t.Fatal("client config rendered before private key migration")
	}
	arguments, _ := json.Marshal(map[string]string{"privateKey": peerKey.String()})
	if _, err := provider.Action(context.Background(), peerResource, "importSecret", arguments); err != nil {
		t.Fatal(err)
	}
	updated, err := inventory.Get("awg", "Peer", "alice-laptop")
	if err != nil {
		t.Fatal(err)
	}
	status, err = provider.Reconcile(context.Background(), updated.Resource)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(status), `"secretMigrationPending":false`) {
		t.Fatalf("migration did not complete: %s", status)
	}
	result, err := provider.Action(context.Background(), updated.Resource, "renderClientConfig", json.RawMessage(`{"endpoint":"192.0.2.1:51820"}`))
	if err != nil {
		t.Fatal(err)
	}
	var config string
	_ = json.Unmarshal(result.Data, &config)
	if !strings.Contains(config, peerKey.String()) {
		t.Fatal("imported private key was not used for client config")
	}
}

func waitForPeerGeneration(t *testing.T, runtime *resource.Runtime, generation int64) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		item, err := runtime.Get("awg", "Peer", "alice-laptop")
		if err == nil && item.Generation == generation && item.Status.Phase == models.ResourceReady && item.Status.ObservedGeneration == generation {
			return
		}
		time.Sleep(time.Millisecond)
	}
	item, _ := runtime.Get("awg", "Peer", "alice-laptop")
	t.Fatalf("peer did not reach generation %d: %+v", generation, item)
}

func mustPeerKey(t *testing.T) models.Key {
	t.Helper()
	key, err := models.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func mustPeerJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
