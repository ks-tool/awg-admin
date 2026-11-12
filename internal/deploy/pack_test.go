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

package deploy

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	packcmd "github.com/horchestra/pack/cmd"
	"github.com/ks-tool/vpn-admin/models"
	vpnagentpack "github.com/ks-tool/vpn-admin/pack/vpn-agent"
)

func TestAgentPackValues(t *testing.T) {
	srv := models.Server{Agent: models.Agent{Address: "127.0.0.1:8080", TLS: &models.AgentTLS{
		CA:     models.CertKeyPair{Certificate: "ca-data"},
		Server: models.CertKeyPair{Certificate: "cert-data", PrivateKey: "key-data"},
	}}}
	values, credentials, err := agentPackValues(srv, "oci://ghcr.io/ks-tool/vpn-agent:v2.0.0-alpha.1")
	if err != nil {
		t.Fatal(err)
	}
	if values["artifact"] != "oci://ghcr.io/ks-tool/vpn-agent:v2.0.0-alpha.1" || len(credentials) != 3 {
		t.Fatalf("unexpected agent values: %#v, credentials: %#v", values, credentials)
	}
	env := values["env"].(map[string]any)
	if env["VPN_AGENT_LEGACY_PROXY"] != "http://127.0.0.1:9091" || env["VPN_AGENT_TLS_CERT"] != remoteServerCertPath {
		t.Fatalf("unexpected agent environment: %#v", env)
	}
}

func TestAgentPackLegacyMigrationHooks(t *testing.T) {
	root := (&vpnagentpack.Config{}).PackRoot()
	if root.Readiness == nil || len(root.Hooks) != 4 {
		t.Fatalf("unexpected agent pack hooks: %#v", root.Hooks)
	}
	if len(root.Packages["debian"].Services) > 0 {
		t.Fatalf("agent must restart once after content delivery, not through Services: %#v", root.Packages["debian"].Services)
	}
	stop, remove := root.Hooks[0], root.Hooks[1]
	if stop.Name != "stop-legacy-agent" || string(stop.Phase) != "preInstall" || !stop.Idempotent {
		t.Fatalf("unexpected stop hook: %#v", stop)
	}
	if remove.Name != "remove-legacy-agent" || string(remove.Phase) != "postInstall" || !remove.Idempotent {
		t.Fatalf("unexpected remove hook: %#v", remove)
	}
	if !strings.Contains(stop.Command[2], "systemctl disable --now awg-agent.service") {
		t.Fatalf("stop command = %q", stop.Command[2])
	}
	if !strings.Contains(remove.Command[2], "rm -f /usr/local/bin/awg-agent") || !strings.Contains(remove.Command[2], `rm -f "$legacy_unit"`) {
		t.Fatalf("remove command = %q", remove.Command[2])
	}
	for index, phase := range []string{"postInstall", "postUpgrade"} {
		restart := root.Hooks[index+2]
		if restart.Name != "restart-agent" || string(restart.Phase) != phase || strings.Join(restart.Command, " ") != "sh -c systemctl enable vpn-agent && systemctl restart vpn-agent" {
			t.Fatalf("unexpected restart hook: %#v", restart)
		}
	}
}

func TestPackArtifactURL(t *testing.T) {
	url, registry, err := packArtifactURL("oci://ghcr.io/ks-tool/vpn-agent:v2.0.0-alpha.1", "vpn-agent")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://127.0.0.1:8081/ks-tool/vpn-agent:v2.0.0-alpha.1/vpn-agent?platform=linux/${os:arch}"
	if url != want || registry != "ghcr.io" {
		t.Fatalf("packArtifactURL = %q, %q; want %q, ghcr.io", url, registry, want)
	}
	httpURL := "https://example.test/vpn-agent"
	if url, registry, err = packArtifactURL(httpURL, "vpn-agent"); err != nil || url != httpURL || registry != "" {
		t.Fatalf("HTTP artifact = %q, %q, %v", url, registry, err)
	}
	for _, artifact := range []string{"file:///tmp/agent", "oci://ghcr.io/ks-tool/vpn-agent", "oci://ghcr.io/ks-tool/vpn-agent@sha256:abc"} {
		if _, _, err := packArtifactURL(artifact, "vpn-agent"); err == nil {
			t.Errorf("packArtifactURL(%q) did not fail", artifact)
		}
	}
}

func TestPackFirewallRules(t *testing.T) {
	rules := packFirewallRules([]string{"awg0", "", "awg1", "awg0"})
	if len(rules) != 2 || rules[0] != "i:awg0" || rules[1] != "i:awg1" {
		t.Fatalf("firewall rules = %#v", rules)
	}
}

