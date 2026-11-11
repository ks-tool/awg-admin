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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	packcmd "github.com/horchestra/pack/cmd"
	"github.com/ks-tool/vpn-admin/models"
)

func TestProviderToPackUsesProviderOwnedPack(t *testing.T) {
	provider := models.ProviderDeployment{
		Name: "awg", Artifact: ociArtifactURL(t), Options: map[string]any{"userspace": true},
	}
	original := embeddedPackApply
	t.Cleanup(func() { embeddedPackApply = original })
	t.Setenv("HOME", t.TempDir())
	called := 0
	embeddedPackApply = func(_ context.Context, options packcmd.ApplyOptions) error {
		called++
		var release map[string]any
		readPackTestJSON(t, filepath.Join(options.Dir, "release.yaml"), &release)
		if !strings.Contains(release["pack"].(string), "vpn-provider-awg") {
			t.Fatalf("unexpected release: %#v", release)
		}
		var values struct {
			Values map[string]any `json:"values"`
		}
		readPackTestJSON(t, filepath.Join(options.Dir, "values.yaml"), &values)
		if values.Values["artifact"] != provider.Artifact {
			t.Fatalf("unexpected artifact: %#v", values.Values)
		}
		if values.Values["registrationPath"] != "/var/lib/vpn-agent/providers.d/awg.conf" {
			t.Fatalf("unexpected registration path: %#v", values.Values)
		}
		registry := values.Values["registry"].(map[string]any)
		registered := registry["providers"].([]any)[0].(map[string]any)
		args := registered["args"].([]any)
		if len(args) != 1 || args[0] != "--userspace" {
			t.Fatalf("provider options were not applied: %#v", registered)
		}
		response, err := http.Get(provider.Artifact)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = response.Body.Close() }()
		binary, err := io.ReadAll(response.Body)
		if err != nil || string(binary) != "test AWG provider binary\n" {
			t.Fatalf("unexpected OCI test binary: %q, %v", binary, err)
		}
		var env map[string]any
		readPackTestJSON(t, filepath.Join(options.Dir, "env.yaml"), &env)
		keys := env["keys"].(map[string]any)
		placement := keys["vpn-provider-awg"].(map[string]any)
		firewall := placement["firewall"].([]any)
		if len(firewall) != 1 || firewall[0] != "i:awg0" {
			t.Fatalf("unexpected firewall: %#v", placement)
		}
		return nil
	}
	srv := models.Server{SSH: models.SSHConfig{Host: "192.0.2.1", User: "root", Key: "/tmp/id"}}
	if err := ProviderToPack(context.Background(), srv, provider, models.Registry{}, []string{"awg0"}, "", "", nil); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("provider apply called %d times", called)
	}
}

type ociDescriptor struct {
	Digest string `json:"digest"`
}

func ociArtifactURL(t *testing.T) string {
	t.Helper()
	root := filepath.Join("testdata", "oci-layout")
	indexData, err := os.ReadFile(filepath.Join(root, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index struct {
		Manifests []ociDescriptor `json:"manifests"`
	}
	if err := json.Unmarshal(indexData, &index); err != nil || len(index.Manifests) != 1 {
		t.Fatalf("invalid OCI index: %v", err)
	}
	manifestData, err := os.ReadFile(filepath.Join(root, "blobs", "sha256", strings.TrimPrefix(index.Manifests[0].Digest, "sha256:")))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Layers []ociDescriptor `json:"layers"`
	}
	if err := json.Unmarshal(manifestData, &manifest); err != nil || len(manifest.Layers) != 1 {
		t.Fatalf("invalid OCI manifest: %v", err)
	}
	binary, err := os.ReadFile(filepath.Join(root, "blobs", "sha256", strings.TrimPrefix(manifest.Layers[0].Digest, "sha256:")))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/linux/amd64/vpn-provider-awg" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(binary)
	}))
	t.Cleanup(server.Close)
	return server.URL + "/linux/amd64/vpn-provider-awg"
}
