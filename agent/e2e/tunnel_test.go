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

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"

	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestUserspaceTunnel brings up two awg-agent-userspace containers, configures
// each as the other's AmneziaWG peer through the agent HTTP API (with a shared,
// generated set of obfuscation params), and asserts traffic flows across the
// resulting tunnel — an end-to-end check that the userspace agent (amneziawg-go
// compiled in, no external process) builds a working, obfuscated WireGuard
// tunnel: TUN creation, wgctrl/UAPI configuration, obfuscation params and the
// data plane.
//
// Needs Docker with /dev/net/tun and NET_ADMIN available to containers. Gated
// behind AWG_E2E=1 so a plain `go test` never requires Docker; run with:
//
//	cd agent/e2e && GOWORK=off AWG_E2E=1 go test -run TestUserspaceTunnel -v .
func TestUserspaceTunnel(t *testing.T) {
	if os.Getenv("AWG_E2E") == "" {
		t.Skip("set AWG_E2E=1 to run (needs Docker with /dev/net/tun + NET_ADMIN)")
	}
	ctx := context.Background()

	nw, err := network.New(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	testcontainers.CleanupNetwork(t, nw)

	a, apiA, ipA := startAgent(ctx, t, nw.Name, "peer-a")
	b, apiB, ipB := startAgent(ctx, t, nw.Name, "peer-b")

	// A keypair per end.
	aPriv, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key a: %v", err)
	}
	bPriv, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key b: %v", err)
	}

	// One shared set of AmneziaWG obfuscation params — both peers must use the
	// same values to talk to each other. This is the app's real generator
	// (AmneziaWG 2.0: s1–s4 and h1–h4 as ranges, i1); the userspace agent is
	// built against amneziawg-go v0.2.19 (the 2.0 line), which accepts it.
	var amz agentmodels.InterfaceConfig
	agentmodels.GenerateAmneziaParams(&amz)

	const port = 51820
	cfgA := ifaceConfig("awg0", "10.10.0.1/24", port, aPriv, bPriv.PublicKey(), "10.10.0.2/32", fmt.Sprintf("%s:%d", ipB, port), amz)
	cfgB := ifaceConfig("awg0", "10.10.0.2/24", port, bPriv, aPriv.PublicKey(), "10.10.0.1/32", fmt.Sprintf("%s:%d", ipA, port), amz)

	putInterface(ctx, t, apiA, cfgA)
	putInterface(ctx, t, apiB, cfgB)

	// The real assertion: the data plane works — ping B's tunnel address from A.
	if out, ok := pingWithin(ctx, t, a, "10.10.0.2", 45*time.Second); !ok {
		dumpAgentLogs(ctx, t, a, "peer-a")
		dumpAgentLogs(ctx, t, b, "peer-b")
		t.Fatalf("no connectivity across the userspace tunnel within timeout; last ping output:\n%s", out)
	}
}

// startAgent builds (once, cached) and starts an awg-agent-userspace container
// on netName with the given alias, returning it, the base URL of its
// /interfaces API (host-mapped) and its IP on the network.
func startAgent(ctx context.Context, t *testing.T, netName, alias string) (testcontainers.Container, string, string) {
	t.Helper()
	req := testcontainers.ContainerRequest{
		FromDockerfile: testcontainers.FromDockerfile{
			Context:    "..", // the agent module root
			Dockerfile: "e2e/Dockerfile",
			Repo:       "awg-agent-userspace-e2e",
			Tag:        "latest",
			KeepImage:  true,
		},
		ExposedPorts:   []string{"8080/tcp"},
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {alias}},
		WaitingFor:     wait.ForHTTP("/interfaces/").WithPort("8080/tcp").WithStartupTimeout(120 * time.Second),
		// The exact, narrow privileges the production deploy grants
		// (internal/deploy/docker.go) — no --privileged, mirroring how
		// e.g. linuxserver/docker-wireguard runs a userspace WireGuard:
		// CAP_NET_ADMIN + /dev/net/tun for the TUN, NET_RAW for the iptables
		// lifecycle hooks. (No ip_forward here — that's only for gateway/NAT
		// routing between interfaces, not an endpoint-to-endpoint ping.)
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.CapAdd = append(hc.CapAdd, "NET_ADMIN", "NET_RAW")
			hc.Devices = append(hc.Devices, container.DeviceMapping{
				PathOnHost:        "/dev/net/tun",
				PathInContainer:   "/dev/net/tun",
				CgroupPermissions: "rwm",
			})
		},
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatalf("start %s: %v", alias, err)
	}

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("host %s: %v", alias, err)
	}
	mapped, err := c.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatalf("mapped port %s: %v", alias, err)
	}
	ip, err := c.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("container ip %s: %v", alias, err)
	}
	return c, fmt.Sprintf("http://%s:%s/interfaces", host, mapped.Port()), ip
}

// ifaceConfig builds an InterfaceConfig for one end of the tunnel, with a single
// peer (the other end) and the shared obfuscation params from amz.
func ifaceConfig(name, addr string, listen uint16, priv, peerPub agentmodels.Key, peerAllowed, peerEndpoint string, amz agentmodels.InterfaceConfig) agentmodels.InterfaceConfig {
	cfg := agentmodels.InterfaceConfig{
		Interface:  name,
		PrivateKey: priv,
		ListenPort: listen,
		Address:    addr,
		Peers: []agentmodels.InterfacePeer{{
			Key:               peerPub,
			AllowedIPs:        []string{peerAllowed},
			Endpoint:          peerEndpoint,
			KeepaliveInterval: 15 * time.Second,
		}},
	}
	cfg.Jc, cfg.Jmin, cfg.Jmax = amz.Jc, amz.Jmin, amz.Jmax
	cfg.S1, cfg.S2, cfg.S3, cfg.S4 = amz.S1, amz.S2, amz.S3, amz.S4
	cfg.H1, cfg.H2, cfg.H3, cfg.H4 = amz.H1, amz.H2, amz.H3, amz.H4
	cfg.I1, cfg.I2, cfg.I3, cfg.I4, cfg.I5 = amz.I1, amz.I2, amz.I3, amz.I4, amz.I5
	return cfg
}

// putInterface applies cfg to an agent via PUT /interfaces, failing the test if
// the agent doesn't accept it (e.g. an obfuscation param the userspace library
// rejects would surface here).
func putInterface(ctx context.Context, t *testing.T, apiURL string, cfg agentmodels.InterfaceConfig) {
	t.Helper()
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, apiURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT %s: %v", apiURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("PUT %s: status %d: %s", apiURL, resp.StatusCode, bytes.TrimSpace(b))
	}
}

// pingWithin repeatedly pings target from inside c until it succeeds or timeout
// elapses, returning the last ping output and whether it ever succeeded.
func pingWithin(ctx context.Context, t *testing.T, c testcontainers.Container, target string, timeout time.Duration) (string, bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var last string
	for time.Now().Before(deadline) {
		code, r, err := c.Exec(ctx, []string{"ping", "-c", "1", "-W", "2", target})
		if r != nil {
			out, _ := io.ReadAll(r)
			last = string(out)
		}
		if err == nil && code == 0 {
			return last, true
		}
		time.Sleep(2 * time.Second)
	}
	return last, false
}

func dumpAgentLogs(ctx context.Context, t *testing.T, c testcontainers.Container, name string) {
	t.Helper()
	r, err := c.Logs(ctx)
	if err != nil {
		return
	}
	defer func() { _ = r.Close() }()
	out, _ := io.ReadAll(r)
	t.Logf("=== %s logs ===\n%s", name, out)
}