func TestApplyPackUsesHostTarget(t *testing.T) {
	original := embeddedPackApply
	t.Cleanup(func() { embeddedPackApply = original })
	called := 0
	embeddedPackApply = func(_ context.Context, options packcmd.ApplyOptions) error {
		called++
		if !options.Refresh || options.Env["PACK_SSH_PASSPHRASE"] != "key-passphrase" {
			t.Fatalf("unexpected apply options: %#v", options)
		}
		var release map[string]any
		readPackTestJSON(t, filepath.Join(options.Dir, "release.yaml"), &release)
		if !strings.Contains(release["pack"].(string), (&vpnagentpack.Config{}).Meta().Name) {
			t.Fatalf("unexpected release: %#v", release)
		}
		targets := release["targets"].(map[string]any)
		host := targets[packTargetName].(map[string]any)
		if host["url"] != packTargetSource {
			t.Fatalf("unexpected target source: %#v", host)
		}
		var values struct {
			Values map[string]any `json:"values"`
		}
		readPackTestJSON(t, filepath.Join(options.Dir, "values.yaml"), &values)
		wantArtifact := "http://127.0.0.1:8081/ks-tool/vpn-agent:v1/vpn-agent?platform=linux/${os:arch}"
		if values.Values["artifact"] != wantArtifact {
			t.Fatalf("unexpected values: %#v", values)
		}
		var env map[string]any
		readPackTestJSON(t, filepath.Join(options.Dir, "env.yaml"), &env)
		if env["target"] != packTargetName || env["mirror_registry"] != "ghcr.io" {
			t.Fatalf("unexpected environment: %#v", env)
		}
		hosts := env["hosts"].(map[string]any)
		server := hosts["server"].(map[string]any)
		if server["host"] != "192.0.2.1" || server["insecure"] != true {
			t.Fatalf("unexpected host: %#v", server)
		}
		keys := env["keys"].(map[string]any)
		placement := keys["vpn-agent"].(map[string]any)
		placementHosts := placement["hosts"].([]any)
		if len(placementHosts) != 1 || placementHosts[0] != "server" {
			t.Fatalf("unexpected key placement: %#v", placement)
		}
		packages, ok := placement["packages"].([]any)
		if !ok || len(packages) != 0 {
			t.Fatalf("packages key must be explicit: %#v", placement)
		}
		firewall := placement["firewall"].([]any)
		if len(firewall) != 1 || firewall[0] != "i:awg0" {
			t.Fatalf("unexpected firewall placement: %#v", placement)
		}
		identity, err := os.ReadFile(server["identity"].(string))
		if err != nil || string(identity) != "private-key" {
			t.Fatalf("SSH identity = %q, %v", identity, err)
		}
		secrets := env["secrets"].(map[string]any)
		if secrets["provider"] != "file:"+filepath.Join(options.Dir, "secrets") {
			t.Fatalf("unexpected secret provider: %#v", secrets)
		}
		data, err := os.ReadFile(filepath.Join(options.Dir, "secrets", "tls.ca"))
		if err != nil || string(data) != "ca-data" {
			t.Fatalf("CA secret = %q, %v", data, err)
		}
		return nil
	}
	srv := models.Server{SSH: models.SSHConfig{Host: "192.0.2.1", KeyData: "private-key"}, Agent: models.Agent{TLS: &models.AgentTLS{
		CA: models.CertKeyPair{Certificate: "ca-data"},
	}}}
	values, credentials, err := agentPackValues(srv, "oci://ghcr.io/ks-tool/vpn-agent:v1")
	if err != nil {
		t.Fatal(err)
	}
	item := packRelease{name: "vpn-agent", step: "agent", binary: "vpn-agent", pack: &vpnagentpack.Config{}, values: values, credentials: credentials, firewall: []string{"i:awg0"}}
	if err := applyPack(context.Background(), srv, item, "key-passphrase", "", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("embedded apply called %d times", called)
	}
}

func TestApplyPackRejectsUnsupportedSSHSettings(t *testing.T) {
	item := packRelease{values: map[string]any{"artifact": "https://example.test/agent"}}
	if err := applyPack(context.Background(), models.Server{SSH: models.SSHConfig{Password: "secret"}}, item, "", "", t.TempDir()); err == nil || !strings.Contains(err.Error(), "key authentication") {
		t.Fatalf("password authentication error = %v", err)
	}
	srv := models.Server{SSH: models.SSHConfig{User: "admin", Key: "/tmp/id"}}
	if err := applyPack(context.Background(), srv, item, "", "sudo-secret", t.TempDir()); err == nil || !strings.Contains(err.Error(), "passwordless sudo") {
		t.Fatalf("sudo password error = %v", err)
	}
}

