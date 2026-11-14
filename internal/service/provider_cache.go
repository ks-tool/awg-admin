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
	"time"
	"uuid"

	"github.com/ks-tool/vpn-admin/internal/agentclient"
	"github.com/ks-tool/vpn-admin/internal/config"
	"github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage"
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

func (s *Service) cachedProviderManifest(serverID string) (*models.Manifest, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	records, err := s.store.Providers().List(id)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, os.ErrNotExist
	}
	manifest := &models.Manifest{ProtocolVersion: models.ProtocolVersion, Providers: make([]models.ProviderDefinition, 0, len(records))}
	for _, record := range records {
		provider := record.Definition
		sort.Slice(provider.Resources, func(i, j int) bool { return provider.Resources[i].Kind < provider.Resources[j].Kind })
		manifest.Providers = append(manifest.Providers, provider)
	}
	sort.Slice(manifest.Providers, func(i, j int) bool { return manifest.Providers[i].Name < manifest.Providers[j].Name })
	return manifest, nil
}

func (s *Service) cacheServerProviderManifest(serverID string, manifest *models.Manifest) error {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	desired := make(map[string]bool, len(manifest.Providers))
	for _, provider := range manifest.Providers {
		desired[provider.Name] = true
		if err := s.store.Providers().Set(&models.ProviderRecord{ServerID: serverID, Definition: provider}); err != nil {
			return err
		}
	}
	existing, err := s.store.Providers().List(id)
	if err != nil {
		return err
	}
	for _, record := range existing {
		if desired[record.Definition.Name] {
			continue
		}
		if err := s.store.Providers().Delete(id, record.Definition.Name); err != nil && !storage.IsNotFound(err) {
			return err
		}
	}
	return nil
}

func (s *Service) removeCachedProvider(serverID uuid.UUID, name string) error {
	err := s.store.Providers().Delete(serverID, name)
	if storage.IsNotFound(err) {
		return nil
	}
	return err
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
	if err := s.cacheServerProviderManifest(serverID, manifest); err != nil {
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

func (s *Service) RefreshProviderManifest(serverID string) (*models.Manifest, error) {
	manifest, err := s.fetchProviderManifest(serverID)
	if err != nil {
		return nil, err
	}
	for _, provider := range manifest.Providers {
		s.providerRegistry.store(serverID, provider.Version, provider)
	}
	return manifest, nil
}

func (s *Service) refreshProviderManifest(serverID string) {
	if !s.providerRegistry.beginRefresh(serverID) {
		return
	}
	defer s.providerRegistry.endRefresh(serverID)
	_, _ = s.RefreshProviderManifest(serverID)
}

func (s *Service) waitForProvider(serverID, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		manifest, err := s.RefreshProviderManifest(serverID)
		if err == nil {
			for _, provider := range manifest.Providers {
				if provider.Name == name {
					return nil
				}
			}
			if discoveryErr := manifest.ProviderErrors[name]; len(discoveryErr) > 0 {
				lastErr = errors.New(discoveryErr)
			} else {
				lastErr = errors.New("provider not found in agent manifest: " + name)
			}
		} else {
			lastErr = err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errors.New("provider " + name + " did not appear in agent manifest after deployment: " + lastErr.Error())
		}
		if remaining > 250*time.Millisecond {
			remaining = 250 * time.Millisecond
		}
		time.Sleep(remaining)
	}
}

func (s *Service) waitForProviderRemoval(serverID, name string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		manifest, err := s.RefreshProviderManifest(serverID)
		if err == nil {
			registered := false
			for _, provider := range manifest.Providers {
				if provider.Name == name {
					registered = true
					break
				}
			}
			if !registered {
				return nil
			}
			lastErr = errors.New("provider is still present in agent manifest: " + name)
		} else {
			lastErr = err
		}
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return errors.New("provider " + name + " did not disappear from agent manifest after removal: " + lastErr.Error())
		}
		if remaining > 250*time.Millisecond {
			remaining = 250 * time.Millisecond
		}
		time.Sleep(remaining)
	}
}
