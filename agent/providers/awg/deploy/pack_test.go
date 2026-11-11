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

package providerdeploy

import (
	"path/filepath"
	"testing"

	"github.com/horchestra/pack/sdk"
	"github.com/ks-tool/vpn-agent/models"
)

func TestPackOwnsDependenciesAndRegistration(t *testing.T) {
	agentDB := t.TempDir()
	deployment := models.ProviderDeployment{Name: "awg", Artifact: "oci://ghcr.io/ks-tool/vpn-providers/awg:v1"}
	pack, values, err := New(deployment, agentDB)
	if err != nil {
		t.Fatal(err)
	}
	packages := pack.PackRoot().Packages["debian"]
	if packages.Sysctls["net.ipv4.ip_forward"] != "1" {
		t.Fatalf("AWG sysctls = %#v", packages.Sysctls)
	}
	for _, name := range []string{"ca-certificates", "iproute2", "procps"} {
		found := false
		for _, dependency := range packages.Prepare {
			found = found || dependency == name
		}
		if !found {
			t.Errorf("dependency %s is missing: %#v", name, packages.Prepare)
		}
	}
	registrationPath := filepath.Join(agentDB, "providers.d", "awg.conf")
	if values["registrationPath"] != registrationPath {
		t.Fatalf("registration path = %q", values["registrationPath"])
	}
	foundRegistration := false
	for _, file := range packages.Content {
		if file.Values == "registry" && file.Path == registrationPath {
			foundRegistration = true
		}
	}
	if !foundRegistration {
		t.Fatalf("registration content does not use absolute path %q: %#v", registrationPath, packages.Content)
	}
	registry := values["registry"].(map[string]any)
	provider := registry["providers"].([]any)[0].(map[string]any)
	if provider["command"] != "/usr/local/bin/vpn-provider-awg" || provider["address"] != "http://127.0.0.1:9091" {
		t.Fatalf("unexpected registration: %#v", provider)
	}
	if _, forced := provider["args"]; forced {
		t.Fatalf("AWG must select its backend automatically: %#v", provider)
	}
	if err := sdk.Generate(t.TempDir(), pack); err != nil {
		t.Fatal(err)
	}
}

func TestKernelPackages(t *testing.T) {
	deployment := models.ProviderDeployment{Name: "awg", Artifact: "oci://ghcr.io/ks-tool/vpn-providers/awg:v1"}
	pack, _, err := New(deployment, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		os       string
		headers  string
		packages int
	}{
		{os: "debian", headers: "linux-headers-${os:kernel}", packages: 1},
		{os: "fedora", headers: "kernel-devel-${os:kernel}", packages: 2},
	} {
		t.Run(test.os, func(t *testing.T) {
			packages := pack.PackRoot().Packages[test.os]
			foundHeaders := false
			for _, dependency := range packages.Prepare {
				foundHeaders = foundHeaders || dependency == test.headers
				if dependency == "software-properties-common" || dependency == "python3-launchpadlib" {
					t.Errorf("unused repository helper %s is installed", dependency)
				}
			}
			if !foundHeaders {
				t.Errorf("running kernel headers are missing: %#v", packages.Prepare)
			}
			if len(packages.Names) != test.packages {
				t.Errorf("kernel packages = %#v", packages.Names)
			}
			if len(packages.Repo) == 0 {
				t.Error("Amnezia repository is not configured")
			}
			if len(packages.Commands) != 1 || len(packages.Commands[0]) != 2 || packages.Commands[0][0] != "modprobe" || packages.Commands[0][1] != "amneziawg" {
				t.Errorf("kernel module is not loaded: %#v", packages.Commands)
			}
			if len(packages.Content) != 3 || packages.Content[2].Path != "/etc/modules-load.d/amneziawg.conf" ||
				packages.Content[2].Values != "files/kernelModule" || packages.Content[2].Format != "text" {
				t.Errorf("kernel module is not loaded at boot: %#v", packages.Content)
			}
		})
	}
}

func TestUserspacePackagesSkipKernel(t *testing.T) {
	deployment := models.ProviderDeployment{
		Name: "awg", Artifact: "oci://ghcr.io/ks-tool/vpn-providers/awg:v1", Options: map[string]any{"userspace": true},
	}
	pack, _, err := New(deployment, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, os := range []string{"debian", "fedora"} {
		packages := pack.PackRoot().Packages[os]
		if len(packages.Names) != 0 || len(packages.Repo) != 0 || len(packages.Commands) != 0 || len(packages.Content) != 2 {
			t.Errorf("%s userspace pack installs a kernel module: %#v", os, packages)
		}
	}
	if err := sdk.Generate(t.TempDir(), pack); err != nil {
		t.Fatal(err)
	}
}

func TestUserspaceOption(t *testing.T) {
	deployment := models.ProviderDeployment{Name: "awg", Artifact: "oci://ghcr.io/ks-tool/vpn-providers/awg:v1", Options: map[string]any{"userspace": true}}
	_, values, err := New(deployment, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	registry := values["registry"].(map[string]any)
	provider := registry["providers"].([]any)[0].(map[string]any)
	args := provider["args"].([]any)
	if len(args) != 1 || args[0] != "--userspace" {
		t.Fatalf("unexpected userspace registration: %#v", provider)
	}
	for _, options := range []map[string]any{{"userspace": "yes"}, {"unknown": true}} {
		deployment.Options = options
		if _, _, err := New(deployment, t.TempDir()); err == nil {
			t.Fatalf("invalid options were accepted: %#v", options)
		}
	}
}
