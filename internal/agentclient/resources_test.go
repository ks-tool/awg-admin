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

package agentclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ks-tool/awg-admin/models"
)

func TestResourceClient(t *testing.T) {
	spec := json.RawMessage(`{"nested":{"enabled":true}}`)
	resource := models.Resource{ID: "id", ServerID: "server", APIVersion: "test/v1", Kind: "Service", Name: "public", Generation: 1, Spec: spec}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /v1/manifest":
			_ = json.NewEncoder(w).Encode(models.Manifest{ProtocolVersion: models.ProtocolVersion, AgentID: "agent"})
		case "GET /v1/providers/test/schema":
			_ = json.NewEncoder(w).Encode(models.ProviderDefinition{Name: "test", Version: "1", Resources: []models.ResourceDefinition{{APIVersion: "test/v1", Kind: "Service", SpecSchema: json.RawMessage(`{"type":"object"}`), SchemaHash: "hash"}}})
		case "GET /v1/providers/test/resources":
			_ = json.NewEncoder(w).Encode([]models.Resource{resource})
		case "PUT /v1/providers/test/resources/Service/public":
			var desired models.Resource
			if err := json.NewDecoder(r.Body).Decode(&desired); err != nil {
				t.Error(err)
			}
			if string(desired.Spec) != string(spec) {
				t.Errorf("spec changed: %s", desired.Spec)
			}
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(desired)
		case "GET /v1/providers/test/resources/Service/public/status":
			_ = json.NewEncoder(w).Encode(models.ResourceStatus{ObservedGeneration: 1, Phase: models.ResourceReady})
		case "POST /v1/providers/test/resources/Service/public/actions/inspect":
			_ = json.NewEncoder(w).Encode(models.ActionResult{Type: models.ActionResultJSON, Data: json.RawMessage(`{"ok":true}`)})
		case "DELETE /v1/providers/test/resources/Service/public":
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := New(server.Client(), server.URL)
	ctx := context.Background()

	manifest, err := client.Manifest(ctx)
	if err != nil || manifest.AgentID != "agent" {
		t.Fatalf("Manifest: %+v, %v", manifest, err)
	}
	definition, err := client.ProviderSchema(ctx, "test")
	if err != nil || len(definition.Resources) != 1 {
		t.Fatalf("ProviderSchema: %+v, %v", definition, err)
	}
	resources, err := client.ListResources(ctx, "test")
	if err != nil || len(resources) != 1 || string(resources[0].Spec) != string(spec) {
		t.Fatalf("ListResources: %+v, %v", resources, err)
	}
	updated, err := client.PutResource(ctx, "test", "Service", "public", resource)
	if err != nil || string(updated.Spec) != string(spec) {
		t.Fatalf("PutResource: %+v, %v", updated, err)
	}
	status, err := client.ResourceStatus(ctx, "test", "Service", "public")
	if err != nil || status.Phase != models.ResourceReady {
		t.Fatalf("ResourceStatus: %+v, %v", status, err)
	}
	action, err := client.RunResourceAction(ctx, "test", "Service", "public", "inspect", json.RawMessage(`{}`))
	if err != nil || string(action.Data) != `{"ok":true}` {
		t.Fatalf("RunResourceAction: %+v, %v", action, err)
	}
	if err := client.DeleteResource(ctx, "test", "Service", "public"); err != nil {
		t.Fatal(err)
	}
}
