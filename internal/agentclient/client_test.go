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
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
)

// fakeAgent mimics agent/internal/api/handlers.go's actual routes/status
// codes closely enough to exercise the client against realistic responses,
// without pulling in the agent module's wgctrl dependency.
type fakeAgent struct {
	mu    sync.Mutex
	store map[string]agentmodels.InterfaceConfig
}

func newFakeAgent() *httptest.Server {
	fa := &fakeAgent{store: make(map[string]agentmodels.InterfaceConfig)}

	mux := http.NewServeMux()
	mux.HandleFunc("PUT /interfaces", func(w http.ResponseWriter, r *http.Request) {
		var cfg agentmodels.InterfaceConfig
		if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if cfg.Interface == "" {
			http.Error(w, "interface name is required", http.StatusBadRequest)
			return
		}
		fa.mu.Lock()
		fa.store[cfg.Interface] = cfg
		fa.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
	})
	mux.HandleFunc("GET /interfaces/", func(w http.ResponseWriter, r *http.Request) {
		fa.mu.Lock()
		defer fa.mu.Unlock()
		out := make([]agentmodels.InterfaceConfig, 0, len(fa.store))
		for _, cfg := range fa.store {
			out = append(out, cfg)
		}
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
	mux.HandleFunc("DELETE /interfaces/{name}", func(w http.ResponseWriter, r *http.Request) {
		fa.mu.Lock()
		_, ok := fa.store[r.PathValue("name")]
		if ok {
			delete(fa.store, r.PathValue("name"))
		}
		fa.mu.Unlock()
		if !ok {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("PATCH /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /info", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(agentmodels.HostInfo{
			Backend:        "userspace",
			Version:        "test",
			Docker:         true,
			InDocker:       true,
			KernelModule:   false,
			InterfaceKinds: []string{"amneziawg", "wireguard"},
		})
	})

	return httptest.NewServer(mux)
}

func TestClientSetGetListDelete(t *testing.T) {
	srv := newFakeAgent()
	defer srv.Close()

	c := New(srv.Client(), srv.URL)
	ctx := context.Background()

	cfg := agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820}
	if err := c.Set(ctx, cfg); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := c.Get(ctx, "wg0")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Interface != "wg0" || got.Address != "10.0.0.1/24" {
		t.Fatalf("Get returned unexpected config: %+v", got)
	}

	list, err := c.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("List: expected 1 interface, got %d", len(list))
	}

	if err := c.Delete(ctx, "wg0"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := c.Get(ctx, "wg0"); err == nil {
		t.Fatal("Get after delete: expected error, got nil")
	} else {
		var notFound *NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("Get after delete: expected NotFoundError, got %T: %v", err, err)
		}
	}

	if err := c.Delete(ctx, "wg0"); err == nil {
		t.Fatal("Delete already-deleted interface: expected error, got nil")
	} else {
		var notFound *NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("Delete already-deleted interface: expected NotFoundError, got %T: %v", err, err)
		}
	}
}

// recordingTransport wraps a real http.RoundTripper just to snapshot
// whether each outgoing request carried the "Idempotency-Key" header, so
// tests can assert on it without needing to actually force net/http's
// internal stale-pooled-connection retry race (which, on loopback, Go's
// idle-connection reaper tends to win before a second request even gets a
// chance to pick the dead connection — not a reliable thing to assert on
// directly; see markIdempotent's doc comment for what this header is for
// and why it's needed).
type recordingTransport struct {
	http.RoundTripper
	mu     sync.Mutex
	hadKey []bool
}

func (rt *recordingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	_, ok := req.Header["Idempotency-Key"]
	rt.mu.Lock()
	rt.hadKey = append(rt.hadKey, ok)
	rt.mu.Unlock()
	return rt.RoundTripper.RoundTrip(req)
}

// TestClientMutationsAreMarkedIdempotent is a regression test for a
// reported bug: "PUT /interfaces: ... EOF" appearing intermittently.
// Root cause: net/http.Transport pools and reuses idle connections
// (including ones carried over an SSH tunnel, see
// internal/sshclient.TunnelClient); if the agent restarts or the tunnel
// drops while a connection sits idle in the pool, the next request to
// reuse it fails as soon as the transport notices the connection is dead.
// Go auto-retries that on a fresh connection for GET/HEAD/OPTIONS/TRACE,
// but for any other method — including PUT/DELETE/PATCH — it requires an
// explicit "Idempotency-Key" header opt-in (see (*http.Request).isReplayable
// and the Transport doc comment in net/http); without it, the raw error
// surfaces to the caller instead of being silently retried. Set, Delete,
// and SetMetricsEnabled are all genuinely idempotent, so they must all set
// this header via markIdempotent.
func TestClientMutationsAreMarkedIdempotent(t *testing.T) {
	srv := newFakeAgent()
	defer srv.Close()

	rt := &recordingTransport{RoundTripper: http.DefaultTransport}
	httpClient := srv.Client()
	httpClient.Transport = rt
	c := New(httpClient, srv.URL)
	ctx := context.Background()

	cfg := agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820}
	if err := c.Set(ctx, cfg); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := c.SetMetricsEnabled(ctx, true); err != nil {
		t.Fatalf("SetMetricsEnabled: %v", err)
	}
	if err := c.Delete(ctx, "wg0"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// A read-only call, for contrast — it must NOT need the header (Go
	// already retries GET on a stale connection on its own).
	if _, err := c.List(ctx); err != nil {
		t.Fatalf("List: %v", err)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.hadKey) != 4 {
		t.Fatalf("expected 4 recorded requests, got %d", len(rt.hadKey))
	}
	wantIdempotent := []bool{true, true, true, false} // Set, SetMetricsEnabled, Delete, List
	for i, want := range wantIdempotent {
		if rt.hadKey[i] != want {
			t.Errorf("request %d: Idempotency-Key present = %v, want %v", i, rt.hadKey[i], want)
		}
	}
}

func TestClientInfo(t *testing.T) {
	srv := newFakeAgent()
	defer srv.Close()

	c := New(srv.Client(), srv.URL)
	info, err := c.Info(context.Background())
	if err != nil {
		t.Fatalf("Info: %v", err)
	}
	if info.Backend != "userspace" || !info.InDocker || info.KernelModule {
		t.Fatalf("Info returned unexpected host info: %+v", info)
	}
	if len(info.InterfaceKinds) != 2 {
		t.Fatalf("Info: expected 2 interface kinds, got %v", info.InterfaceKinds)
	}
}

func TestClientSetRejectsMissingInterfaceName(t *testing.T) {
	srv := newFakeAgent()
	defer srv.Close()

	c := New(srv.Client(), srv.URL)
	if err := c.Set(context.Background(), agentmodels.InterfaceConfig{}); err == nil {
		t.Fatal("expected error for missing interface name, got nil")
	}
}
