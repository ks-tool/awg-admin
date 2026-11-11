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
	"testing"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
	agentmodels "github.com/ks-tool/vpn-agent/models"
)

func TestDistributedLinkCoordinationAndRecovery(t *testing.T) {
	svc := newTestService(t)
	serverA, agentA := newLinkTestServer(t, svc, "a")
	serverB, agentB := newLinkTestServer(t, svc, "b")
	resourceA := newLinkTestResource(t, svc, serverA)
	resourceB := newLinkTestResource(t, svc, serverB)

	link, err := svc.CreateProviderLink(resourceA.ID, resourceB.ID)
	if err != nil {
		t.Fatalf("CreateProviderLink: %v", err)
	}
	if link.Status.Phase != models.LinkReady || link.Source.Address != serverA.SSH.Host || link.Target.Address != serverB.SSH.Host {
		t.Fatalf("unexpected ready link: %+v", link)
	}
	agentA.mu.Lock()
	sourceCommits := agentA.commitCalls
	agentA.mu.Unlock()
	agentB.mu.Lock()
	targetCommits := agentB.commitCalls
	agentB.mu.Unlock()
	if sourceCommits != 1 || targetCommits != 1 {
		t.Fatalf("commit calls = %d/%d", sourceCommits, targetCommits)
	}
	if _, err := svc.ApplyProviderLink(link.ID); err != nil {
		t.Fatalf("ApplyProviderLink: %v", err)
	}
	if _, err := svc.CreateProviderLink(resourceB.ID, resourceA.ID); err == nil {
		t.Fatal("cycle was accepted")
	}

	resourceA2 := newLinkTestResource(t, svc, serverA)
	resourceB2 := newLinkTestResource(t, svc, serverB)
	agentB.mu.Lock()
	agentB.failCommit = true
	agentB.mu.Unlock()
	failed, err := svc.CreateProviderLink(resourceA2.ID, resourceB2.ID)
	if err == nil {
		t.Fatal("partial commit unexpectedly succeeded")
	}
	if failed.Status.Phase != models.LinkDegraded {
		t.Fatalf("partial link state = %+v", failed.Status)
	}
	recovered, err := svc.ApplyProviderLink(failed.ID)
	if err != nil {
		t.Fatalf("recovery after partial commit: %v", err)
	}
	if recovered.Status.Phase != models.LinkReady {
		t.Fatalf("recovered link state = %+v", recovered.Status)
	}

	links, err := svc.ListProviderLinks()
	if err != nil || len(links) != 2 {
		t.Fatalf("ListProviderLinks: %+v, %v", links, err)
	}
	for _, item := range links {
		if err := svc.DeleteProviderLink(item.ID); err != nil {
			t.Fatalf("DeleteProviderLink(%s): %v", item.ID, err)
		}
	}
	if err := svc.DeleteProviderLink(link.ID); err != nil {
		t.Fatalf("idempotent DeleteProviderLink: %v", err)
	}
	agentA.mu.Lock()
	sourceDeletes := agentA.deleteCalls
	agentA.mu.Unlock()
	agentB.mu.Lock()
	targetDeletes := agentB.deleteCalls
	agentB.mu.Unlock()
	if sourceDeletes != 2 || targetDeletes != 2 {
		t.Fatalf("delete calls = %d/%d", sourceDeletes, targetDeletes)
	}
}

func newLinkTestServer(t *testing.T, svc *Service, name string) (*models.Server, *fakeAgentTLS) {
	t.Helper()
	ts, tlsMaterial, agent := newFakeAgentTLS(t)
	t.Cleanup(ts.Close)
	server, err := svc.CreateServer(ServerInput{
		Name: name, SSH: models.SSHConfig{Host: "192.0.2." + name, User: "root", Password: "x"},
		Agent: models.Agent{Address: ts.Listener.Addr().String(), TLS: tlsMaterial},
	})
	if err != nil {
		t.Fatal(err)
	}
	return server, agent
}

func newLinkTestResource(t *testing.T, svc *Service, server *models.Server) models.Resource {
	t.Helper()
	resource := models.Resource{
		ID: uuid.New().String(), ServerID: server.ID.String(), APIVersion: "test/v1", Kind: "Service", Name: "resource-" + uuid.New().String()[:8], Generation: 1,
		Spec: json.RawMessage(`{}`), Status: models.ResourceStatus{ObservedGeneration: 1, Phase: models.ResourceReady},
	}
	if err := svc.saveProviderResource(server.ID.String(), resource); err != nil {
		t.Fatal(err)
	}
	return resource
}

