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
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/ks-tool/vpn-admin/internal/pki"
	adminservice "github.com/ks-tool/vpn-admin/internal/service"
	adminmodels "github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage/boltdb"

	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

type providerEndpoint struct {
	node     int
	provider string
}

func TestXrayProviderChain(t *testing.T) {
	if os.Getenv("PROVIDER_E2E") == "" {
		t.Skip("set PROVIDER_E2E=1 to run (needs Docker with /dev/net/tun + NET_ADMIN)")
	}
	runProviderChain(t, "xray", []providerEndpoint{{0, "xray"}, {1, "xray"}, {2, "xray"}, {3, "xray"}, {4, "xray"}})
}

func TestHysteriaProviderChain(t *testing.T) {
	if os.Getenv("PROVIDER_E2E") == "" {
		t.Skip("set PROVIDER_E2E=1 to run (needs Docker with /dev/net/tun + NET_ADMIN)")
	}
	runProviderChain(t, "hysteria2", []providerEndpoint{{0, "hysteria2"}, {1, "hysteria2"}, {2, "hysteria2"}, {3, "hysteria2"}, {4, "hysteria2"}})
}

func TestMixedProviderChain(t *testing.T) {
	if os.Getenv("PROVIDER_E2E") == "" {
		t.Skip("set PROVIDER_E2E=1 to run (needs Docker with /dev/net/tun + NET_ADMIN)")
	}
	runProviderChain(t, "xray", []providerEndpoint{
		{0, "xray"}, {1, "xray"}, {1, "awg"}, {2, "awg"}, {2, "hysteria2"}, {3, "hysteria2"},
		{3, "awg"}, {4, "awg"}, {4, "xray"}, {5, "xray"}, {5, "hysteria2"}, {6, "hysteria2"},
		{6, "awg"}, {7, "awg"},
	})
}

func runProviderChain(t *testing.T, clientProtocol string, endpoints []providerEndpoint) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	ctx := context.Background()
	nw, err := network.New(ctx)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	testcontainers.CleanupNetwork(t, nw)
	nodes := make([]agentNode, endpoints[len(endpoints)-1].node+1)
	for index := range nodes {
		nodes[index] = startProviderAgent(ctx, t, nw.Name, fmt.Sprintf("provider-%c", 'a'+index))
	}
	defer func() {
		if t.Failed() {
			dumpTunnelState(ctx, t, nodes)
		}
	}()
	store, err := boltdb.Open(t.TempDir() + "/admin.db")
	if err != nil {
		t.Fatalf("open admin storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	admin := adminservice.New(store)
	servers := make([]*adminmodels.Server, len(nodes))
	resources := make(map[string]*adminmodels.Resource)
	for index := range nodes {
		servers[index] = addAdminServer(t, admin, fmt.Sprintf("%c", 'a'+index), nodes[index])
	}
	for index, endpoint := range endpoints {
		if endpoint.provider == "awg" && index > 0 && endpoints[index-1].node == endpoint.node && endpoints[index-1].provider == "awg" {
			continue
		}
		resource := createProviderInterface(t, admin, servers[endpoint.node], endpoint.node, endpoint.provider)
		resources[resourceKey(endpoint)] = resource
	}
	for index := 0; index < len(endpoints)-1; index++ {
		source := resources[resourceKey(endpoints[index])]
		target := resources[resourceKey(endpoints[index+1])]
		link, err := admin.CreateProviderLink(source.ID, target.ID)
		if err != nil {
			dumpTunnelState(ctx, t, nodes)
			t.Fatalf("create link %+v → %+v: %v", endpoints[index], endpoints[index+1], err)
		}
		if link.Status.Phase != adminmodels.LinkReady {
			t.Fatalf("link %+v → %+v is not ready: %+v", endpoints[index], endpoints[index+1], link.Status)
		}
	}

	clientURI := createProviderClientPeer(t, admin, servers[endpoints[0].node], resources[resourceKey(endpoints[0])], clientProtocol)
	httpServer := startHTTPServer(ctx, t, nw.Name)
	client := startProviderClient(ctx, t, nw.Name, clientProtocol, clientURI)
	target, err := httpServer.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("HTTP server IP: %v", err)
	}
	address := "http://" + net.JoinHostPort(target, "18080") + "/"
	deadline := time.Now().Add(90 * time.Second)
	var output string
	for time.Now().Before(deadline) {
		code, reader, err := client.Exec(ctx, []string{"curl", "-fsS", "--max-time", "10", "--socks5-hostname", "127.0.0.1:1080", address})
		data, _ := io.ReadAll(reader)
		output = string(data)
		if err == nil && code == 0 {
			t.Logf("HTTP response through %d links: %s", len(endpoints)-1, output)
			return
		}
		time.Sleep(2 * time.Second)
	}
	dumpTunnelState(ctx, t, nodes)
	t.Fatalf("HTTP through %s chain failed: %s", clientProtocol, output)
}

