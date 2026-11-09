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
	"net/http"
	"net/http/httptest"
	"testing"

	agentmodels "github.com/ks-tool/vpn-admin/agent/models"
	"github.com/ks-tool/vpn-admin/internal/plugin"
	"github.com/ks-tool/vpn-admin/models"
)

func TestPluginApplyAndUserLifecycle(t *testing.T) {
	var stored *agentmodels.InterfaceConfig
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/interfaces/awg0":
			if stored == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_ = json.NewEncoder(w).Encode(stored)
		case r.Method == http.MethodPut && r.URL.Path == "/interfaces":
			var cfg agentmodels.InterfaceConfig
			if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
				t.Error(err)
			}
			stored = &cfg
			w.WriteHeader(http.StatusCreated)
		case r.Method == http.MethodDelete && r.URL.Path == "/interfaces/awg0":
			stored = nil
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer agent.Close()

	deployment := plugin.Deployment{Service: models.VPNService{Config: map[string]any{
		"agentUrl": agent.URL, "interface": "awg0", "privateKey": "", "address": "10.0.0.1/24", "listenPort": 51820, "amnezia": true,
	}}}
	adapter := New()
	status, err := adapter.Apply(context.Background(), deployment)
	if err != nil || status.State != "running" {
		t.Fatalf("apply: %#v, %v", status, err)
	}
	if stored == nil || agentmodels.IsEmpty(stored.PrivateKey) || !stored.IsAmnezia() {
		t.Fatalf("invalid applied config: %#v", stored)
	}

	created, err := adapter.CreateUser(context.Background(), deployment, map[string]any{"allowedIPs": []string{"10.0.0.2/32"}, "keepalive": 25})
	if err != nil {
		t.Fatal(err)
	}
	if created.Data["privateKey"] == "" || created.ID == "" {
		t.Fatalf("generated credentials missing: %#v", created)
	}
	users, err := adapter.ListUsers(context.Background(), deployment)
	if err != nil || len(users) != 1 {
		t.Fatalf("list users: %#v, %v", users, err)
	}
	updated, err := adapter.UpdateUser(context.Background(), deployment, created.ID, map[string]any{"allowedIPs": []string{"10.0.0.3/32"}, "keepalive": 30})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID || updated.Data["allowedIPs"].([]string)[0] != "10.0.0.3/32" {
		t.Fatalf("unexpected updated user: %#v", updated)
	}
	if err := adapter.DeleteUser(context.Background(), deployment, created.ID); err != nil {
		t.Fatal(err)
	}
	if len(stored.Peers) != 0 {
		t.Fatal("peer was not deleted")
	}
	if err := adapter.Delete(context.Background(), deployment); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Fatal("interface was not deleted")
	}
}
