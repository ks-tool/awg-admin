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
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ks-tool/vpn-admin/internal/pki"
	adminservice "github.com/ks-tool/vpn-admin/internal/service"
	adminmodels "github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage/boltdb"
	agentmodels "github.com/ks-tool/vpn-agent/models"

	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

// TestUserspaceTunnel uses the admin service to build A → B → C → D → E through
// four provider links. B, C and D each terminate two links on one interface.
// Every path is checked with bare iptables, UFW and firewalld after a firewall
// reload, covering both live and persistent rules.
//
// Needs Docker with /dev/net/tun and NET_ADMIN available to containers. Gated
// behind AWG_E2E=1 so a plain go test never requires Docker; run with:
//
//	cd agent/e2e && GOWORK=off AWG_E2E=1 go test -count=1 -run TestUserspaceTunnel -v .
func TestUserspaceTunnel(t *testing.T) {
	if os.Getenv("AWG_E2E") == "" {
		t.Skip("set AWG_E2E=1 to run (needs Docker with /dev/net/tun + NET_ADMIN)")
	}
	for _, backend := range []string{"iptables", "ufw", "firewalld"} {
		t.Run(backend, func(t *testing.T) { testUserspaceTunnel(t, backend) })
	}
}

func testUserspaceTunnel(t *testing.T, backend string) {
	ctx := context.Background()

	nw, err := network.New(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	testcontainers.CleanupNetwork(t, nw)

	names := []string{"a", "b", "c", "d", "e"}
	nodes := make([]agentNode, len(names))
	for i, name := range names {
		nodes[i] = startAgent(ctx, t, nw.Name, "peer-"+name, backend)
	}

	store, err := boltdb.Open(filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatalf("open admin storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	admin := adminservice.New(store)
	interfaces := make([]*adminmodels.Interface, len(nodes))
	for i, name := range names {
		server := addAdminServer(t, admin, name, nodes[i])
		address := fmt.Sprintf("10.10.0.%d/24", i+1)
		interfaces[i] = createAdminInterface(t, admin, server, "awg0", address, uint16(51820+i))
	}

	for i := 0; i < len(interfaces)-1; i++ {
		link, err := admin.CreateProviderLink(interfaces[i].ID.String(), interfaces[i+1].ID.String())
		if err != nil {
			dumpTunnelState(ctx, t, nodes)
			t.Fatalf("create provider link %s → %s: %v", names[i], names[i+1], err)
		}
		if link.Status.Phase != adminmodels.LinkReady {
			t.Fatalf("link %s is not ready: %+v", link.ID, link.Status)
		}
	}

	reloadFirewall(ctx, t, backend, nodes)
	for i := 0; i < len(nodes)-1; i++ {
		target := fmt.Sprintf("10.10.0.%d", i+2)
		if out, ok := pingWithin(ctx, t, nodes[i].container, target, 20*time.Second); !ok {
			dumpTunnelState(ctx, t, nodes[i:i+2])
			t.Fatalf("no connectivity through admin-built tunnel %s → %s after %s reload; last ping output:\n%s", names[i], names[i+1], backend, out)
		}
	}
	for hops := 2; hops < len(nodes); hops++ {
		for i := 0; i+hops < len(nodes); i++ {
			target := fmt.Sprintf("10.10.0.%d", i+hops+1)
			if out, ok := pingWithin(ctx, t, nodes[i].container, target, 30*time.Second); !ok {
				dumpTunnelState(ctx, t, nodes)
				t.Fatalf("no connectivity from %s through %d admin-built tunnels to %s after %s reload; last ping output:\n%s", names[i], hops, names[i+hops], backend, out)
			}
		}
	}
}

func reloadFirewall(ctx context.Context, t *testing.T, backend string, nodes []agentNode) {
	t.Helper()
	if backend == "iptables" {
		return
	}
	command := []string{"ufw", "reload"}
	if backend == "firewalld" {
		command = []string{"firewall-cmd", "--reload"}
	}
	for _, node := range nodes {
		code, output, err := node.container.Exec(ctx, command)
		data, _ := io.ReadAll(output)
		if err != nil || code != 0 {
			dumpAgentLogs(ctx, t, node.container, backend)
			t.Fatalf("reload %s: code=%d err=%v output=%s", backend, code, err, data)
		}
	}
}

type agentNode struct {
	container   testcontainers.Container
	address     string
	ip          string
	tlsMaterial *adminmodels.AgentTLS
}

func startAgent(ctx context.Context, t *testing.T, netName, alias, backend string) agentNode {
	t.Helper()
	tlsMaterial, err := pki.IssueAgentTLS(alias, []net.IP{net.ParseIP("127.0.0.1")}, []string{"localhost"})
	if err != nil {
		t.Fatalf("issue TLS for %s: %v", alias, err)
	}
	clientTLS := clientTLSConfig(t, tlsMaterial)
	req := testcontainers.ContainerRequest{
		Context:        "..",
		Dockerfile:     "e2e/Dockerfile",
		Repo:           "vpn-provider-awg-e2e",
		Tag:            "latest",
		KeepImage:      true,
		ExposedPorts:   []string{"8080/tcp"},
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {alias}},
		Env: map[string]string{
			"AWG_E2E_FIREWALL":               backend,
			"VPN_PROVIDER_AWG_TLS_CERT":      "/tls/server.pem",
			"VPN_PROVIDER_AWG_TLS_KEY":       "/tls/server-key.pem",
			"VPN_PROVIDER_AWG_TLS_CLIENT_CA": "/tls/ca.pem",
		},
		Files: []testcontainers.ContainerFile{
			{Reader: strings.NewReader(tlsMaterial.CA.Certificate), ContainerFilePath: "/tls/ca.pem", FileMode: 0o600},
			{Reader: strings.NewReader(tlsMaterial.Server.Certificate), ContainerFilePath: "/tls/server.pem", FileMode: 0o600},
			{Reader: strings.NewReader(tlsMaterial.Server.PrivateKey), ContainerFilePath: "/tls/server-key.pem", FileMode: 0o600},
		},
		WaitingFor: wait.ForHTTP("/interfaces/").WithPort("8080/tcp").WithTLS(true, clientTLS).WithStartupTimeout(120 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.CapAdd = append(hc.CapAdd, "NET_ADMIN", "NET_RAW")
			hc.Sysctls = map[string]string{"net.ipv4.ip_forward": "1"}
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

	mapped, err := c.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatalf("mapped port %s: %v", alias, err)
	}
	ip, err := c.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("container ip %s: %v", alias, err)
	}
	return agentNode{container: c, address: net.JoinHostPort("localhost", mapped.Port()), ip: ip, tlsMaterial: tlsMaterial}
}

func clientTLSConfig(t *testing.T, material *adminmodels.AgentTLS) *tls.Config {
	t.Helper()
	certificate, err := tls.X509KeyPair([]byte(material.Client.Certificate), []byte(material.Client.PrivateKey))
	if err != nil {
		t.Fatalf("load admin client certificate: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(material.CA.Certificate)) {
		t.Fatal("parse agent CA")
	}
	return &tls.Config{Certificates: []tls.Certificate{certificate}, RootCAs: roots, ServerName: "localhost", MinVersion: tls.VersionTLS12}
}

func addAdminServer(t *testing.T, admin *adminservice.Service, name string, node agentNode) *adminmodels.Server {
	t.Helper()
	server, err := admin.CreateServer(adminservice.ServerInput{
		Name: name,
		SSH:  adminmodels.SSHConfig{Host: node.ip, User: "root", Password: "unused-by-e2e"},
		Agent: adminmodels.Agent{
			Address: node.address,
			TLS:     node.tlsMaterial,
		},
	})
	if err != nil {
		t.Fatalf("create admin server %s: %v", name, err)
	}
	return server
}

func createAdminInterface(t *testing.T, admin *adminservice.Service, server *adminmodels.Server, name, address string, listenPort uint16) *adminmodels.Interface {
	t.Helper()
	cfg := agentmodels.InterfaceConfig{Interface: name, Address: address, ListenPort: listenPort}
	agentmodels.GenerateAmneziaParams(&cfg)
	iface, err := admin.CreateInterface(server.ID.String(), cfg)
	if err != nil {
		t.Fatalf("create interface %s on %s through admin: %v", name, server.Name, err)
	}
	if !iface.InSync {
		t.Fatalf("interface %s on %s is not synced: %s", name, server.Name, iface.LastSyncError)
	}
	return iface
}

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

func dumpTunnelState(ctx context.Context, t *testing.T, nodes []agentNode) {
	t.Helper()
	for _, node := range nodes {
		dumpLiveInterfaces(t, node)
		dumpNetworkState(ctx, t, node.container, node.container.GetContainerID())
		dumpAgentLogs(ctx, t, node.container, node.container.GetContainerID())
	}
}

func dumpLiveInterfaces(t *testing.T, node agentNode) {
	t.Helper()
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: clientTLSConfig(t, node.tlsMaterial)}, Timeout: 5 * time.Second}
	response, err := client.Get("https://" + node.address + "/interfaces/")
	if err != nil {
		t.Logf("read live interfaces: %v", err)
		return
	}
	defer func() { _ = response.Body.Close() }()
	var configs []agentmodels.InterfaceConfig
	if err := json.NewDecoder(response.Body).Decode(&configs); err != nil {
		t.Logf("decode live interfaces: %v", err)
		return
	}
	for index := range configs {
		configs[index].PrivateKey = agentmodels.Key{}
	}
	data, err := json.Marshal(configs)
	if err != nil {
		t.Logf("encode live interfaces: %v", err)
		return
	}
	t.Logf("=== %s live interfaces ===\n%s", node.ip, data)
}

func dumpNetworkState(ctx context.Context, t *testing.T, c testcontainers.Container, name string) {
	t.Helper()
	for _, command := range [][]string{{"ip", "address"}, {"ip", "route", "show", "table", "all"}, {"ip", "rule"}, {"iptables", "-t", "nat", "-S"}, {"iptables", "-t", "mangle", "-S"}, {"ufw", "status", "verbose"}, {"firewall-cmd", "--direct", "--get-all-rules"}, {"firewall-cmd", "--permanent", "--direct", "--get-all-rules"}} {
		_, output, err := c.Exec(ctx, command)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(output)
		t.Logf("=== %s: %s ===\n%s", name, strings.Join(command, " "), data)
	}
	for _, command := range [][]string{{"sh", "-c", "for f in /data/xray/cores/*/core.log /data/hysteria/cores/*/core.log; do [ ! -f \"$f\" ] || { echo \"=== $f ===\"; cat \"$f\"; }; done"}} {
		_, output, err := c.Exec(ctx, command)
		if err != nil {
			continue
		}
		data, _ := io.ReadAll(output)
		t.Logf("=== %s core logs ===\n%s", name, data)
	}
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
