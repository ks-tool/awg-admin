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
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/internal/pki"
	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage/boltdb"
)

// fakeAgentTLS is a minimal stand-in for the real agent's HTTP API, served
// over mTLS like a server with AgentTLS configured would be reached
// directly (no SSH tunnel). It tracks every config it was asked to Set, so
// tests can assert pushes actually happened.
type fakeAgentTLS struct {
	mu    sync.Mutex
	store map[string]agentmodels.InterfaceConfig
}

func newFakeAgentTLS(t *testing.T) (*httptest.Server, *models.AgentTLS, *fakeAgentTLS) {
	t.Helper()

	fa := &fakeAgentTLS{store: make(map[string]agentmodels.InterfaceConfig)}
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /interfaces", func(w http.ResponseWriter, r *http.Request) {
		var cfg agentmodels.InterfaceConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		fa.mu.Lock()
		fa.store[cfg.Interface] = cfg
		fa.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("DELETE /interfaces/{name}", func(w http.ResponseWriter, r *http.Request) {
		fa.mu.Lock()
		_, ok := fa.store[r.PathValue("name")]
		delete(fa.store, r.PathValue("name"))
		fa.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("GET /interfaces/", func(w http.ResponseWriter, r *http.Request) {
		fa.mu.Lock()
		out := make([]agentmodels.InterfaceConfig, 0, len(fa.store))
		for _, cfg := range fa.store {
			out = append(out, cfg)
		}
		fa.mu.Unlock()
		_ = json.NewEncoder(w).Encode(out)
	})
	mux.HandleFunc("GET /interfaces/{name}", func(w http.ResponseWriter, r *http.Request) {
		fa.mu.Lock()
		cfg, ok := fa.store[r.PathValue("name")]
		fa.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		_ = json.NewEncoder(w).Encode(cfg)
	})

	ts := httptest.NewUnstartedServer(mux)

	tlsMaterial, err := pki.IssueAgentTLS("test-agent", []net.IP{net.ParseIP("127.0.0.1")}, nil)
	if err != nil {
		t.Fatalf("IssueAgentTLS: %v", err)
	}

	serverCert, err := tls.X509KeyPair([]byte(tlsMaterial.Server.Certificate), []byte(tlsMaterial.Server.PrivateKey))
	if err != nil {
		t.Fatalf("parse server cert: %v", err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(tlsMaterial.CA.Certificate)) {
		t.Fatal("failed to load CA cert into pool")
	}
	ts.TLS = &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    pool,
	}
	ts.StartTLS()

	return ts, tlsMaterial, fa
}

func (fa *fakeAgentTLS) get(name string) (agentmodels.InterfaceConfig, bool) {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	cfg, ok := fa.store[name]
	return cfg, ok
}

func (fa *fakeAgentTLS) clear() {
	fa.mu.Lock()
	defer fa.mu.Unlock()
	fa.store = make(map[string]agentmodels.InterfaceConfig)
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	db, err := boltdb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(db)
}

func mustParseKey(t *testing.T, k agentmodels.Key) string {
	t.Helper()
	return k.String()
}

func TestInterfaceAndPeerMutationsPushToAgent(t *testing.T) {
	ts, tlsMaterial, fa := newFakeAgentTLS(t)
	defer ts.Close()

	svc := newTestService(t)

	srv, err := svc.CreateServer(ServerInput{
		Name: "srv1",
		SSH:  models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
		Agent: models.Agent{
			Address: ts.Listener.Addr().String(),
			TLS:     tlsMaterial,
		},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	// Create: should push immediately and come back InSync.
	iface, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface:  "wg0",
		Address:    "10.0.0.1/24",
		ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}
	if !iface.InSync || iface.LastSyncError != "" {
		t.Fatalf("CreateInterface: expected InSync, got InSync=%v err=%q", iface.InSync, iface.LastSyncError)
	}
	if cfg, ok := fa.get("wg0"); !ok || cfg.Address != "10.0.0.1/24" {
		t.Fatalf("agent did not receive pushed config: %+v, ok=%v", cfg, ok)
	}

	// Update: should re-push with the new address.
	updated, err := svc.UpdateInterfaceConfig(srv.ID.String(), iface.ID.String(), agentmodels.InterfaceConfig{
		Interface:  "wg0",
		Address:    "10.0.0.2/24",
		ListenPort: 51821,
	})
	if err != nil {
		t.Fatalf("UpdateInterfaceConfig: %v", err)
	}
	if !updated.InSync {
		t.Fatalf("UpdateInterfaceConfig: expected InSync, got %+v", updated)
	}
	if cfg, ok := fa.get("wg0"); !ok || cfg.Address != "10.0.0.2/24" {
		t.Fatalf("agent did not receive updated config: %+v, ok=%v", cfg, ok)
	}

	// AddPeer: the agent has no separate peers endpoint, so the peer
	// mutation must trigger a full interface re-push including the peer.
	user, err := svc.CreateUser(UserInput{Name: "u1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err = svc.AddPeer(user.ID.String(), AddPeerInput{
		InterfaceID: iface.ID,
		AllowedIPs:  []string{"10.0.0.5/32"},
	}); err != nil {
		t.Fatalf("AddPeer: %v", err)
	}
	if cfg, ok := fa.get("wg0"); !ok || len(cfg.Peers) != 1 {
		t.Fatalf("agent did not receive peer via interface push: %+v, ok=%v", cfg, ok)
	}

	ifaceAfterAdd, err := svc.GetInterface(srv.ID.String(), iface.ID.String())
	if err != nil {
		t.Fatalf("GetInterface: %v", err)
	}
	if !ifaceAfterAdd.InSync || len(ifaceAfterAdd.Peers) != 1 {
		t.Fatalf("expected interface InSync with 1 peer after AddPeer, got %+v", ifaceAfterAdd)
	}

	// DeletePeer: removing the peer should also re-push.
	pubKey := mustParseKey(t, ifaceAfterAdd.Peers[0].Key)
	if _, err = svc.DeletePeer(user.ID.String(), pubKey); err != nil {
		t.Fatalf("DeletePeer: %v", err)
	}
	if cfg, ok := fa.get("wg0"); !ok || len(cfg.Peers) != 0 {
		t.Fatalf("agent still has peer after DeletePeer: %+v, ok=%v", cfg, ok)
	}

	// DeleteInterface: should tell the agent to tear it down too.
	if err = svc.DeleteInterface(srv.ID.String(), iface.ID.String()); err != nil {
		t.Fatalf("DeleteInterface: %v", err)
	}
	if _, ok := fa.get("wg0"); ok {
		t.Fatal("agent still has interface after DeleteInterface")
	}
}

func TestDeleteInterfaceCascadesPeers(t *testing.T) {
	ts, tlsMaterial, fa := newFakeAgentTLS(t)
	defer ts.Close()

	svc := newTestService(t)

	srv, err := svc.CreateServer(ServerInput{
		Name:  "srv1",
		SSH:   models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
		Agent: models.Agent{Address: ts.Listener.Addr().String(), TLS: tlsMaterial},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	iface, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}

	user, err := svc.CreateUser(UserInput{Name: "u1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	for _, ip := range []string{"10.0.0.5/32", "10.0.0.6/32"} {
		if _, err = svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: iface.ID, AllowedIPs: []string{ip}}); err != nil {
			t.Fatalf("AddPeer(%s): %v", ip, err)
		}
	}

	peers, err := svc.ListPeers(user.ID.String())
	if err != nil {
		t.Fatalf("ListPeers: %v", err)
	}
	if len(peers) != 2 {
		t.Fatalf("expected 2 peers before delete, got %d", len(peers))
	}

	if err = svc.DeleteInterface(srv.ID.String(), iface.ID.String()); err != nil {
		t.Fatalf("DeleteInterface: %v", err)
	}

	// The interface's peers must be removed from the user's records too, not
	// left orphaned — orphaned peer records are what kept them showing in the
	// peer metrics after the interface was gone.
	peers, err = svc.ListPeers(user.ID.String())
	if err != nil {
		t.Fatalf("ListPeers after delete: %v", err)
	}
	if len(peers) != 0 {
		t.Fatalf("expected 0 peers after interface delete, got %d: %+v", len(peers), peers)
	}

	if _, ok := fa.get("wg0"); ok {
		t.Fatal("agent still has interface after DeleteInterface")
	}
}

func TestSyncServerResendsAllInterfaces(t *testing.T) {
	ts, tlsMaterial, fa := newFakeAgentTLS(t)
	defer ts.Close()

	svc := newTestService(t)

	srv, err := svc.CreateServer(ServerInput{
		Name: "srv1",
		SSH:  models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
		Agent: models.Agent{
			Address: ts.Listener.Addr().String(),
			TLS:     tlsMaterial,
		},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	for _, name := range []string{"wg0", "wg1"} {
		if _, err = svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
			Interface: name, Address: "10.0.0.1/24", ListenPort: 51820,
		}); err != nil {
			t.Fatalf("CreateInterface(%s): %v", name, err)
		}
	}

	// Simulate the agent having lost its state (e.g. redeployed) and
	// confirm SyncServer catches it back up.
	fa.clear()
	if err = svc.SyncServer(srv.ID.String()); err != nil {
		t.Fatalf("SyncServer: %v", err)
	}

	for _, name := range []string{"wg0", "wg1"} {
		if _, ok := fa.get(name); !ok {
			t.Fatalf("SyncServer did not push interface %q", name)
		}
	}
}

func TestPushRecordsFailureWhenAgentUnreachable(t *testing.T) {
	svc := newTestService(t)

	srv, err := svc.CreateServer(ServerInput{
		Name: "srv1",
		// Port 1 on loopback: nothing listens there, so the dial fails
		// immediately (connection refused) without any real network or
		// DNS dependency — no tunnel ends up open for this server.
		SSH: models.SSHConfig{Host: "127.0.0.1", Port: 1, User: "root", Password: "x"},
		Agent: models.Agent{
			Address: "127.0.0.1:1",
		},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	iface, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{
		Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface: %v", err)
	}

	if iface.InSync {
		t.Fatal("expected InSync=false when no tunnel/agent is reachable")
	}
	if iface.LastSyncError == "" {
		t.Fatal("expected LastSyncError to be set when push fails")
	}
}