func resourceKey(endpoint providerEndpoint) string {
	return fmt.Sprintf("%d/%s", endpoint.node, endpoint.provider)
}

func createProviderInterface(t *testing.T, admin *adminservice.Service, server *adminmodels.Server, node int, provider string) *adminmodels.Resource {
	t.Helper()
	port := 20000 + node*100
	var apiVersion, name string
	var spec map[string]any
	switch provider {
	case "xray":
		apiVersion, name = "xray.ks-tool.dev/v1", "xray0"
		port += 1
		spec = map[string]any{"name": name, "listen": "0.0.0.0", "port": port, "network": "tcp", "tls": false}
	case "hysteria2":
		apiVersion, name = "hysteria2.ks-tool.dev/v1", "hy2"
		port += 2
		spec = map[string]any{"name": name, "listen": "0.0.0.0", "port": port, "serverName": "localhost"}
	case "awg":
		apiVersion, name = "awg.ks-tool.dev/v1", "awg0"
		port += 3
		spec = map[string]any{"iface": name, "addr": fmt.Sprintf("10.10.0.%d/24", node+1), "listen": port}
	default:
		t.Fatalf("unsupported provider %s", provider)
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	resource, err := admin.PutProviderResource(server.ID.String(), provider, "Interface", name, raw)
	if err != nil {
		t.Fatalf("create %s Interface on %s: %v", provider, server.Name, err)
	}
	if resource.APIVersion != apiVersion {
		t.Fatalf("unexpected %s apiVersion: %s", provider, resource.APIVersion)
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		status, err := admin.GetProviderResourceStatus(server.ID.String(), provider, "Interface", name)
		if err == nil && status.ObservedGeneration >= resource.Generation {
			if status.Phase == adminmodels.ResourceReady {
				return resource
			}
			if status.Phase == adminmodels.ResourceDegraded {
				t.Fatalf("%s Interface is degraded: %+v", provider, status.Conditions)
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("%s Interface did not become ready", provider)
	return nil
}

func createProviderClientPeer(t *testing.T, admin *adminservice.Service, server *adminmodels.Server, iface *adminmodels.Resource, protocol string) string {
	t.Helper()
	interfaceID, err := uuid.Parse(iface.ID)
	if err != nil {
		t.Fatal(err)
	}
	user, err := admin.CreateUser(adminservice.UserInput{Name: "e2e-client"})
	if err != nil {
		t.Fatalf("create client user: %v", err)
	}
	updated, err := admin.AddPeer(user.ID.String(), adminservice.AddPeerInput{Name: "client", InterfaceID: interfaceID})
	if err != nil {
		t.Fatalf("create client peer: %v", err)
	}
	if len(updated.Peers) != 1 {
		t.Fatalf("unexpected client peers: %+v", updated.Peers)
	}
	config, err := admin.GetPeerConfig(user.ID.String(), updated.Peers[0].PrivateKey.String())
	if err != nil {
		t.Fatalf("render %s client config: %v", protocol, err)
	}
	return config
}

func startProviderAgent(ctx context.Context, t *testing.T, netName, alias string) agentNode {
	t.Helper()
	tlsMaterial, err := pki.IssueAgentTLS(alias, []net.IP{net.ParseIP("127.0.0.1")}, []string{"localhost"})
	if err != nil {
		t.Fatalf("issue TLS for %s: %v", alias, err)
	}
	clientTLS := clientTLSConfig(t, tlsMaterial)
	arch := runtime.GOARCH
	req := testcontainers.ContainerRequest{
		Context:        "..",
		Dockerfile:     "e2e/Dockerfile.providers",
		BuildArgs:      map[string]*string{"TARGETARCH": &arch},
		Repo:           "vpn-providers-e2e",
		Tag:            "latest",
		KeepImage:      true,
		ExposedPorts:   []string{"8080/tcp"},
		Networks:       []string{netName},
		NetworkAliases: map[string][]string{netName: {alias}},
		Env: map[string]string{
			"VPN_AGENT_TLS_CERT":      "/tls/server.pem",
			"VPN_AGENT_TLS_KEY":       "/tls/server-key.pem",
			"VPN_AGENT_TLS_CLIENT_CA": "/tls/ca.pem",
		},
		Files: []testcontainers.ContainerFile{
			{Reader: strings.NewReader(tlsMaterial.CA.Certificate), ContainerFilePath: "/tls/ca.pem", FileMode: 0o600},
			{Reader: strings.NewReader(tlsMaterial.Server.Certificate), ContainerFilePath: "/tls/server.pem", FileMode: 0o600},
			{Reader: strings.NewReader(tlsMaterial.Server.PrivateKey), ContainerFilePath: "/tls/server-key.pem", FileMode: 0o600},
		},
		WaitingFor: wait.ForHTTP("/v1/manifest").WithPort("8080/tcp").WithTLS(true, clientTLS).WithStartupTimeout(180 * time.Second),
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.CapAdd = append(hc.CapAdd, "NET_ADMIN", "NET_RAW")
			hc.Sysctls = map[string]string{"net.ipv4.ip_forward": "1"}
			hc.Devices = append(hc.Devices, container.DeviceMapping{PathOnHost: "/dev/net/tun", PathInContainer: "/dev/net/tun", CgroupPermissions: "rwm"})
		},
	}
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
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

func startHTTPServer(ctx context.Context, t *testing.T, netName string) testcontainers.Container {
	t.Helper()
	request := testcontainers.ContainerRequest{
		Image: "vpn-providers-e2e:latest", Entrypoint: []string{"python3", "-m", "http.server", "18080", "--bind", "0.0.0.0"},
		Networks: []string{netName}, NetworkAliases: map[string][]string{netName: {"http-exit"}},
	}
	server, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: request, Started: true})
	testcontainers.CleanupContainer(t, server)
	if err != nil {
		t.Fatalf("start HTTP server: %v", err)
	}
	return server
}

func startProviderClient(ctx context.Context, t *testing.T, netName, protocol, clientURI string) testcontainers.Container {
	t.Helper()
	var config string
	var command []string
	configPath := "/client/config.json"
	switch protocol {
	case "xray":
		config = xrayClientConfig(t, clientURI)
		command = []string{"xray", "run", "-config", configPath}
	case "hysteria2":
		config = fmt.Sprintf("server: %q\nsocks5:\n  listen: 127.0.0.1:1080\n", clientURI)
		configPath = "/client/config.yaml"
		command = []string{"hysteria", "client", "-c", configPath}
	default:
		t.Fatalf("unsupported client protocol %s", protocol)
	}
	req := testcontainers.ContainerRequest{
		Image:      "vpn-providers-e2e:latest",
		Entrypoint: []string{"sleep", "infinity"},
		Networks:   []string{netName},
		Files:      []testcontainers.ContainerFile{{Reader: strings.NewReader(config), ContainerFilePath: configPath, FileMode: 0o600}},
		HostConfigModifier: func(hc *container.HostConfig) {
			hc.CapAdd = append(hc.CapAdd, "NET_ADMIN", "NET_RAW")
			hc.Devices = append(hc.Devices, container.DeviceMapping{PathOnHost: "/dev/net/tun", PathInContainer: "/dev/net/tun", CgroupPermissions: "rwm"})
		},
	}
	client, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{ContainerRequest: req, Started: true})
	testcontainers.CleanupContainer(t, client)
	if err != nil {
		t.Fatalf("start client: %v", err)
	}
	_, _, err = client.Exec(ctx, []string{"sh", "-c", strings.Join(command, " ") + " >/tmp/client-core.log 2>&1 &"})
	if err != nil {
		t.Fatalf("start %s client: %v", protocol, err)
	}
	return client
}

func xrayClientConfig(t *testing.T, clientURI string) string {
	t.Helper()
	uri, err := url.Parse(clientURI)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(uri.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	document := map[string]any{
		"log": map[string]any{"loglevel": "warning"},
		"inbounds": []map[string]any{{
			"listen": "127.0.0.1", "port": 1080, "protocol": "socks",
			"settings": map[string]any{"auth": "noauth", "udp": true},
		}},
		"outbounds": []map[string]any{{
			"protocol": "vless",
			"settings": map[string]any{"vnext": []map[string]any{{
				"address": host, "port": port,
				"users": []map[string]any{{"id": uri.User.Username(), "encryption": "none"}},
			}}},
			"streamSettings": map[string]any{"network": "tcp", "security": "none"},
		}},
	}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
