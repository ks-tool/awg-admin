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
	"strings"
	"testing"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
	agentmodels "github.com/ks-tool/vpn-agent/models"
)

func TestProviderResourceLifecycleAndSchemaCache(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	serverID := server.ID.String()
	manifest, err := svc.GetProviderManifest(serverID)
	if err != nil || manifest.AgentID != "agent" || len(manifest.Providers) != 1 {
		t.Fatalf("GetProviderManifest: %+v, %v", manifest, err)
	}
	definition, err := svc.GetProviderSchema(serverID, "test")
	if err != nil || len(definition.Resources) != 1 {
		t.Fatalf("GetProviderSchema: %+v, %v", definition, err)
	}
	if _, err := svc.GetProviderSchema(serverID, "test"); err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	calls := agent.schemaCalls
	agent.mu.Unlock()
	if calls != 1 {
		t.Fatalf("schema cache missed: %d requests", calls)
	}

	spec := json.RawMessage(`{"nested":{"enabled":true}}`)
	created, err := svc.PutProviderResource(serverID, "test", "Service", "public", spec)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "resource-id" || created.Generation != 1 || string(created.Spec) != string(spec) {
		t.Fatalf("unexpected created resource: %+v", created)
	}
	updated, err := svc.PutProviderResource(serverID, "test", "Service", "public", json.RawMessage(`{"nested":{"enabled":false}}`))
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Generation != 2 {
		t.Fatalf("identity or generation changed incorrectly: %+v", updated)
	}
	resources, err := svc.ListProviderResources(serverID, "test")
	if err != nil || len(resources) != 1 {
		t.Fatalf("ListProviderResources: %+v, %v", resources, err)
	}
	status, err := svc.GetProviderResourceStatus(serverID, "test", "Service", "public")
	if err != nil || status.ObservedGeneration != 2 {
		t.Fatalf("GetProviderResourceStatus: %+v, %v", status, err)
	}
	action, err := svc.RunProviderResourceAction(serverID, "test", "Service", "public", "inspect", json.RawMessage(`{}`))
	if err != nil || string(action.Data) != `{"ok":true}` {
		t.Fatalf("RunProviderResourceAction: %+v, %v", action, err)
	}
	if err := svc.DeleteProviderResource(serverID, "test", "Service", "public"); err != nil {
		t.Fatal(err)
	}
	resources, err = svc.ListProviderResources(serverID, "test")
	if err != nil || len(resources) != 0 {
		t.Fatalf("resource was not deleted: %+v, %v", resources, err)
	}
}

func TestMigratedSecretIsSentOnlyThroughAction(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	key, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.CreateUser(UserInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	user.Peers = []models.Peer{{Name: "laptop", PrivateKey: key}}
	if err := svc.store.Users().Set(user); err != nil {
		t.Fatal(err)
	}
	publicKey := key.PublicKey()
	spec, _ := json.Marshal(map[string]any{"userRef": user.ID.String(), "publicKey": publicKey.String()})
	item := models.Resource{
		ID: "pending-peer", ServerID: server.ID.String(), APIVersion: "test/v1", Kind: "Service", Name: "alice-laptop", Generation: 1, Spec: spec,
		Status: models.ResourceStatus{Conditions: []models.Condition{{Type: "Secrets", Status: models.ConditionFalse, Reason: "SecretMigrationPending"}}},
	}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	resources, err := svc.ListProviderResources(server.ID.String(), "test")
	if err != nil || len(resources) != 1 {
		t.Fatalf("ListProviderResources: %+v, %v", resources, err)
	}
	agent.mu.Lock()
	calls := agent.actionCalls
	agent.mu.Unlock()
	if calls != 1 {
		t.Fatalf("secret import action calls = %d, want 1", calls)
	}
	if !json.Valid(resources[0].Spec) {
		t.Fatal("invalid resource spec")
	}
	if strings.Contains(string(resources[0].Spec), key.String()) {
		t.Fatal("private key leaked into resource spec")
	}
}

func TestServerMigrationCompletesOnlyWithAWGProvider(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	server.Unmanaged = true
	if err := svc.store.Servers().Set(server); err != nil {
		t.Fatal(err)
	}
	item := models.Resource{
		ID: "migrated-interface", ServerID: server.ID.String(), APIVersion: "awg/v1alpha1", Kind: "Interface", Name: "awg0", Generation: 1,
		Spec:   json.RawMessage(`{"iface":"awg0","addr":"10.0.0.1/24","listen":51820}`),
		Status: models.ResourceStatus{Phase: models.ResourcePending, Conditions: []models.Condition{{Type: "Managed", Status: models.ConditionFalse, Reason: "Unmanaged"}}},
	}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}

	status, err := svc.ServerAgentStatus(server.ID.String())
	if err != nil || status != models.AgentStatusUnmanaged {
		t.Fatalf("status without AWG provider = %q, %v", status, err)
	}
	stored, err := svc.store.Servers().Get(server.ID)
	if err != nil || !stored.Unmanaged {
		t.Fatalf("server became managed without AWG provider: %+v, %v", stored, err)
	}

	agent.mu.Lock()
	agent.awg = true
	agent.mu.Unlock()
	status, err = svc.ServerAgentStatus(server.ID.String())
	if err != nil || status != models.AgentStatusOK {
		t.Fatalf("status after AWG provider registration = %q, %v", status, err)
	}
	stored, err = svc.store.Servers().Get(server.ID)
	if err != nil || stored.Unmanaged {
		t.Fatalf("server migration did not complete: %+v, %v", stored, err)
	}
	agent.mu.Lock()
	resourceCount := len(agent.resources)
	agent.mu.Unlock()
	if resourceCount != 1 {
		t.Fatalf("migrated resources on agent = %d, want 1", resourceCount)
	}
}

