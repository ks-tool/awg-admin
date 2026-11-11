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
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ks-tool/vpn-admin/internal/agentclient"
	"github.com/ks-tool/vpn-admin/internal/config"
	"github.com/ks-tool/vpn-admin/models"
)

func providerCacheRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return config.AdminCachePath(home), nil
}

func providerDefinitionCachePath(root string, definition models.ResourceDefinition) string {
	apiVersion := strings.ReplaceAll(definition.APIVersion, "/", "__")
	return filepath.Join(root, apiVersion+"."+definition.Kind+".spec")
}

func cacheProviderDefinition(root string, definition models.ResourceDefinition) error {
	if len(definition.APIVersion) == 0 || len(definition.Kind) == 0 {
		return errors.New("cached resource definition requires apiVersion and kind")
	}
	data, err := json.MarshalIndent(definition, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".schema-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, providerDefinitionCachePath(root, definition))
}

func cacheProviderManifest(manifest *models.Manifest) error {
	root, err := providerCacheRoot()
	if err != nil {
		return err
	}
	for _, provider := range manifest.Providers {
		for _, definition := range provider.Resources {
			if err := cacheProviderDefinition(root, definition); err != nil {
				return err
			}
		}
	}
	return nil
}

func cachedProviderManifest(serverID string) (*models.Manifest, error) {
	root, err := providerCacheRoot()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, os.ErrNotExist
	}
	if err != nil {
		return nil, err
	}
	providers := make(map[string]*models.ProviderDefinition)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".spec" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		var definition models.ResourceDefinition
		if err := json.Unmarshal(data, &definition); err != nil {
			return nil, err
		}
		name := providerFromAPIVersion(definition.APIVersion)
		if len(name) == 0 {
			continue
		}
		provider := providers[name]
		if provider == nil {
			provider = &models.ProviderDefinition{Name: name, Version: "cached"}
			providers[name] = provider
		}
		provider.Resources = append(provider.Resources, definition)
	}
	if len(providers) == 0 {
		return nil, os.ErrNotExist
	}
	manifest := &models.Manifest{ProtocolVersion: models.ProtocolVersion, AgentID: serverID, Providers: make([]models.ProviderDefinition, 0, len(providers))}
	for _, provider := range providers {
		sort.Slice(provider.Resources, func(i, j int) bool { return provider.Resources[i].Kind < provider.Resources[j].Kind })
		manifest.Providers = append(manifest.Providers, *provider)
	}
	sort.Slice(manifest.Providers, func(i, j int) bool { return manifest.Providers[i].Name < manifest.Providers[j].Name })
	return manifest, nil
}

func (s *Service) fetchProviderManifest(serverID string) (*models.Manifest, error) {
	var manifest *models.Manifest
	err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		manifest, err = client.Manifest(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := cacheProviderManifest(manifest); err != nil {
		return nil, err
	}
	return manifest, nil
}

func (r *providerRegistry) beginRefresh(serverID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.refreshing[serverID] {
		return false
	}
	r.refreshing[serverID] = true
	return true
}

func (r *providerRegistry) endRefresh(serverID string) {
	r.mu.Lock()
	delete(r.refreshing, serverID)
	r.mu.Unlock()
}

func (s *Service) refreshProviderManifest(serverID string) {
	if !s.providerRegistry.beginRefresh(serverID) {
		return
	}
	defer s.providerRegistry.endRefresh(serverID)
	manifest, err := s.fetchProviderManifest(serverID)
	if err != nil {
		return
	}
	for _, provider := range manifest.Providers {
		s.providerRegistry.store(manifest.AgentID, provider.Version, provider)
	}
}
