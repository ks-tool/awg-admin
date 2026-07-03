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

// Package agentclient is a typed HTTP client over the agent's actual API
// (agent/internal/api/handlers.go: PUT/GET/DELETE /interfaces). It takes an
// arbitrary *http.Client so the caller decides the transport — an SSH
// tunnel (internal/sshclient.TunnelClient) or a direct mTLS client — the
// client itself doesn't care which.
package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
)

// NotFoundError is returned by Get/Delete when the agent has no interface
// with the given name (agent storage.NotFound, mapped to HTTP 404 by the
// agent's handlers).
type NotFoundError struct {
	Interface string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("interface %q not found on agent", e.Interface)
}

// Client talks to a single agent's HTTP API.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// New returns a Client that issues requests through httpClient against
// baseURL (e.g. "http://127.0.0.1:8080" for a tunnelled agent, or
// "https://<agent-addr>" for direct mTLS).
func New(httpClient *http.Client, baseURL string) *Client {
	return &Client{httpClient: httpClient, baseURL: strings.TrimSuffix(baseURL, "/")}
}

// Set applies cfg to the agent's interface of the same name, creating it if
// it doesn't already exist (PUT /interfaces).
func (c *Client) Set(ctx context.Context, cfg agentmodels.InterfaceConfig) error {
	body, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal interface config: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+"/interfaces", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	markIdempotent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PUT /interfaces: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusCreated {
		return responseError(resp, cfg.Interface)
	}
	return nil
}

// Get fetches the current config of the named interface (GET /interfaces/{name}).
func (c *Client) Get(ctx context.Context, name string) (*agentmodels.InterfaceConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/interfaces/"+name, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /interfaces/%s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp, name)
	}

	var cfg agentmodels.InterfaceConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode interface config: %w", err)
	}
	return &cfg, nil
}

// List fetches every interface configured on the agent (GET /interfaces/).
func (c *Client) List(ctx context.Context) ([]agentmodels.InterfaceConfig, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/interfaces/", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /interfaces/: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp, "")
	}

	var cfgs []agentmodels.InterfaceConfig
	if err := json.NewDecoder(resp.Body).Decode(&cfgs); err != nil {
		return nil, fmt.Errorf("decode interface list: %w", err)
	}
	return cfgs, nil
}

// Delete removes the named interface from the agent (DELETE /interfaces/{name}).
func (c *Client) Delete(ctx context.Context, name string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/interfaces/"+name, nil)
	if err != nil {
		return err
	}
	markIdempotent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DELETE /interfaces/%s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNoContent {
		return responseError(resp, name)
	}
	return nil
}

// Info fetches the host facts the agent discovered at startup — its backend and
// creatable interface kinds, whether Docker is available on the host, whether
// the agent itself runs in a container, kernel-module presence (GET /info).
func (c *Client) Info(ctx context.Context) (*agentmodels.HostInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/info", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /info: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp, "")
	}

	var info agentmodels.HostInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode host info: %w", err)
	}
	return &info, nil
}

// Metrics fetches the agent's latest CPU/RAM/load/network/peer snapshot
// (GET /metrics).
func (c *Client) Metrics(ctx context.Context) (*agentmodels.MetricsSnapshot, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/metrics", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp, "")
	}

	var snap agentmodels.MetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		return nil, fmt.Errorf("decode metrics snapshot: %w", err)
	}
	return &snap, nil
}

// MetricsHistory fetches every host-level sample plus every per-peer sample
// still retained in the agent's in-memory ring buffers (GET /metrics/history)
// — up to 48h, oldest first — for charting instead of just the latest value
// (see Metrics). Per-peer series arrive in SystemHistory.Interfaces; history
// is served only through this one endpoint.
func (c *Client) MetricsHistory(ctx context.Context) (*agentmodels.SystemHistory, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/metrics/history", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /metrics/history: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp, "")
	}

	var hist agentmodels.SystemHistory
	if err := json.NewDecoder(resp.Body).Decode(&hist); err != nil {
		return nil, fmt.Errorf("decode metrics history: %w", err)
	}
	return &hist, nil
}

