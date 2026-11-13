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
	"fmt"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
	agentmodels "github.com/ks-tool/vpn-agent/v2/models"
)

func TestProviderResourceLifecycleAndSchemaCache(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	serverID := server.ID.String()
	manifest, err := svc.GetProviderManifest(serverID)
	if err != nil || len(manifest.Providers) != 1 {
		t.Fatalf("GetProviderManifest: %+v, %v", manifest, err)
	}
	definition, err := svc.GetProviderSchema(serverID, "test")
	if err != nil || len(definition.Resources) != 3 {
		t.Fatalf("GetProviderSchema: %+v, %v", definition, err)
	}
	if _, err := svc.GetProviderSchema(serverID, "test"); err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	calls := agent.schemaCalls
	agent.mu.Unlock()
	if calls != 0 {
		t.Fatalf("file schema cache missed: %d requests", calls)
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

func TestRefreshProviderManifestReplacesCachedProviders(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	agent.mu.Lock()
	agent.awg = true
	agent.mu.Unlock()
	manifest, err := svc.GetProviderManifest(server.ID.String())
	if err != nil || len(manifest.Providers) != 2 {
		t.Fatalf("initial manifest: %+v, %v", manifest, err)
	}

	agent.mu.Lock()
	agent.awg = false
	agent.mu.Unlock()
	cached, err := svc.GetProviderManifest(server.ID.String())
	if err != nil || len(cached.Providers) != 2 {
		t.Fatalf("expected the previous cached manifest: %+v, %v", cached, err)
	}
	refreshed, err := svc.RefreshProviderManifest(server.ID.String())
	if err != nil || len(refreshed.Providers) != 1 || refreshed.Providers[0].Name != "test" {
		t.Fatalf("refreshed manifest: %+v, %v", refreshed, err)
	}
	cached, err = svc.GetProviderManifest(server.ID.String())
	if err != nil || len(cached.Providers) != 1 || cached.Providers[0].Name != "test" {
		t.Fatalf("removed provider returned from cache: %+v, %v", cached, err)
	}
}

func TestWaitForProviderRequiresManifestRegistration(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	agent.mu.Lock()
	agent.awg = true
	agent.mu.Unlock()
	if err := svc.waitForProvider(server.ID.String(), "awg", time.Second); err != nil {
		t.Fatalf("registered provider was not accepted: %v", err)
	}

	agent.mu.Lock()
	agent.awg = false
	agent.mu.Unlock()
	err := svc.waitForProvider(server.ID.String(), "awg", 0)
	if err == nil || !strings.Contains(err.Error(), "did not appear in agent manifest") {
		t.Fatalf("unregistered provider reported a successful deployment: %v", err)
	}
}

func TestWaitForProviderReportsDiscoveryError(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	agent.mu.Lock()
	agent.awg = false
	agent.providerErrors = map[string]string{"awg": "create TUN failed"}
	agent.mu.Unlock()
	err := svc.waitForProvider(server.ID.String(), "awg", 0)
	if err == nil || !strings.Contains(err.Error(), "create TUN failed") {
		t.Fatalf("discovery error was not reported: %v", err)
	}
}

func TestWaitForProviderRemovalRequiresManifestDeregistration(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	agent.mu.Lock()
	agent.awg = false
	agent.mu.Unlock()
	if err := svc.waitForProviderRemoval(server.ID.String(), "awg", time.Second); err != nil {
		t.Fatalf("unregistered provider was not accepted: %v", err)
	}

	agent.mu.Lock()
	agent.awg = true
	agent.mu.Unlock()
	err := svc.waitForProviderRemoval(server.ID.String(), "awg", 0)
	if err == nil || !strings.Contains(err.Error(), "did not disappear from agent manifest") {
		t.Fatalf("registered provider reported a successful removal: %v", err)
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

func TestProviderInterfaceAppearsInUserInterfaceList(t *testing.T) {
	svc, server, _ := newTestServerWithAgent(t)
	interfaceID := uuid.New()
	item := models.Resource{
		ID: interfaceID.String(), ServerID: server.ID.String(), APIVersion: "awg.ks-tool.dev/v1", Kind: "Interface", Name: "awg0", Generation: 1,
		Spec: json.RawMessage(`{"iface":"awg0","addr":"10.0.0.1/24","listen":51820}`),
	}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	interfaces, err := svc.ListInterfaces(server.ID.String())
	if err != nil || len(interfaces) != 1 {
		t.Fatalf("interfaces = %+v, %v", interfaces, err)
	}
	if interfaces[0].ID != interfaceID || interfaces[0].Interface != "awg0" || interfaces[0].Address != "10.0.0.1/24" {
		t.Fatalf("unexpected interface: %+v", interfaces[0])
	}
}

func TestGenericProviderPeerLifecycle(t *testing.T) {
	svc, server, _ := newTestServerWithAgent(t)
	interfaceID := uuid.New()
	item := models.Resource{
		ID: interfaceID.String(), ServerID: server.ID.String(), APIVersion: "test/v1", Kind: "Interface", Name: "test0", Generation: 1,
		Spec: json.RawMessage(`{"name":"test0","listen":"0.0.0.0","port":443}`),
	}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	user, err := svc.CreateUser(UserInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.AddPeer(user.ID.String(), AddPeerInput{Name: "phone", InterfaceID: interfaceID, Spec: map[string]any{"custom": "value"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Peers) != 1 || updated.Peers[0].Provider != "test" || len(updated.Peers[0].ResourceName) == 0 {
		t.Fatalf("unexpected generic peer: %+v", updated.Peers)
	}
	peerKey := updated.Peers[0].PrivateKey.String()
	config, err := svc.GetPeerConfig(user.ID.String(), peerKey)
	if err != nil || config != "test://client" {
		t.Fatalf("GetPeerConfig = %q, %v", config, err)
	}
	updated, err = svc.SetPeerDisabled(user.ID.String(), peerKey, true)
	if err != nil || !updated.Peers[0].Disabled {
		t.Fatalf("SetPeerDisabled: %+v, %v", updated, err)
	}
	updated, err = svc.DeletePeer(user.ID.String(), peerKey)
	if err != nil || len(updated.Peers) != 0 {
		t.Fatalf("DeletePeer: %+v, %v", updated, err)
	}
}

func TestServerMigrationCompletesOnlyWithAWGProvider(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	server.Unmanaged = true
	if err := svc.store.Servers().Set(server); err != nil {
		t.Fatal(err)
	}
	item := models.Resource{
		ID: "migrated-interface", ServerID: server.ID.String(), APIVersion: "awg.ks-tool.dev/v1", Kind: "Interface", Name: "awg0", Generation: 1,
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

func TestMigrationNormalizesLegacyAWGAPIVersion(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	agent.mu.Lock()
	agent.awg = true
	interfaceID := uuid.New()
	agent.resources["awg0"] = models.Resource{
		ID: interfaceID.String(), ServerID: server.ID.String(), APIVersion: "awg/v1alpha1", Kind: "Interface", Name: "awg0", Generation: 1,
		Spec:   json.RawMessage(`{"iface":"awg0","addr":"10.0.0.1/24","listen":51820}`),
		Status: models.ResourceStatus{ObservedGeneration: 1, Phase: models.ResourceReady},
	}
	agent.mu.Unlock()

	resources, err := svc.ListProviderResources(server.ID.String(), "awg")
	if err != nil {
		t.Fatal(err)
	}
	if len(resources) != 1 || resources[0].APIVersion != "awg.ks-tool.dev/v1" {
		t.Fatalf("unexpected migrated resources: %+v", resources)
	}
	stored, err := svc.store.Resources(server.ID).Get(interfaceID.String())
	if err != nil || stored.APIVersion != "awg.ks-tool.dev/v1" {
		t.Fatalf("stored resource was not normalized: %+v, %v", stored, err)
	}
	iface, err := svc.store.Servers().Interfaces(server.ID).Get(interfaceID)
	if err != nil || iface.Provider != "awg" || iface.ListenPort != 51820 {
		t.Fatalf("legacy interface projection is incorrect: %+v, %v", iface, err)
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
	iface := &models.Interface{ID: interfaceID, Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, PrivateKey: privateKey}
	if err := svc.store.Servers().Interfaces(server.ID).Set(iface); err != nil {
		t.Fatal(err)
	}
	peer := models.Resource{ID: "00000000-0000-0000-0000-000000000000", ServerID: server.ID.String(), APIVersion: "awg.ks-tool.dev/v1", Kind: "Peer", Name: "peer", Spec: json.RawMessage(`{}`)}
	item := models.Resource{ID: interfaceID.String(), ServerID: server.ID.String(), APIVersion: "awg.ks-tool.dev/v1", Kind: "Interface", Name: "awg0", Spec: json.RawMessage(`{"iface":"awg0","addr":"10.0.0.1/24","listen":51820}`)}
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
		ID: "resource", ServerID: server.ID.String(), APIVersion: "awg.ks-tool.dev/v1", Kind: "Interface", Name: "awg0", Generation: 1,
		Status: models.ResourceStatus{ObservedGeneration: 1, Phase: models.ResourceDegraded},
	}
	agent.mu.Unlock()
	item := models.Resource{ID: "resource", ServerID: server.ID.String(), APIVersion: "awg.ks-tool.dev/v1", Kind: "Interface", Name: "awg0", Generation: 1, Spec: json.RawMessage(`{"iface":"awg0","addr":"10.0.0.1/24","listen":51820}`)}
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
	iface := &models.Interface{ID: ifaceID, Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820, PrivateKey: interfaceKey}
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
	item := models.Resource{ID: uuid.New().String(), ServerID: server.ID.String(), APIVersion: "awg.ks-tool.dev/v1", Kind: "Peer", Name: "alice-laptop", Generation: 1, Spec: spec}
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

func TestDeleteUserRevokesProviderPeers(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	interfaceID := uuid.New()
	item := models.Resource{
		ID: interfaceID.String(), ServerID: server.ID.String(), APIVersion: "test/v1", Kind: "Interface", Name: "test0", Generation: 1,
		Spec: json.RawMessage(`{"name":"test0","listen":"0.0.0.0","port":443}`),
	}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	user, err := svc.CreateUser(UserInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.AddPeer(user.ID.String(), AddPeerInput{Name: "phone", InterfaceID: interfaceID})
	if err != nil {
		t.Fatal(err)
	}
	resourceName := updated.Peers[0].ResourceName
	agent.mu.Lock()
	_, exists := agent.resources["Peer/"+resourceName]
	agent.mu.Unlock()
	if !exists {
		t.Fatal("provider peer was not created on the agent")
	}
	if err := svc.DeleteUser(user.ID.String()); err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	_, exists = agent.resources["Peer/"+resourceName]
	agent.mu.Unlock()
	if exists {
		t.Fatal("provider credentials survived user deletion")
	}
	if _, err := svc.store.Users().Get(user.ID); err == nil {
		t.Fatal("local user survived deletion")
	}
}

func TestDeleteUserRevokesAdoptedAWGPeer(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	privateKey, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	user, err := svc.CreateUser(UserInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	interfaceID := uuid.New()
	iface := &models.Interface{ID: interfaceID, Interface: "awg0", Address: "10.0.0.1/24", PrivateKey: privateKey}
	if err := svc.store.Servers().Interfaces(server.ID).Set(iface); err != nil {
		t.Fatal(err)
	}
	peer := &models.Peer{Name: "laptop", PrivateKey: privateKey, InterfaceId: interfaceID}
	if err := svc.store.Users().Peers(user.ID).Set(peer); err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.PublicKey()
	spec, _ := json.Marshal(map[string]any{"interfaceRef": interfaceID.String(), "userRef": user.ID.String(), "publicKey": publicKey.String()})
	item := models.Resource{ID: uuid.New().String(), ServerID: server.ID.String(), APIVersion: "awg.ks-tool.dev/v1", Kind: "Peer", Name: "alice-laptop", Generation: 1, Spec: spec}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	agent.resources["Peer/"+item.Name] = item
	agent.mu.Unlock()
	if err := svc.DeleteUser(user.ID.String()); err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	_, exists := agent.resources["Peer/"+item.Name]
	agent.mu.Unlock()
	if exists {
		t.Fatal("adopted AWG credentials survived user deletion")
	}
	if _, err := svc.store.Resources(server.ID).Get(item.ID); err == nil {
		t.Fatal("adopted AWG resource survived user deletion")
	}
}

func TestGenericLegacyMutationsUseResourceProvider(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	interfaceID := uuid.New()
	item := models.Resource{ID: interfaceID.String(), ServerID: server.ID.String(), APIVersion: "test/v1", Kind: "Interface", Name: "test0", Generation: 1, Spec: json.RawMessage(`{"name":"test0","listen":"0.0.0.0","port":443,"custom":"preserved"}`)}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	iface, err := svc.store.Servers().Interfaces(server.ID).Get(interfaceID)
	if err != nil {
		t.Fatal(err)
	}
	iface.Interface = "renamed"
	handled, err := svc.pushProviderInterface(server.ID, iface)
	if err != nil || !handled {
		t.Fatalf("pushProviderInterface: handled=%v, %v", handled, err)
	}
	agent.mu.Lock()
	putProviders := append([]string(nil), agent.putProviders...)
	updated := agent.resources["Interface/"+item.Name]
	agent.mu.Unlock()
	if len(putProviders) == 0 || putProviders[len(putProviders)-1] != "test" {
		t.Fatalf("generic update was sent to the wrong provider: %v", putProviders)
	}
	var spec map[string]any
	if err := json.Unmarshal(updated.Spec, &spec); err != nil || spec["custom"] != "preserved" || spec["name"] != "renamed" {
		t.Fatalf("generic spec was replaced by AWG projection: %+v, %v", spec, err)
	}
	if err := svc.DeleteInterface(server.ID.String(), interfaceID.String()); err != nil {
		t.Fatal(err)
	}
	agent.mu.Lock()
	deleteProviders := append([]string(nil), agent.deleteProviders...)
	agent.mu.Unlock()
	if len(deleteProviders) == 0 || deleteProviders[len(deleteProviders)-1] != "test" {
		t.Fatalf("generic delete was sent to the wrong provider: %v", deleteProviders)
	}
}

func TestDeleteProviderInterfaceRejectsDependencies(t *testing.T) {
	svc, server, _ := newTestServerWithAgent(t)
	interfaceID := uuid.New()
	item := models.Resource{ID: interfaceID.String(), ServerID: server.ID.String(), APIVersion: "test/v1", Kind: "Interface", Name: "test0", Generation: 1, Spec: json.RawMessage(`{"name":"test0"}`)}
	peer := models.Resource{ID: uuid.New().String(), ServerID: server.ID.String(), APIVersion: "test/v1", Kind: "Peer", Name: "peer", Generation: 1, Spec: json.RawMessage(fmt.Sprintf(`{"interfaceRef":%q}`, interfaceID.String()))}
	if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
		t.Fatal(err)
	}
	if err := svc.saveProviderResource(server.ID.String(), peer); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProviderResource(server.ID.String(), "test", "Interface", item.Name); err == nil || !strings.Contains(err.Error(), "remove peers") {
		t.Fatalf("interface with peer was deleted: %v", err)
	}
	if err := svc.store.Resources(server.ID).Delete(peer.ID); err != nil {
		t.Fatal(err)
	}
	link := &models.Link{ID: uuid.New().String(), Source: models.LinkEndpoint{ResourceID: interfaceID.String()}, Target: models.LinkEndpoint{ResourceID: uuid.New().String()}}
	if err := svc.store.Links().Set(link); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProviderResource(server.ID.String(), "test", "Interface", item.Name); err == nil || !strings.Contains(err.Error(), "remove provider links") {
		t.Fatalf("interface with link was deleted: %v", err)
	}
}

func TestServerStatusDoesNotRequireLegacyAWGAPI(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	agent.mu.Lock()
	agent.legacyDisabled = true
	agent.mu.Unlock()
	status, err := svc.ServerAgentStatus(server.ID.String())
	if err != nil || status != models.AgentStatusOK {
		t.Fatalf("provider-only agent status = %q, %v", status, err)
	}
}

func TestMissingAWGProviderDoesNotFallBackToLegacyInterfaceAPI(t *testing.T) {
	svc, server, agent := newTestServerWithAgent(t)
	server.Unmanaged = true
	if err := svc.store.Servers().Set(server); err != nil {
		t.Fatal(err)
	}
	iface := &models.Interface{ID: uuid.New(), Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820}
	if err := svc.store.Servers().Interfaces(server.ID).Set(iface); err != nil {
		t.Fatal(err)
	}
	svc.pushInterface(server.ID, iface)
	stored, err := svc.store.Servers().Interfaces(server.ID).Get(iface.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.LastSyncError, `provider not found in agent manifest: awg`) {
		t.Fatalf("unexpected sync error: %q", stored.LastSyncError)
	}
	if strings.Contains(stored.LastSyncError, `interface ""`) {
		t.Fatalf("sync error hides the missing provider: %q", stored.LastSyncError)
	}
	agent.mu.Lock()
	legacyCalls := len(agent.store)
	agent.mu.Unlock()
	if legacyCalls != 0 {
		t.Fatal("missing provider fell back to the legacy interface API")
	}
}
