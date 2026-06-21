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
	"testing"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
)

func newTestServerWithAgent(t *testing.T) (*Service, *models.Server, *fakeAgentTLS) {
	t.Helper()
	ts, tlsMaterial, fa := newFakeAgentTLS(t)
	t.Cleanup(ts.Close)

	svc := newTestService(t)
	srv, err := svc.CreateServer(ServerInput{
		Name:  "srv1",
		SSH:   models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
		Agent: models.Agent{Address: ts.Listener.Addr().String(), TLS: tlsMaterial},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}
	return svc, srv, fa
}

// TestReconcileServerFindsAgentOnlyInterface covers "lost/recreated the
// local admin DB": the agent has a config the DB never created.
func TestReconcileServerFindsAgentOnlyInterface(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)

	fa.mu.Lock()
	fa.store["wg-orphan"] = agentmodels.InterfaceConfig{Interface: "wg-orphan", Address: "10.9.0.1/24", ListenPort: 51900}
	fa.mu.Unlock()

	report, err := svc.ReconcileServer(srv.ID.String())
	if err != nil {
		t.Fatalf("ReconcileServer: %v", err)
	}
	if len(report.DBOnly) != 0 {
		t.Fatalf("DBOnly = %+v, want none", report.DBOnly)
	}
	if len(report.AgentOnly) != 1 || report.AgentOnly[0].Interface != "wg-orphan" {
		t.Fatalf("AgentOnly = %+v, want exactly [wg-orphan]", report.AgentOnly)
	}
}

// TestReconcileServerFindsDBOnlyInterface covers "agent lost its storage":
// the DB has an interface the agent never confirmed receiving.
func TestReconcileServerFindsDBOnlyInterface(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)

	iface, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}
	// CreateInterface already pushed it to the fake agent — wipe the
	// agent's side to simulate it losing its storage afterward.
	fa.clear()

	report, err := svc.ReconcileServer(srv.ID.String())
	if err != nil {
		t.Fatalf("ReconcileServer: %v", err)
	}
	if len(report.AgentOnly) != 0 {
		t.Fatalf("AgentOnly = %+v, want none", report.AgentOnly)
	}
	if len(report.DBOnly) != 1 || report.DBOnly[0].ID != iface.ID {
		t.Fatalf("DBOnly = %+v, want exactly [%s]", report.DBOnly, iface.ID)
	}
}

func TestReconcileServerNoMismatch(t *testing.T) {
	svc, srv, _ := newTestServerWithAgent(t)

	if _, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820,
	}); err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}

	report, err := svc.ReconcileServer(srv.ID.String())
	if err != nil {
		t.Fatalf("ReconcileServer: %v", err)
	}
	if len(report.AgentOnly) != 0 || len(report.DBOnly) != 0 {
		t.Fatalf("report = %+v, want no mismatches", report)
	}
}

func TestImportInterfaceCreatesItFromAgentConfig(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)

	fa.mu.Lock()
	fa.store["wg-orphan"] = agentmodels.InterfaceConfig{Interface: "wg-orphan", Address: "10.9.0.1/24", ListenPort: 51900}
	fa.mu.Unlock()

	iface, err := svc.ImportInterface(srv.ID.String(), "wg-orphan")
	if err != nil {
		t.Fatalf("ImportInterface: %v", err)
	}
	if iface.Interface != "wg-orphan" || iface.Address != "10.9.0.1/24" {
		t.Fatalf("imported interface = %+v, want matching agent config", iface)
	}
	if !iface.InSync {
		t.Fatal("expected imported interface to be marked InSync")
	}

	got, err := svc.GetInterface(srv.ID.String(), iface.ID.String())
	if err != nil {
		t.Fatalf("GetInterface after import: %v", err)
	}
	if got.Interface != "wg-orphan" {
		t.Fatalf("GetInterface after import = %+v", got)
	}

	report, err := svc.ReconcileServer(srv.ID.String())
	if err != nil {
		t.Fatalf("ReconcileServer after import: %v", err)
	}
	if len(report.AgentOnly) != 0 {
		t.Fatalf("AgentOnly after import = %+v, want none (it's in the DB now)", report.AgentOnly)
	}
}

func TestDeleteAgentInterfaceRemovesItFromAgentOnly(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)

	fa.mu.Lock()
	fa.store["wg-orphan"] = agentmodels.InterfaceConfig{Interface: "wg-orphan", Address: "10.9.0.1/24", ListenPort: 51900}
	fa.mu.Unlock()

	if err := svc.DeleteAgentInterface(srv.ID.String(), "wg-orphan"); err != nil {
		t.Fatalf("DeleteAgentInterface: %v", err)
	}

	if _, ok := fa.get("wg-orphan"); ok {
		t.Fatal("expected wg-orphan to be removed from the agent")
	}

	report, err := svc.ReconcileServer(srv.ID.String())
	if err != nil {
		t.Fatalf("ReconcileServer after delete: %v", err)
	}
	if len(report.AgentOnly) != 0 {
		t.Fatalf("AgentOnly after delete = %+v, want none", report.AgentOnly)
	}
}
