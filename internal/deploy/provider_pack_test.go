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
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	packcmd "github.com/horchestra/pack/cmd"
	"github.com/horchestra/pack/pkg/render"
	packtarget "github.com/horchestra/pack/pkg/target"
	"github.com/horchestra/pack/pkg/target/remote"
	"github.com/horchestra/pack/pkg/target/remote/remotepb"
	"github.com/horchestra/pack/sdk"
	packschema "github.com/horchestra/pack/sdk/schema"
	"github.com/ks-tool/vpn-admin/models"
	"google.golang.org/grpc"
)

type fileURLTarget struct {
	remotepb.UnimplementedTargetServer
	unitJSON chan []byte
}

func (f *fileURLTarget) Handshake(context.Context, *remotepb.HandshakeRequest) (*remotepb.HandshakeResponse, error) {
	return &remotepb.HandshakeResponse{Protocol: remote.Protocol, Target: "host", Capabilities: []string{remote.CapFileURL}}, nil
}

func (f *fileURLTarget) Configure(context.Context, *remotepb.ConfigureRequest) (*remotepb.ConfigureResponse, error) {
	return &remotepb.ConfigureResponse{Namespace: "test"}, nil
}

func (f *fileURLTarget) Execute(req *remotepb.ExecuteRequest, stream grpc.ServerStreamingServer[remotepb.ExecuteEvent]) error {
	f.unitJSON <- req.UnitJson
	return stream.Send(&remotepb.ExecuteEvent{Done: true})
}

func TestPackFileURLCapabilityStripsLargeLocalFile(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	target := &fileURLTarget{unitJSON: make(chan []byte, 1)}
	server := grpc.NewServer()
	remotepb.RegisterTargetServer(server, target)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); _ = listener.Close() })

	plugin, err := remote.Connect("host", "test", listener.Addr().String(), "token", time.Second, make(chan error, 1), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(plugin.Close)
	installation, err := plugin.Configure("test", nil)
	if err != nil {
		t.Fatal(err)
	}
	url := "file:///tmp/vpn-provider-awg"
	unit := &render.Unit{Files: []render.File{{Path: "/usr/local/bin/vpn-provider-awg", URL: url, Content: make([]byte, 5<<20)}}}
	if err := installation.Execute(context.Background(), packtarget.Step{N: 1, Kind: packtarget.Apply, Unit: unit}, nil); err != nil {
		t.Fatal(err)
	}
	raw := <-target.unitJSON
	if len(raw) >= 4<<20 {
		t.Fatalf("file content crossed gRPC: %d bytes", len(raw))
	}
	var received render.Unit
	if err := json.Unmarshal(raw, &received); err != nil {
		t.Fatal(err)
	}
	if len(received.Files) != 1 || received.Files[0].URL != url || len(received.Files[0].Content) != 0 {
		t.Fatalf("unexpected unit: %#v", received.Files)
	}
	if len(unit.Files[0].Content) != 5<<20 {
		t.Fatal("the source unit was mutated")
	}
}

func TestProviderToPackUsesGenericDeployCommand(t *testing.T) {
	provider := models.ProviderDeployment{
		Name: "awg", Artifact: ociArtifactURL(t), Options: map[string]any{"userspace": true},
	}
	original := embeddedPackApply
	t.Cleanup(func() { embeddedPackApply = original })
	t.Setenv("HOME", t.TempDir())
	called := 0
	embeddedPackApply = func(_ context.Context, options packcmd.ApplyOptions) error {
		called++
		if len(options.Force) != 1 || options.Force[0] != "vpn-provider-awg" {
			t.Fatalf("explicit provider deployment is not forced: %v", options.Force)
		}
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
		deployment := values.Values["deployment"].(map[string]any)
		if deployment["agentDB"] != "/var/lib/vpn-agent" {
			t.Fatalf("unexpected deployment config: %#v", deployment)
		}
		providerOptions := deployment["options"].(map[string]any)
		if providerOptions["userspace"] != true {
			t.Fatalf("provider options were not passed through: %#v", providerOptions)
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
		return nil
	}
	srv := models.Server{SSH: models.SSHConfig{Host: "192.0.2.1", User: "root", Key: "/tmp/id"}}
	if err := ProviderToPack(context.Background(), srv, provider, models.Registry{}, "", "", nil); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("provider apply called %d times", called)
	}
}

