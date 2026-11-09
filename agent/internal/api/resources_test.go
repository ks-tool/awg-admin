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

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ks-tool/vpn-admin/agent/internal/resource"
	"github.com/ks-tool/vpn-admin/agent/models"
)

type resourceAPIProvider struct{}

func (resourceAPIProvider) Name() string    { return "test" }
func (resourceAPIProvider) Version() string { return "1.0.0" }
func (resourceAPIProvider) Definitions() []models.ResourceDefinition {
	return []models.ResourceDefinition{{
		APIVersion: "test/v1", Kind: "Service", SpecSchema: json.RawMessage(`{"type":"object"}`), SchemaHash: "hash",
		Actions: []models.ActionDefinition{{Name: "inspect", Title: "Inspect", ResultType: models.ActionResultJSON}},
	}}
}
func (resourceAPIProvider) Validate(context.Context, models.Resource) error { return nil }
func (resourceAPIProvider) Reconcile(context.Context, models.Resource) (json.RawMessage, error) {
	return json.RawMessage(`{"ready":true}`), nil
}
func (resourceAPIProvider) Delete(context.Context, models.Resource) error { return nil }
func (resourceAPIProvider) Action(_ context.Context, _ models.Resource, _ string, arguments json.RawMessage) (models.ActionResult, error) {
	return models.ActionResult{Type: models.ActionResultJSON, Data: arguments}, nil
}

func TestResourceAPI(t *testing.T) {
	store, err := resource.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime := resource.NewRuntime(store, "agent-id", time.Millisecond, time.Hour)
	if err := runtime.Register(resourceAPIProvider{}); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	server := httptest.NewServer(NewResourceHandler(runtime))
	defer server.Close()

	response := request(t, http.MethodGet, server.URL+"/v1/manifest", nil)
	var manifest models.Manifest
	decodeResponse(t, response, &manifest)
	if manifest.ProtocolVersion != models.ProtocolVersion || manifest.AgentID != "agent-id" || len(manifest.Providers) != 1 {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}

	response = request(t, http.MethodGet, server.URL+"/v1/providers/test/schema", nil)
	var definition models.ProviderDefinition
	decodeResponse(t, response, &definition)
	if len(definition.Resources) != 1 || definition.Resources[0].SchemaHash != "hash" {
		t.Fatalf("unexpected schema: %+v", definition)
	}

	body := []byte(`{"apiVersion":"test/v1","kind":"Service","name":"public","spec":{"enabled":true}}`)
	response = request(t, http.MethodPut, server.URL+"/v1/providers/test/resources/Service/public", body)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("PUT status: %s", response.Status)
	}
	var created models.Resource
	decodeResponse(t, response, &created)
	if created.ID == "" || created.Generation != 1 {
		t.Fatalf("unexpected created resource: %+v", created)
	}

	var status models.ResourceStatus
	deadline := time.Now().Add(2 * time.Second)
	for {
		response = request(t, http.MethodGet, server.URL+"/v1/providers/test/resources/Service/public/status", nil)
		decodeResponse(t, response, &status)
		if status.Phase == models.ResourceReady || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if status.Phase != models.ResourceReady || status.ObservedGeneration != 1 || len(status.Conditions) != 1 {
		t.Fatalf("unexpected resource status: %+v", status)
	}

	response = request(t, http.MethodGet, server.URL+"/v1/providers/test/resources", nil)
	var listed []models.Resource
	decodeResponse(t, response, &listed)
	if len(listed) != 1 || string(listed[0].Spec) != `{"enabled":true}` {
		t.Fatalf("unexpected resource list: %+v", listed)
	}

	response = request(t, http.MethodPost, server.URL+"/v1/providers/test/resources/Service/public/actions/inspect", []byte(`{"arguments":{"verbose":true}}`))
	var action models.ActionResult
	decodeResponse(t, response, &action)
	if string(action.Data) != `{"verbose":true}` {
		t.Fatalf("unexpected action response: %+v", action)
	}

	response = request(t, http.MethodDelete, server.URL+"/v1/providers/test/resources/Service/public", nil)
	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("DELETE status: %s", response.Status)
	}
	deadline = time.Now().Add(2 * time.Second)
	for {
		response = request(t, http.MethodGet, server.URL+"/v1/providers/test/resources/Service/public/status", nil)
		response.Body.Close()
		if response.StatusCode == http.StatusNotFound || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("deleted resource status: %s", response.Status)
	}
}

func request(t *testing.T, method, url string, body []byte) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeResponse(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if response.StatusCode >= 400 {
		var message bytes.Buffer
		_, _ = message.ReadFrom(response.Body)
		t.Fatalf("request failed: %s: %s", response.Status, message.String())
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}
