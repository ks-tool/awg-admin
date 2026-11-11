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

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"uuid"

	"github.com/ks-tool/vpn-admin/internal/config"
	"github.com/ks-tool/vpn-admin/models"
)

func TestProviderDefinitionCacheUsesAPIVersionFileNames(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	svc := newTestService(t)
	serverID := uuid.New().String()
	definition := models.ResourceDefinition{APIVersion: "xray.ks-tool.dev/v1", Kind: "Interface", SpecSchema: json.RawMessage(`{"type":"object"}`)}
	manifest := &models.Manifest{ProtocolVersion: models.ProtocolVersion, AgentID: "agent", Providers: []models.ProviderDefinition{{Name: "xray", Version: "1", Resources: []models.ResourceDefinition{definition}}}}
	if err := cacheProviderManifest(manifest); err != nil {
		t.Fatal(err)
	}
	if err := svc.cacheServerProviderManifest(serverID, manifest); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(config.AdminCachePath(home), "xray.ks-tool.dev__v1.Interface.spec")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	cached, err := svc.cachedProviderManifest(serverID)
	if err != nil || len(cached.Providers) != 1 || cached.Providers[0].Name != "xray" || len(cached.Providers[0].Resources) != 1 {
		t.Fatalf("unexpected cached manifest: %+v, %v", cached, err)
	}
	if cached.Providers[0].Resources[0].APIVersion != definition.APIVersion {
		t.Fatalf("unexpected cached definition: %+v", cached.Providers[0].Resources[0])
	}
	if _, err := svc.cachedProviderManifest(uuid.New().String()); !os.IsNotExist(err) {
		t.Fatalf("global schema leaked into another server manifest: %v", err)
	}
}

func TestCachedProviderManifestTracksServerProviderRemoval(t *testing.T) {
	svc := newTestService(t)
	serverID := uuid.New().String()
	otherServerID := uuid.New().String()
	xray := models.ProviderDefinition{Name: "xray", Version: "1", Resources: []models.ResourceDefinition{{APIVersion: "xray.ks-tool.dev/v1", Kind: "Interface"}}}
	awg := models.ProviderDefinition{Name: "awg", Version: "1", Resources: []models.ResourceDefinition{{APIVersion: "awg.ks-tool.dev/v1", Kind: "Interface"}}}
	if err := svc.cacheServerProviderManifest(serverID, &models.Manifest{Providers: []models.ProviderDefinition{xray, awg}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.cacheServerProviderManifest(otherServerID, &models.Manifest{Providers: []models.ProviderDefinition{xray}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.cacheServerProviderManifest(serverID, &models.Manifest{Providers: []models.ProviderDefinition{awg}}); err != nil {
		t.Fatal(err)
	}
	cached, err := svc.cachedProviderManifest(serverID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cached.Providers) != 1 || cached.Providers[0].Name != "awg" {
		t.Fatalf("removed provider survived in server manifest: %+v", cached.Providers)
	}
	other, err := svc.cachedProviderManifest(otherServerID)
	if err != nil || len(other.Providers) != 1 || other.Providers[0].Name != "xray" {
		t.Fatalf("other server manifest changed: %+v, %v", other, err)
	}
}
