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
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	packcmd "github.com/horchestra/pack/cmd"
	"github.com/horchestra/pack/pkg/render"
	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"
	"github.com/ks-tool/vpn-admin/internal/config"
	"github.com/ks-tool/vpn-admin/models"
	vpnagentpack "github.com/ks-tool/vpn-admin/pack/vpn-agent"
)

const (
	packTargetName   = "host"
	packTargetSource = "https://github.com/horchestra/targets/releases/download/host/v0.2.3/pack-target-host_{os}_{arch}.tar.gz"
	packMirrorURL    = "http://127.0.0.1:8081"
	packApplyTimeout = 15 * time.Minute
)

var embeddedPackApply = packcmd.Apply
var packTargetLockMu sync.Mutex

type packRelease struct {
	name        string
	step        string
	binary      string
	localBinary string
	pack        sdk.Pack
	values      map[string]any
	credentials []render.Credential
	firewall    []string
	registry    models.Registry
}

type firewallPack struct{}

func (*firewallPack) SetDefaults() {}

func (*firewallPack) Meta() sdk.Meta {
	return sdk.Meta{Name: "vpn-firewall", Version: "1.0.0", Description: "VPN input interfaces", License: "Apache-2.0"}
}

func (*firewallPack) PackRoot() packschema.Root {
	return packschema.Root{Packages: map[string]packschema.OSInstall{
		"debian": {},
		"rhel":   {},
	}}
}