func readPackTestJSON(t *testing.T, path string, target any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func TestSyncFirewallUsesDedicatedPack(t *testing.T) {
	original := embeddedPackApply
	t.Cleanup(func() { embeddedPackApply = original })
	t.Setenv("HOME", t.TempDir())
	embeddedPackApply = func(_ context.Context, options packcmd.ApplyOptions) error {
		var release map[string]any
		readPackTestJSON(t, filepath.Join(options.Dir, "release.yaml"), &release)
		if !strings.Contains(release["pack"].(string), "vpn-firewall") {
			t.Fatalf("unexpected release: %#v", release)
		}
		var values struct {
			Values map[string]any `json:"values"`
		}
		readPackTestJSON(t, filepath.Join(options.Dir, "values.yaml"), &values)
		if _, ok := values.Values["artifact"]; ok {
			t.Fatalf("firewall-only pack must not download an artifact: %#v", values)
		}
		var env map[string]any
		readPackTestJSON(t, filepath.Join(options.Dir, "env.yaml"), &env)
		keys := env["keys"].(map[string]any)
		placement := keys["vpn-firewall"].(map[string]any)
		firewall := placement["firewall"].([]any)
		if len(firewall) != 2 || firewall[0] != "i:awg0" || firewall[1] != "i:awg1" {
			t.Fatalf("unexpected firewall placement: %#v", placement)
		}
		if _, ok := env["mirror_registry"]; ok {
			t.Fatalf("firewall-only pack must not start an OCI mirror: %#v", env)
		}
		return nil
	}
	srv := models.Server{SSH: models.SSHConfig{Host: "192.0.2.1", User: "root", Key: "/tmp/id"}}
	if err := SyncFirewall(context.Background(), srv, []string{"awg0", "awg1"}, "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPackSelfDeploysLocalAgentBinary(t *testing.T) {
	original := embeddedPackApply
	t.Cleanup(func() { embeddedPackApply = original })
	binary := []byte{'b', 'i', 'n', 0, 'a', 'r', 'y'}
	path := filepath.Join(t.TempDir(), "vpn-agent")
	if err := os.WriteFile(path, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	embeddedPackApply = func(_ context.Context, options packcmd.ApplyOptions) error {
		deployed, err := os.ReadFile(filepath.Join(options.Dir, "packs", "vpn-agent", "templates", "vpn-agent"))
		if err != nil || string(deployed) != string(binary) {
			t.Fatalf("local binary = %q, %v", deployed, err)
		}
		var values struct {
			Values map[string]any `json:"values"`
		}
		readPackTestJSON(t, filepath.Join(options.Dir, "values.yaml"), &values)
		if _, ok := values.Values["artifact"]; ok {
			t.Fatalf("self-deploy must not use an artifact URL: %#v", values)
		}
		return nil
	}
	srv := models.Server{SSH: models.SSHConfig{Host: "192.0.2.1", Key: "/tmp/id"}}
	values, _, err := agentPackValues(srv, "")
	if err != nil {
		t.Fatal(err)
	}
	item := packRelease{
		name: "vpn-agent", step: "agent", binary: "vpn-agent", localBinary: path,
		pack: &vpnagentpack.Config{SelfDeploy: true}, values: values,
	}
	if err := applyPack(context.Background(), srv, item, "", "", t.TempDir()); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPackUsesPrivateRegistrySettings(t *testing.T) {
	original := embeddedPackApply
	t.Cleanup(func() { embeddedPackApply = original })
	embeddedPackApply = func(_ context.Context, options packcmd.ApplyOptions) error {
		if options.Env["VPN_ADMIN_OCI_USERNAME"] != "reader" || options.Env["VPN_ADMIN_OCI_PASSWORD"] != "secret" {
			t.Fatalf("registry credentials were not passed to pack: %#v", options.Env)
		}
		var env map[string]any
		readPackTestJSON(t, filepath.Join(options.Dir, "env.yaml"), &env)
		if env["mirror_plain_http"] != true || env["mirror_username_env"] != "VPN_ADMIN_OCI_USERNAME" || env["mirror_password_env"] != "VPN_ADMIN_OCI_PASSWORD" {
			t.Fatalf("unexpected registry environment: %#v", env)
		}
		return nil
	}
	srv := models.Server{SSH: models.SSHConfig{Host: "192.0.2.1", Key: "/tmp/id"}}
	item := packRelease{
		name: "vpn-agent", step: "agent", binary: "vpn-agent", pack: &vpnagentpack.Config{},
		values:   map[string]any{"artifact": "oci://registry.example/team/vpn-agent:v1"},
		registry: models.Registry{Username: "reader", Password: "secret", PlainHTTP: true},
	}
	if err := applyPack(context.Background(), srv, item, "", "", t.TempDir()); err != nil {
		t.Fatal(err)
	}
}