// SetMetricsEnabled turns the agent's metrics collection on/off at runtime
// (PATCH /metrics). Not persisted by the agent across restarts — callers
// that want the setting to survive a redeploy re-apply it via SyncServer,
// same as interface config.
func (c *Client) SetMetricsEnabled(ctx context.Context, enabled bool) error {
	body, err := json.Marshal(map[string]bool{"enabled": enabled})
	if err != nil {
		return fmt.Errorf("marshal metrics state: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/metrics", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	markIdempotent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH /metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return responseError(resp, "")
	}
	return nil
}

// SetProfilingEnabled turns the agent's Go runtime profiling (the /debug/pprof
// endpoints) on/off at runtime (PATCH /profiling). Like metrics, not persisted
// by the agent across restarts — re-applied via SyncServer.
func (c *Client) SetProfilingEnabled(ctx context.Context, enabled bool) error {
	body, err := json.Marshal(map[string]bool{"enabled": enabled})
	if err != nil {
		return fmt.Errorf("marshal profiling state: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.baseURL+"/profiling", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	markIdempotent(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("PATCH /profiling: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return responseError(resp, "")
	}
	return nil
}

// maxProfileBytes caps how much of a profiling dump the admin will read, so a
// runaway/hostile response can't exhaust its memory. Real pprof dumps are far
// smaller (KBs to a few MB).
const maxProfileBytes = 128 << 20 // 128 MiB

// Profile fetches a Go runtime profiling dump from the agent
// (GET /debug/pprof/<name>). For the CPU "profile" and "trace" kinds, seconds
// sets the sampling window (appended as ?seconds=N); it's ignored for the
// instantaneous kinds (heap/goroutine/allocs/…). The agent answers 403 unless
// profiling is enabled (SetProfilingEnabled). Returns the raw dump bytes.
func (c *Client) Profile(ctx context.Context, name string, seconds int) ([]byte, error) {
	u := c.baseURL + "/debug/pprof/" + name
	if seconds > 0 {
		u += fmt.Sprintf("?seconds=%d", seconds)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /debug/pprof/%s: %w", name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, responseError(resp, "")
	}

	// Read one byte past the cap so hitting it is distinguishable from a genuine
	// EOF: io.ReadAll(io.LimitReader(body, N)) returns exactly N bytes with a nil
	// error when the body is larger, which would silently hand back a truncated —
	// and therefore corrupt, unparseable — dump. Error out instead.
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxProfileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}
	if int64(len(data)) > maxProfileBytes {
		return nil, fmt.Errorf("profile dump exceeds %d bytes; refusing to return a truncated dump", maxProfileBytes)
	}
	return data, nil
}

// markIdempotent tells net/http.Transport it's safe to silently retry req
// on a fresh connection if the pooled (often SSH-tunnelled, see
// internal/sshclient.TunnelClient) connection it picked turned out to
// already be dead — e.g. the agent process restarted, or the SSH session
// was idle long enough to be dropped. Without this, that case surfaces as
// a bare `PUT /interfaces: ... EOF` (or DELETE/PATCH equivalent): Go only
// auto-retries GET/HEAD/OPTIONS/TRACE on a connection that turned out to
// have zero bytes written to it; for any other method it requires an
// explicit signal that resending is safe (see (*Request).isReplayable in
// net/http, and the Transport doc comment on "Idempotency-Key"). PUT
// /interfaces (full replace), DELETE /interfaces/{name}, and PATCH
// /metrics are all genuinely idempotent — repeating the exact same request
// has the same effect as sending it once — so this is safe here. The
// value is intentionally empty: the agent doesn't need to see it, and an
// empty value tells Transport not to even put it on the wire (see the
// same doc comment).
func markIdempotent(req *http.Request) {
	req.Header.Set("Idempotency-Key", "")
}

func responseError(resp *http.Response, name string) error {
	if resp.StatusCode == http.StatusNotFound {
		return &NotFoundError{Interface: name}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("agent returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
}