// SyncFirewall accepts traffic on the current VPN interfaces without
// reinstalling the provider or restarting the agent. The host target keeps rules additive.
func SyncFirewall(ctx context.Context, srv models.Server, interfaceNames []string, passphrase, sudoPassword string) error {
	rules := packFirewallRules(interfaceNames)
	if len(rules) == 0 {
		return nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	item := packRelease{
		name: "VPN firewall", step: "firewall", pack: &firewallPack{}, values: map[string]any{}, firewall: rules,
	}
	if err := applyPack(ctx, srv, item, passphrase, sudoPassword, config.AdminCachePath(home)); err != nil {
		return fmt.Errorf("update VPN firewall: %w", err)
	}
	return nil
}

// ToPack installs only the agent. Providers are deployed separately through
// ProviderToPack. A local binary is shipped as pack content; an OCI artifact is
// downloaded by the host target through its registry proxy.
func ToPack(ctx context.Context, srv models.Server, deployment models.AgentDeployment, passphrase, sudoPassword string, onStep func(string)) error {
	agentValues, agentCredentials, err := agentPackValues(srv, deployment.Artifact)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if onStep != nil {
		onStep("install_agent")
	}
	agentPack := &vpnagentpack.Config{SelfDeploy: len(deployment.BinaryPath) > 0}
	item := packRelease{
		name: "vpn-agent", step: "agent", binary: "vpn-agent", localBinary: deployment.BinaryPath,
		pack: agentPack, values: agentValues, credentials: agentCredentials, registry: deployment.Registry,
	}
	if err := applyPack(ctx, srv, item, passphrase, sudoPassword, config.AdminCachePath(home)); err != nil {
		return fmt.Errorf("install vpn-agent: %w", err)
	}
	return nil
}

func ProviderToPack(ctx context.Context, srv models.Server, provider models.ProviderDeployment, registry models.Registry, passphrase, sudoPassword string, onStep func(string)) error {
	pack, values, err := newProviderPack(provider)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if onStep != nil {
		onStep("install_provider_" + provider.Name)
	}
	item := packRelease{
		name: provider.Name, step: "provider_" + provider.Name, binary: pack.Meta().Name, localBinary: provider.BinaryPath,
		pack: pack, values: values, registry: registry,
	}
	if err := applyPack(ctx, srv, item, passphrase, sudoPassword, config.AdminCachePath(home)); err != nil {
		return fmt.Errorf("install %s provider: %w", provider.Name, err)
	}
	return nil
}

func applyPack(ctx context.Context, srv models.Server, item packRelease, passphrase, sudoPassword, cache string) error {
	if len(srv.SSH.Password) > 0 && len(srv.SSH.Key) == 0 && len(srv.SSH.KeyData) == 0 {
		return fmt.Errorf("pack host target requires SSH key authentication; password authentication is not supported")
	}
	if len(srv.SSH.User) > 0 && srv.SSH.User != "root" && len(sudoPassword) > 0 {
		return fmt.Errorf("pack host target requires passwordless sudo for a non-root SSH user")
	}
	ctx, cancel := context.WithTimeout(ctx, packApplyTimeout)
	defer cancel()
	root := filepath.Join(cache, "releases")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(root, item.step+"-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(dir) }()

	var binary []byte
	var contentPack interface{ setContentDir(string) }

	if len(item.localBinary) > 0 {
		binary, err = os.ReadFile(item.localBinary)
		if err != nil {
			return fmt.Errorf("read local binary: %w", err)
		}
		contentPack, _ = item.pack.(interface{ setContentDir(string) })
		if contentPack != nil {
			contentDir := filepath.Join(dir, "content")
			if err := os.MkdirAll(contentDir, 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(filepath.Join(contentDir, item.binary), binary, 0o600); err != nil {
				return fmt.Errorf("prepare local binary: %w", err)
			}
			contentPack.setContentDir(dir)
		}
	}
	if err := sdk.Generate(filepath.Join(dir, "packs"), item.pack); err != nil {
		return err
	}
	if len(item.localBinary) > 0 && contentPack == nil {
		path := filepath.Join(dir, "packs", item.pack.Meta().Name, "templates", item.binary)
		if err := os.WriteFile(path, binary, 0o600); err != nil {
			return fmt.Errorf("prepare local binary: %w", err)
		}
	}
	if err := writePackJSON(filepath.Join(dir, "release.yaml"), map[string]any{
		"pack": filepath.Join(dir, "packs", item.pack.Meta().Name),
		"targets": map[string]any{
			packTargetName: map[string]any{"url": packTargetSource},
		},
	}); err != nil {
		return err
	}
	values := make(map[string]any, len(item.values))

	maps.Copy(values, item.values)

	var registry string
	if artifact, ok := item.values["artifact"].(string); ok {
		artifactURL, artifactRegistry, err := packArtifactURL(artifact, item.binary)
		if err != nil {
			return err
		}
		values["artifact"] = artifactURL
		registry = artifactRegistry
	} else if len(item.binary) > 0 && len(item.localBinary) == 0 {
		return fmt.Errorf("%s artifact is required", item.name)
	}
	if err := writePackJSON(filepath.Join(dir, "values.yaml"), map[string]any{"values": values}); err != nil {
		return err
	}
	host := map[string]any{
		"host":     srv.SSH.Host,
		"user":     srv.SSH.User,
		"insecure": true,
	}
	if srv.SSH.Port != 0 {
		host["port"] = strconv.FormatUint(uint64(srv.SSH.Port), 10)
	}
	if len(srv.SSH.KeyData) > 0 {
		identity := filepath.Join(dir, "ssh-identity")
		if err := os.WriteFile(identity, []byte(srv.SSH.KeyData), 0o600); err != nil {
			return err
		}
		host["identity"] = identity
	} else if len(srv.SSH.Key) > 0 {
		host["identity"] = srv.SSH.Key
	}
	placement := map[string]any{"hosts": []string{"server"}, "packages": []string{}}
	if len(item.firewall) > 0 {
		placement["firewall"] = item.firewall
	}
	env := map[string]any{
		"target": packTargetName,
		"state":  filepath.Join(cache, "host-state.json"),
		"hosts":  map[string]any{"server": host},
		"keys": map[string]any{
			item.pack.Meta().Name: placement,
		},
		"run": map[string]any{"jobs": 1},
	}
	if len(registry) > 0 {
		env["mirror_registry"] = registry
		if item.registry.PlainHTTP {
			env["mirror_plain_http"] = true
		}
		if len(item.registry.Username) > 0 {
			env["mirror_username_env"] = "VPN_ADMIN_OCI_USERNAME"
			env["mirror_password_env"] = "VPN_ADMIN_OCI_PASSWORD"
		}
	}
	if len(item.credentials) > 0 {
		secrets := filepath.Join(dir, "secrets")
		if err := os.MkdirAll(secrets, 0o700); err != nil {
			return err
		}
		provision := make([]string, 0, len(item.credentials))
		for _, credential := range item.credentials {
			pointer := "/tls/" + credential.Leaf
			provision = append(provision, pointer)
			name := "tls." + credential.Leaf
			if err := os.WriteFile(filepath.Join(secrets, name), []byte(credential.Value), 0o600); err != nil {
				return err
			}
		}
		env["secrets"] = map[string]any{"provider": "file:" + secrets, "provision": provision}
	}
	if err := writePackJSON(filepath.Join(dir, "env.yaml"), env); err != nil {
		return err
	}
	options := packcmd.ApplyOptions{Dir: dir, CacheDir: filepath.Join(cache, "pack"), Refresh: true, Env: map[string]string{}}
	if len(passphrase) > 0 {
		options.Env["PACK_SSH_PASSPHRASE"] = passphrase
	}
	if len(item.registry.Username) > 0 {
		options.Env["VPN_ADMIN_OCI_USERNAME"] = item.registry.Username
		options.Env["VPN_ADMIN_OCI_PASSWORD"] = item.registry.Password
	}
	packTargetLockMu.Lock()
	defer packTargetLockMu.Unlock()
	lockPath := filepath.Join(root, item.step+".lock.json")
	cachedLock, err := os.ReadFile(lockPath)
	if err == nil {
		if err := os.WriteFile(filepath.Join(dir, "release.lock.json"), cachedLock, 0o600); err != nil {
			return fmt.Errorf("restore pack target lock: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read pack target lock: %w", err)
	}
	if err := embeddedPackApply(ctx, options); err != nil {
		return err
	}
	generatedLock, err := os.ReadFile(filepath.Join(dir, "release.lock.json"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read generated pack target lock: %w", err)
	}
	if err := os.WriteFile(lockPath, generatedLock, 0o600); err != nil {
		return fmt.Errorf("save pack target lock: %w", err)
	}
	return nil
}

func packFirewallRules(interfaceNames []string) []string {
	seen := map[string]bool{}
	rules := make([]string, 0, len(interfaceNames))
	for _, name := range interfaceNames {
		if len(name) == 0 || seen[name] {
			continue
		}
		seen[name] = true
		rules = append(rules, "i:"+name)
	}
	return rules
}

func packArtifactURL(artifact, binary string) (string, string, error) {
	if strings.HasPrefix(artifact, "https://") || strings.HasPrefix(artifact, "http://") {
		return artifact, "", nil
	}
	if !strings.HasPrefix(artifact, "oci://") {
		return "", "", fmt.Errorf("unsupported artifact URL %q", artifact)
	}
	ref := strings.TrimPrefix(artifact, "oci://")
	registry, path, ok := strings.Cut(ref, "/")
	if !ok || len(registry) == 0 || len(path) == 0 || strings.Contains(path, "@") {
		return "", "", fmt.Errorf("OCI artifact %q must include a registry, repository and tag", artifact)
	}
	name := path[strings.LastIndex(path, "/")+1:]
	if !strings.Contains(name, ":") || strings.HasSuffix(name, ":") {
		return "", "", fmt.Errorf("OCI artifact %q must include a tag", artifact)
	}
	return packMirrorURL + "/" + path + "/" + binary + "?platform=linux/${os:arch}", registry, nil
}

func writePackJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func agentPackValues(srv models.Server, artifact string) (map[string]any, []render.Credential, error) {
	addr := srv.Agent.Address
	if len(addr) == 0 {
		addr = "127.0.0.1:" + defaultAgentPort
	}
	env := map[string]any{
		"VPN_AGENT_ADDR":         addr,
		"VPN_AGENT_DB":           remoteDBPath,
		"VPN_AGENT_LEGACY_PROXY": "http://127.0.0.1:9091",
	}
	values := map[string]any{"env": env}
	if len(artifact) > 0 {
		values["artifact"] = artifact
	}
	if srv.Agent.TLS.IsZero() {
		return values, nil, nil
	}
	env["VPN_AGENT_TLS_CERT"] = remoteServerCertPath
	env["VPN_AGENT_TLS_KEY"] = remoteServerKeyPath
	env["VPN_AGENT_TLS_CLIENT_CA"] = remoteCACertPath
	values["tls"] = map[string]any{}
	credentials := []render.Credential{
		{Name: "agent-tls", Key: "vpn-agent", Leaf: "ca", Value: srv.Agent.TLS.CA.Certificate},
		{Name: "agent-tls", Key: "vpn-agent", Leaf: "certificate", Value: srv.Agent.TLS.Server.Certificate},
		{Name: "agent-tls", Key: "vpn-agent", Leaf: "privateKey", Value: srv.Agent.TLS.Server.PrivateKey},
	}
	return values, credentials, nil
}