func TestProviderToPackSelfDeploysLocalBinary(t *testing.T) {
	binary := []byte{'p', 'r', 'o', 'v', 'i', 'd', 'e', 'r', 0}
	path := filepath.Join(t.TempDir(), "vpn-provider-awg")
	if err := os.WriteFile(path, binary, 0o700); err != nil {
		t.Fatal(err)
	}
	original := embeddedPackApply
	t.Cleanup(func() { embeddedPackApply = original })
	t.Setenv("HOME", t.TempDir())
	embeddedPackApply = func(_ context.Context, options packcmd.ApplyOptions) error {
		deployed, err := os.ReadFile(filepath.Join(options.Dir, "packs", "vpn-provider-awg", "templates", "vpn-provider-awg"))
		if err != nil || string(deployed) != string(binary) {
			t.Fatalf("local provider binary = %q, %v", deployed, err)
		}
		var values struct {
			Values map[string]any `json:"values"`
		}
		readPackTestJSON(t, filepath.Join(options.Dir, "values.yaml"), &values)
		if _, ok := values.Values["artifact"]; ok {
			t.Fatalf("local provider must not use an artifact URL: %#v", values.Values)
		}
		deployment := values.Values["deployment"].(map[string]any)
		if deployment["agentDB"] != "/var/lib/vpn-agent" {
			t.Fatalf("unexpected deployment config: %#v", deployment)
		}
		return nil
	}
	provider := models.ProviderDeployment{Name: "awg", BinaryPath: path}
	srv := models.Server{SSH: models.SSHConfig{Host: "192.0.2.1", User: "root", Key: "/tmp/id"}}
	if err := ProviderToPack(context.Background(), srv, provider, models.Registry{}, "", "", nil); err != nil {
		t.Fatal(err)
	}
}

func TestProviderPackAcceptsUnknownProvider(t *testing.T) {
	pack, values, err := newProviderPack(models.ProviderDeployment{
		Name: "vpn-huita", Artifact: "oci://registry.example.com/vpn-providers/vpn-huita:v1", Options: map[string]any{"custom": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.Meta().Name != "vpn-provider-vpn-huita" {
		t.Fatalf("unexpected pack name: %s", pack.Meta().Name)
	}
	deployment := values["deployment"].(map[string]any)
	if deployment["options"].(map[string]any)["custom"] != true {
		t.Fatalf("custom options were not preserved: %#v", deployment)
	}
	raw, err := sdk.Schema(pack)
	if err != nil {
		t.Fatal(err)
	}
	document, err := packschema.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := document.Compile("pack://" + pack.Meta().Name)
	if err != nil {
		t.Fatal(err)
	}
	if violations := compiled.Validate(values); len(violations) > 0 {
		t.Fatalf("provider values do not validate: %+v", violations)
	}
	root := pack.PackRoot()
	debian := root.Packages["debian"]
	if len(debian.Commands) > 0 || len(debian.Services) > 0 {
		t.Fatalf("provider deploy must not run before content delivery: %+v", debian)
	}
	if len(debian.Prepare) > 0 || len(debian.Names) > 0 || len(debian.Repo) > 0 || len(debian.Sysctls) > 0 {
		t.Fatalf("admin must leave OS provisioning to the provider: %+v", debian)
	}
	if len(root.Hooks) != 2 {
		t.Fatalf("install/upgrade deploy hooks = %+v", root.Hooks)
	}
	for index, phase := range []string{"postInstall", "postUpgrade"} {
		deploy := root.Hooks[index]
		if string(deploy.Phase) != phase || deploy.Name != "deploy-provider" || strings.Join(deploy.Command, " ") != "/usr/local/bin/vpn-provider-vpn-huita deploy --config /etc/vpn-provider-vpn-huita/deploy.json" {
			t.Fatalf("deploy runs before the provider binary is available: %+v", deploy)
		}
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