func TestMigratedResourcesAreSubmittedInterfaceFirst(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	agent.mu.Lock()
	agent.awg = true
	agent.mu.Unlock()
	privateKey, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	interfaceID := uuid.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")
	iface := &models.Interface{ID: interfaceID, InterfaceConfig: agentmodels.InterfaceConfig{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, PrivateKey: privateKey}}
	if err := svc.store.Servers().Interfaces(server.ID).Set(iface); err != nil {
		t.Fatal(err)
	}
	peer := models.Resource{ID: "00000000-0000-0000-0000-000000000000", ServerID: server.ID.String(), APIVersion: "awg/v1alpha1", Kind: "Peer", Name: "peer", Spec: json.RawMessage(`{}`)}
	item := models.Resource{ID: interfaceID.String(), ServerID: server.ID.String(), APIVersion: "awg/v1alpha1", Kind: "Interface", Name: "awg0", Spec: json.RawMessage(`{"iface":"awg0","addr":"10.0.0.1/24","listen":51820}`)}
	if err := svc.saveProviderResource(server.ID.String(), peer); err != nil {
		t.Fatal(err)
	}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListProviderResources(server.ID.String(), "awg"); err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	order := append([]string(nil), agent.putOrder...)
	agent.mu.Unlock()
	if len(order) != 2 || order[0] != "Interface" || order[1] != "Peer" {
		t.Fatalf("resources were submitted in dependency-breaking order: %v", order)
	}
}

func TestServerStatusReflectsDegradedProviderResources(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	agent.mu.Lock()
	agent.awg = true
	agent.resources["Interface/awg0"] = models.Resource{
		ID: "resource", ServerID: server.ID.String(), APIVersion: "awg/v1alpha1", Kind: "Interface", Name: "awg0", Generation: 1,
		Status: models.ResourceStatus{ObservedGeneration: 1, Phase: models.ResourceDegraded},
	}
	agent.mu.Unlock()
	item := models.Resource{ID: "resource", ServerID: server.ID.String(), APIVersion: "awg/v1alpha1", Kind: "Interface", Name: "awg0", Generation: 1, Spec: json.RawMessage(`{"iface":"awg0","addr":"10.0.0.1/24","listen":51820}`)}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	status, err := svc.ServerAgentStatus(server.ID.String())
	if err != nil || status != models.AgentStatusDegraded {
		t.Fatalf("degraded resource was hidden by server status %q, %v", status, err)
	}
}

func TestProviderPeerDeleteAlsoRemovesLegacyCredentials(t *testing.T) {
	svc, server, _ := newTestServerWithAgent(t)
	interfaceKey, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	peerKey, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.CreateUser(UserInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	ifaceID := uuid.New()
	iface := &models.Interface{ID: ifaceID, InterfaceConfig: agentmodels.InterfaceConfig{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, PrivateKey: interfaceKey}}
	if err := svc.store.Servers().Interfaces(server.ID).Set(iface); err != nil {
		t.Fatal(err)
	}
	ifacePeer := agentmodels.InterfacePeer{Key: peerKey.PublicKey(), AllowedIPs: []string{"10.0.0.2/32"}}
	peer := &models.Peer{Name: "laptop", PrivateKey: peerKey, InterfaceId: ifaceID}
	if err := svc.store.Users().Peers(user.ID).Set(peer); err != nil {
		t.Fatal(err)
	}
	iface.Peers = append(iface.Peers, ifacePeer)
	if err := svc.store.Servers().Interfaces(server.ID).Set(iface); err != nil {
		t.Fatal(err)
	}
	publicKey := peerKey.PublicKey()
	spec, _ := json.Marshal(map[string]any{"name": "laptop", "interfaceRef": ifaceID.String(), "userRef": user.ID.String(), "publicKey": publicKey.String(), "allowedIPs": []string{"10.0.0.2/32"}})
	item := models.Resource{ID: uuid.New().String(), ServerID: server.ID.String(), APIVersion: "awg/v1alpha1", Kind: "Peer", Name: "alice-laptop", Generation: 1, Spec: spec}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProviderResource(server.ID.String(), "awg", "Peer", item.Name); err != nil {
		t.Fatal(err)
	}
	storedUser, err := svc.store.Users().Get(user.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedInterface, err := svc.store.Servers().Interfaces(server.ID).Get(ifaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(storedUser.Peers) != 0 || len(storedInterface.Peers) != 0 {
		t.Fatalf("legacy credentials survived provider deletion: user=%d interface=%d", len(storedUser.Peers), len(storedInterface.Peers))
	}
}
