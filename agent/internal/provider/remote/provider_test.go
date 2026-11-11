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

package remote

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ks-tool/vpn-agent/internal/api"
	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/providers/example"
)

func TestExternalProviderLifecycleCrashAndRestart(t *testing.T) {
	externalStore, err := resource.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exampleProvider, err := example.New()
	if err != nil {
		t.Fatal(err)
	}
	externalRuntime := resource.NewRuntime(externalStore, "external", time.Millisecond, time.Hour)
	if err := externalRuntime.Register(exampleProvider); err != nil {
		t.Fatal(err)
	}
	externalRuntime.Start(context.Background())
	defer externalRuntime.Close()
	server := httptest.NewServer(api.NewResourceHandler(externalRuntime))

	remote, err := NewProvider(server.URL, models.ProviderDefinition{Name: "example", Version: "0.1.0", Resources: exampleProvider.Definitions()}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	coreStore, err := resource.NewFileStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core := resource.NewRuntime(coreStore, "core", time.Millisecond, time.Hour)
	if err := core.Register(remote); err != nil {
		t.Fatal(err)
	}
	core.Start(context.Background())
	defer core.Close()
	if _, err := core.Put("example", "Endpoint", "public", models.Resource{APIVersion: "example.ks-tool.dev/v1", Spec: json.RawMessage(`{"title":"Public","address":"192.0.2.1"}`)}); err != nil {
		t.Fatal(err)
	}
	waitRemoteStatus(t, core, "public", models.ResourceReady)
	result, err := core.Action(context.Background(), "example", "Endpoint", "public", "inspect", json.RawMessage(`{}`))
	if err != nil || string(result.Data) != `{"title":"Public","address":"192.0.2.1"}` {
		t.Fatalf("external action: %+v, %v", result, err)
	}
	link := models.Link{ID: "link", Kind: "example", Source: models.LinkEndpoint{ResourceID: "a"}, Target: models.LinkEndpoint{ResourceID: "b"}}
	offer, err := core.PrepareLink(context.Background(), "example", link, link.Source)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.CommitLink(context.Background(), "example", link, link.Source, offer); err != nil {
		t.Fatal(err)
	}

	server.Close()
	if _, err := core.Put("example", "Endpoint", "offline", models.Resource{APIVersion: "example.ks-tool.dev/v1", Spec: json.RawMessage(`{"title":"Offline","address":"192.0.2.2"}`)}); err != nil {
		t.Fatal(err)
	}
	waitRemoteStatus(t, core, "offline", models.ResourceDegraded)
	manifest := core.Manifest()
	if len(manifest.Providers) != 1 || manifest.Providers[0].Name != "example" {
		t.Fatalf("core lost provider discovery after crash: %+v", manifest)
	}

	server = httptest.NewServer(api.NewResourceHandler(externalRuntime))
	defer server.Close()
	core.Close()
	remote, err = NewProvider(server.URL, models.ProviderDefinition{Name: "example", Version: "0.1.0", Resources: exampleProvider.Definitions()}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	core = resource.NewRuntime(coreStore, "core", time.Millisecond, time.Hour)
	if err := core.Register(remote); err != nil {
		t.Fatal(err)
	}
	core.Start(context.Background())
	defer core.Close()
	waitRemoteStatus(t, core, "offline", models.ResourceReady)
}

func TestRemoteDeleteReportsPhysicalDeletionFailure(t *testing.T) {
	definition := models.ProviderDefinition{Name: "example", Version: "1"}
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/manifest":
			_ = json.NewEncoder(w).Encode(models.Manifest{ProtocolVersion: models.ProtocolVersion, Providers: []models.ProviderDefinition{definition}})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			statusCalls++
			_ = json.NewEncoder(w).Encode(models.ResourceStatus{Phase: models.ResourceDegraded, Conditions: []models.Condition{{Message: "physical deletion failed"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider, err := NewProvider(server.URL, definition, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	item := models.Resource{Kind: "Endpoint", Name: "test"}
	if err := provider.Delete(context.Background(), item); err == nil || !strings.Contains(err.Error(), "physical deletion failed") {
		t.Fatalf("accepted DELETE hid physical deletion failure: %v", err)
	}
	if statusCalls == 0 {
		t.Fatal("remote DELETE was treated as synchronous")
	}
}

func TestRemoteDeleteWaitsUntilResourceDisappears(t *testing.T) {
	definition := models.ProviderDefinition{Name: "example", Version: "1"}
	statusCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/manifest":
			_ = json.NewEncoder(w).Encode(models.Manifest{ProtocolVersion: models.ProtocolVersion, Providers: []models.ProviderDefinition{definition}})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/status"):
			statusCalls++
			if statusCalls == 1 {
				_ = json.NewEncoder(w).Encode(models.ResourceStatus{Phase: models.ResourceDeleting})
				return
			}
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	provider, err := NewProvider(server.URL, definition, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	if err := provider.Delete(context.Background(), models.Resource{Kind: "Endpoint", Name: "test"}); err != nil {
		t.Fatal(err)
	}
	if statusCalls != 2 {
		t.Fatalf("status calls = %d, want 2", statusCalls)
	}
}

func TestLoadProviderRegistry(t *testing.T) {
	definition := models.ProviderDefinition{Name: "example", Version: "1", Resources: []models.ResourceDefinition{{APIVersion: "example/v1", Kind: "Endpoint", SpecSchema: json.RawMessage(`{"type":"object"}`), SchemaHash: "hash"}}}
	config := Config{ProtocolVersion: models.ProtocolVersion, Providers: []models.ProviderRegistration{{Address: "http://127.0.0.1:9090", Provider: definition}}}
	data, _ := json.Marshal(config)
	path := filepath.Join(t.TempDir(), "providers.conf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	providers, err := Load(path, nil)
	if err != nil || len(providers) != 1 {
		t.Fatalf("Load: %+v, %v", providers, err)
	}
	config.ProtocolVersion = "unsupported"
	data, _ = json.Marshal(config)
	_ = os.WriteFile(path, data, 0600)
	if _, err := Load(path, nil); err == nil {
		t.Fatal("unsupported protocol version was accepted")
	}
}

func waitRemoteStatus(t *testing.T, runtime *resource.Runtime, name string, phase models.ResourcePhase) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		item, err := runtime.Get("example", "Endpoint", name)
		if err == nil && item.Status.Phase == phase {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	item, _ := runtime.Get("example", "Endpoint", name)
	t.Fatalf("resource %s did not reach %s: %+v", name, phase, item.Status)
}

func TestRemoteProtocolVersionIsCheckedBeforeMutation(t *testing.T) {
	mutations := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/manifest" {
			_ = json.NewEncoder(w).Encode(models.Manifest{ProtocolVersion: "v2", AgentID: "external"})
			return
		}
		mutations++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	provider, err := NewProvider(server.URL, models.ProviderDefinition{Name: "example", Version: "1"}, server.Client())
	if err != nil {
		t.Fatal(err)
	}
	item := models.Resource{APIVersion: "example/v1", Kind: "Endpoint", Name: "test", Generation: 1, Spec: json.RawMessage(`{}`)}
	if _, err := provider.Reconcile(context.Background(), item); err == nil {
		t.Fatal("incompatible protocol was accepted")
	}
	if mutations != 0 {
		t.Fatalf("resource was mutated before protocol validation: %d", mutations)
	}
}

func TestLoadProviderRegistryDirectory(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "providers.conf")
	dir := filepath.Join(root, "providers.d")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, registration := range []struct {
		path string
		name string
	}{{legacy, "legacy"}, {filepath.Join(dir, "one.conf"), "one"}, {filepath.Join(dir, "two.conf"), "two"}} {
		definition := models.ProviderDefinition{Name: registration.name, Version: "1"}
		config := Config{ProtocolVersion: models.ProtocolVersion, Providers: []models.ProviderRegistration{{Address: "http://127.0.0.1:9090", Provider: definition}}}
		data, _ := json.Marshal(config)
		if err := os.WriteFile(registration.path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	providers, err := LoadDir(dir, legacy, nil)
	if err != nil || len(providers) != 3 {
		t.Fatalf("LoadDir returned %d providers, %v", len(providers), err)
	}
}

func TestLoadProviderRegistryDiscoversManifest(t *testing.T) {
	definition := models.ProviderDefinition{
		Name: "awg", Version: "1.2.3",
		Resources: []models.ResourceDefinition{{APIVersion: "awg.ks-tool.dev/v1", Kind: "Interface", SpecSchema: json.RawMessage(`{"type":"object"}`), SchemaHash: "hash"}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/manifest" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(models.Manifest{ProtocolVersion: models.ProtocolVersion, Providers: []models.ProviderDefinition{definition}})
	}))
	defer server.Close()

	config := Config{ProtocolVersion: models.ProtocolVersion, Providers: []models.ProviderRegistration{{Address: server.URL, Discover: true, Provider: models.ProviderDefinition{Name: "awg"}}}}
	data, _ := json.Marshal(config)
	path := filepath.Join(t.TempDir(), "awg.conf")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	providers, err := Load(path, server.Client())
	if err != nil || len(providers) != 1 {
		t.Fatalf("Load: %+v, %v", providers, err)
	}
	if providers[0].Version() != definition.Version || len(providers[0].Definitions()) != 1 {
		t.Fatalf("discovered definition = %s %+v", providers[0].Version(), providers[0].Definitions())
	}
}
