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

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
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