func TestLinkPortsMustAcceptEachOther(t *testing.T) {
	source := resolvedEndpoint{port: models.PortDefinition{Name: "source", LinkKinds: []string{"vpn"}, RemotePorts: []string{"target"}}}
	target := resolvedEndpoint{port: models.PortDefinition{Name: "target", LinkKinds: []string{"vpn"}, RemotePorts: []string{"not-source"}}}
	if _, err := compatibleLinkKind(source, target); err == nil {
		t.Fatal("link was accepted although target rejected the source port")
	}
}

func TestDeleteProviderLinkClearsMigratedLegacyTunnel(t *testing.T) {
	svc := newTestService(t)
	server, _ := newLinkTestServer(t, svc, "a")
	tunnelID := uuid.New()
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	for _, id := range ids {
		iface := &models.Interface{ID: id, Tunnel: &tunnelID, InterfaceConfig: agentmodels.InterfaceConfig{Interface: "awg" + id.String()[:4], Address: "10.0.0.1/24", ListenPort: 51820}}
		if err := svc.store.Servers().Interfaces(server.ID).Set(iface); err != nil {
			t.Fatal(err)
		}
		item := models.Resource{ID: id.String(), ServerID: server.ID.String(), APIVersion: "test/v1", Kind: "Service", Name: id.String(), Spec: json.RawMessage(`{}`)}
		if err := svc.saveProviderResource(server.ID.String(), item); err != nil {
			t.Fatal(err)
		}
	}
	link, err := svc.CreateProviderLink(ids[0].String(), ids[1].String())
	if err != nil {
		t.Fatal(err)
	}
	link.Spec = json.RawMessage(`{"tunnelId":"` + tunnelID.String() + `"}`)
	if err := svc.store.Links().Set(link); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProviderLink(link.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		iface, err := svc.store.Servers().Interfaces(server.ID).Get(id)
		if err != nil {
			t.Fatal(err)
		}
		if iface.Tunnel != nil {
			t.Fatalf("legacy tunnel guard survived link deletion: %+v", iface.Tunnel)
		}
	}
}

func TestMoveProviderLink(t *testing.T) {
	svc := newTestService(t)
	serverA, _ := newLinkTestServer(t, svc, "a")
	serverB, _ := newLinkTestServer(t, svc, "b")
	serverC, agentC := newLinkTestServer(t, svc, "c")
	resourceA := newLinkTestResource(t, svc, serverA)
	resourceB := newLinkTestResource(t, svc, serverB)
	resourceC := newLinkTestResource(t, svc, serverC)

	original, err := svc.CreateProviderLink(resourceA.ID, resourceB.ID)
	if err != nil {
		t.Fatal(err)
	}
	movedTarget, err := svc.MoveProviderLink(original.ID, resourceA.ID, resourceC.ID)
	if err != nil {
		t.Fatalf("move target: %v", err)
	}
	if movedTarget.Source.ResourceID != resourceA.ID || movedTarget.Target.ResourceID != resourceC.ID || movedTarget.Status.Phase != models.LinkReady {
		t.Fatalf("unexpected target move: %+v", movedTarget)
	}
	movedSource, err := svc.MoveProviderLink(movedTarget.ID, resourceB.ID, resourceC.ID)
	if err != nil {
		t.Fatalf("move source: %v", err)
	}
	if movedSource.Source.ResourceID != resourceB.ID || movedSource.Target.ResourceID != resourceC.ID || movedSource.Status.Phase != models.LinkReady {
		t.Fatalf("unexpected source move: %+v", movedSource)
	}

	agentC.mu.Lock()
	agentC.failCommit = true
	agentC.mu.Unlock()
	if _, err := svc.MoveProviderLink(movedSource.ID, resourceA.ID, resourceC.ID); err == nil {
		t.Fatal("failed replacement was accepted")
	}
	agentC.mu.Lock()
	agentC.failCommit = false
	agentC.mu.Unlock()
	links, err := svc.ListProviderLinks()
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].ID != movedSource.ID || links[0].Source.ResourceID != resourceB.ID || links[0].Target.ResourceID != resourceC.ID {
		t.Fatalf("previous link was not preserved after failed move: %+v", links)
	}
}
