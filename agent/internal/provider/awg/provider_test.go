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
	"sync"
	"testing"
	"time"

	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/storage"
	"github.com/ks-tool/vpn-agent/storage/fs"
)

type fakeNetwork struct {
	mu       sync.Mutex
	previous *models.InterfaceConfig
	config   models.InterfaceConfig
	deleted  string
}

func (f *fakeNetwork) One(previous *models.InterfaceConfig, config models.InterfaceConfig) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.previous, f.config = previous, config
	return nil
}
func (f *fakeNetwork) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = name
	return nil
}
func (f *fakeNetwork) deletedName() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deleted
}

func TestInterfaceSchemaAndGenericLifecycle(t *testing.T) {
	store, err := fs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	network := &fakeNetwork{}
	provider, err := New(store, t.TempDir(), network, "test")
	if err != nil {
		t.Fatal(err)
	}
	definition := provider.Definitions()[0]
	schema := string(definition.SpecSchema)
	for _, field := range []string{`"pk"`, `"peers"`, `"preUp"`, `"postUp"`, `"preDown"`, `"postDown"`} {
		if strings.Contains(schema, field) {
			t.Fatalf("private or lifecycle field %s leaked into schema", field)
		}
	}
	if definition.SchemaHash == "" || len(definition.Ports) != 1 {
		t.Fatalf("incomplete definition: %+v", definition)
	}

	resourceStore, err := resource.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime := resource.NewRuntime(resourceStore, "agent", time.Millisecond, time.Hour)
	if err := runtime.Register(provider); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	created, err := runtime.Put("awg", "Interface", "office", models.Resource{
		APIVersion: APIVersion,
		Spec:       json.RawMessage(`{"iface":"awg0","listen":51820,"addr":"10.0.0.1/24","mtu":1420,"dns":["1.1.1.1"]}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForProviderStatus(t, runtime, models.ResourceReady)
	if models.IsEmpty(network.config.PrivateKey) {
		t.Fatal("agent did not generate a private key")
	}
	if len(network.config.Peers) != 0 || len(network.config.PreUp) != 0 {
		t.Fatalf("unexpected peers or hooks in new interface: %+v", network.config)
	}
	current, err := runtime.Get("awg", "Interface", "office")
	if err != nil {
		t.Fatal(err)
	}
	var providerStatus map[string]any
	if err := json.Unmarshal(current.Status.Provider, &providerStatus); err != nil {
		t.Fatal(err)
	}
	if providerStatus["publicKey"] != publicKeyString(network.config.PrivateKey) {
		t.Fatalf("unexpected public status: %+v", providerStatus)
	}
	if strings.Contains(string(current.Spec), network.config.PrivateKey.String()) || strings.Contains(string(current.Status.Provider), network.config.PrivateKey.String()) {
		t.Fatal("private key leaked into resource response")
	}
	firstKey := network.config.PrivateKey
	if _, err := provider.Reconcile(context.Background(), created); err != nil {
		t.Fatal(err)
	}
	if network.config.PrivateKey != firstKey {
		t.Fatal("key changed during idempotent reconcile")
	}

	if err := runtime.Delete("awg", "Interface", "office"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for network.deletedName() == "" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if network.deletedName() != "awg0" {
		t.Fatalf("interface was not deleted: %q", network.deletedName())
	}
	if _, err := store.Get("awg0"); !storage.IsNotFound(err) {
		t.Fatalf("legacy config survived deletion: %v", err)
	}
}

func TestReconcilePreservesLegacySecretsPeersAndHooks(t *testing.T) {
	store, err := fs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := models.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	peerKey, err := models.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	legacy := models.InterfaceConfig{
		Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, PrivateKey: privateKey,
		Peers: []models.InterfacePeer{{Key: peerKey.PublicKey(), AllowedIPs: []string{"10.0.0.2/32"}}},
		PreUp: []string{"pre-up"}, PostUp: []string{"post-up"}, PreDown: []string{"pre-down"}, PostDown: []string{"post-down"},
	}
	if err := store.Set(&legacy); err != nil {
		t.Fatal(err)
	}
	network := &fakeNetwork{}
	provider, err := New(store, t.TempDir(), network, "test")
	if err != nil {
		t.Fatal(err)
	}
	resource := models.Resource{ID: "resource", APIVersion: APIVersion, Kind: "Interface", Name: "office", Spec: json.RawMessage(`{"iface":"awg0","listen":51820,"addr":"10.0.0.2/24"}`)}
	if err := provider.Validate(context.Background(), resource); err != nil {
		t.Fatal(err)
	}
	status, err := provider.Reconcile(context.Background(), resource)
	if err != nil {
		t.Fatal(err)
	}
	if network.previous == nil || network.config.PrivateKey != privateKey || len(network.config.Peers) != 1 {
		t.Fatalf("legacy config was not preserved: %+v", network.config)
	}
	if len(network.config.PreUp) != 1 || len(network.config.PostDown) != 1 {
		t.Fatalf("legacy hooks were not preserved: %+v", network.config)
	}
	if strings.Contains(string(status), privateKey.String()) {
		t.Fatal("legacy private key leaked into status")
	}
}

func TestInterfaceValidation(t *testing.T) {
	store, err := fs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	existing := models.InterfaceConfig{Interface: "awg-existing", Address: "10.0.0.1/24", ListenPort: 51820}
	if err := store.Set(&existing); err != nil {
		t.Fatal(err)
	}
	provider, err := New(store, t.TempDir(), &fakeNetwork{}, "test")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		spec string
		want string
	}{
		{"missing port", `{"iface":"awg0","addr":"10.1.0.1/24"}`, "listen"},
		{"unknown field", `{"iface":"awg0","listen":51821,"addr":"10.1.0.1/24","privateKey":"secret"}`, "unknown field"},
		{"bad address", `{"iface":"awg0","listen":51821,"addr":"bad"}`, "CIDR"},
		{"bad dns", `{"iface":"awg0","listen":51821,"addr":"10.1.0.1/24","dns":["bad"]}`, "DNS"},
		{"duplicate port", `{"iface":"awg0","listen":51820,"addr":"10.1.0.1/24"}`, "already used"},
		{"overlap", `{"iface":"awg0","listen":51821,"addr":"10.0.0.2/24"}`, "overlaps"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := provider.Validate(context.Background(), models.Resource{Spec: json.RawMessage(test.spec)})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func waitForProviderStatus(t *testing.T, runtime *resource.Runtime, phase models.ResourcePhase) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		current, err := runtime.Get("awg", "Interface", "office")
		if err == nil && current.Status.Phase == phase {
			return
		}
		time.Sleep(time.Millisecond)
	}
	current, _ := runtime.Get("awg", "Interface", "office")
	t.Fatalf("provider status did not reach %s: %+v", phase, current.Status)
}

func TestInterfaceSecretMigrationPreservesKeysHooksAndZeroListenPort(t *testing.T) {
	store, err := fs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	network := &fakeNetwork{}
	provider, err := New(store, t.TempDir(), network, "test")
	if err != nil {
		t.Fatal(err)
	}
	item := models.Resource{ID: "legacy-interface", Kind: "Interface", Name: "legacy", Spec: json.RawMessage(`{"iface":"awg0","listen":0,"addr":"10.0.0.2/24"}`)}
	if err := provider.Validate(context.Background(), item); err != nil {
		t.Fatalf("zero listen port was rejected: %v", err)
	}
	privateKey, err := models.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(map[string]any{
		"privateKey": privateKey.String(), "preUp": []string{"pre-up"}, "postUp": []string{"post-up"},
		"preDown": []string{"pre-down"}, "postDown": []string{"post-down"},
	})
	if _, err := provider.Action(context.Background(), item, "importSecret", arguments); err != nil {
		t.Fatal(err)
	}
	if network.config.PrivateKey != privateKey || len(network.config.PreUp) != 1 || len(network.config.PostUp) != 1 || len(network.config.PreDown) != 1 || len(network.config.PostDown) != 1 {
		t.Fatalf("legacy secrets and hooks were not imported: %+v", network.config)
	}
	if _, err := provider.Reconcile(context.Background(), item); err != nil {
		t.Fatal(err)
	}
	if network.config.PrivateKey != privateKey || len(network.config.PreUp) != 1 {
		t.Fatal("reconcile discarded imported state")
	}
}

func TestInterfaceResourceRejectsDuplicateOperatingSystemName(t *testing.T) {
	store, err := fs.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	inventory, err := resource.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	first := models.Resource{ID: "first", APIVersion: APIVersion, Kind: "Interface", Name: "first", Spec: json.RawMessage(`{"iface":"awg0","listen":51820,"addr":"10.0.0.1/24"}`)}
	if err := inventory.Put(resource.StoredResource{Provider: "awg", Resource: first}); err != nil {
		t.Fatal(err)
	}
	provider, err := New(store, t.TempDir(), &fakeNetwork{}, "test", inventory)
	if err != nil {
		t.Fatal(err)
	}
	second := models.Resource{ID: "second", Kind: "Interface", Name: "second", Spec: first.Spec}
	if err := provider.Validate(context.Background(), second); err == nil || !strings.Contains(err.Error(), "already managed") {
		t.Fatalf("duplicate operating system interface was accepted: %v", err)
	}
}
