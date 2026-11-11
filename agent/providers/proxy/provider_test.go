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

package proxy

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
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
)

type testInterfaceSpec struct {
	Name     string `json:"name" jsonschema:"required"`
	Port     int    `json:"port" jsonschema:"required,minimum=1,maximum=65535"`
	Disabled bool   `json:"disabled,omitempty"`
}

type testAdapter struct{}

func (testAdapter) Name() string                            { return "test" }
func (testAdapter) APIVersion() string                      { return "test.ks-tool.dev/v1" }
func (testAdapter) FirewallProtocol() string                { return "tcp" }
func (testAdapter) InterfaceSchema() any                    { return testInterfaceSpec{} }
func (testAdapter) InterfaceUISchema() json.RawMessage      { return nil }
func (testAdapter) ValidateInterface(json.RawMessage) error { return nil }
func (testAdapter) NewSecret() (string, error) {
	value := make([]byte, 16)
	_, err := rand.Read(value)
	return hex.EncodeToString(value), err
}
func (testAdapter) ValidateSecret(secret string) error {
	if len(secret) == 0 {
		return errors.New("empty secret")
	}
	return nil
}
func (testAdapter) RenderServer(raw json.RawMessage, peers []Peer, _ []Route, _ string) (CoreConfig, error) {
	credentials := make([]map[string]string, 0, len(peers))
	for _, peer := range peers {
		credentials = append(credentials, map[string]string{"name": peer.Spec.Name, "secret": peer.Secret})
	}
	document := map[string]any{"interface": json.RawMessage(raw), "peers": credentials}
	data, err := json.Marshal(document)
	return CoreConfig{Data: data, Arguments: func(string) []string { return nil }}, err
}
func (testAdapter) RenderClient(_ json.RawMessage, peer Peer, endpoint string) (string, error) {
	return "test://" + peer.Secret + "@" + endpoint, nil
}

func TestProviderResourceLifecycleWithRealCoreProcess(t *testing.T) {
	root := t.TempDir()
	core := filepath.Join(root, "core")
	if err := os.WriteFile(core, []byte("#!/bin/sh\nwhile :; do sleep 1; done\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	store, err := resource.NewFileStore(filepath.Join(root, "resources"))
	if err != nil {
		t.Fatal(err)
	}
	provider, err := New(testAdapter{}, "test", store, root, core)
	if err != nil {
		t.Fatal(err)
	}
	provider.processes.startDelay = 10 * time.Millisecond
	provider.firewall.system = &fakeFirewallSystem{}
	runtime := resource.NewRuntime(store, "agent", time.Millisecond, time.Hour)
	if err := runtime.Register(provider); err != nil {
		t.Fatal(err)
	}
	runtime.Start(context.Background())
	defer runtime.Close()
	defer func() { _ = provider.Close() }()
	server := httptest.NewServer(api.NewResourceHandler(runtime))
	defer server.Close()
	response, err := http.Get(server.URL + "/v1/manifest")
	if err != nil {
		t.Fatal(err)
	}
	var manifest models.Manifest
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if manifest.ProtocolVersion != models.ProtocolVersion || len(manifest.Providers) != 1 || len(manifest.Providers[0].Resources) != 2 {
		t.Fatalf("unexpected provider manifest: %+v", manifest)
	}

	interfaceResource, err := runtime.Put("test", "Interface", "public", models.Resource{APIVersion: "test.ks-tool.dev/v1", Spec: json.RawMessage(`{"name":"public","port":10443}`)})
	if err != nil {
		t.Fatal(err)
	}
	waitTestResource(t, runtime, "Interface", "public")
	peerResource, err := runtime.Put("test", "Peer", "alice", models.Resource{APIVersion: "test.ks-tool.dev/v1", Spec: json.RawMessage(`{"name":"alice","interfaceRef":"` + interfaceResource.ID + `","publicKey":"public-id"}`)})
	if err != nil {
		t.Fatal(err)
	}
	waitTestResource(t, runtime, "Peer", "alice")
	result, err := runtime.Action(context.Background(), "test", "Peer", "alice", "renderClientConfig", json.RawMessage(`{"endpoint":"192.0.2.1:10443"}`))
	if err != nil {
		t.Fatal(err)
	}
	var uri string
	if err := json.Unmarshal(result.Data, &uri); err != nil || len(uri) == 0 {
		t.Fatalf("invalid client URI: %q, %v", uri, err)
	}
	secret, err := provider.secrets.get(peerResource.ID)
	if err != nil || !strings.Contains(uri, secret) {
		t.Fatalf("client URI does not use provider secret: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(provider.processes.resourceDir(interfaceResource.ID), "config.json"))
	if err != nil || !strings.Contains(string(config), secret) {
		t.Fatalf("core config does not contain reconciled peer: %v", err)
	}
	provider.processes.mu.Lock()
	pid := provider.processes.processes[interfaceResource.ID].cmd.Process.Pid
	provider.processes.mu.Unlock()
	runtime.Resync()
	time.Sleep(100 * time.Millisecond)
	provider.processes.mu.Lock()
	resyncedPID := provider.processes.processes[interfaceResource.ID].cmd.Process.Pid
	provider.processes.mu.Unlock()
	if pid != resyncedPID {
		t.Fatalf("unchanged core was restarted: pid %d became %d", pid, resyncedPID)
	}
	if err := runtime.Delete("test", "Peer", "alice"); err != nil {
		t.Fatal(err)
	}
	waitTestDeletion(t, runtime, "Peer", "alice")
	if err := runtime.Delete("test", "Interface", "public"); err != nil {
		t.Fatal(err)
	}
	waitTestDeletion(t, runtime, "Interface", "public")
}

func waitTestResource(t *testing.T, runtime *resource.Runtime, kind, name string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		item, err := runtime.Get("test", kind, name)
		if err == nil && item.Status.Phase == models.ResourceReady {
			return
		}
		if err == nil && item.Status.Phase == models.ResourceDegraded {
			t.Fatalf("resource degraded: %+v", item.Status.Conditions)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("resource %s/%s did not become ready", kind, name)
}

func waitTestDeletion(t *testing.T, runtime *resource.Runtime, kind, name string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := runtime.Get("test", kind, name); err != nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("resource %s/%s was not deleted", kind, name)
}
