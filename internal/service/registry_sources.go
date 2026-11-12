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
	"fmt"
	"path/filepath"
	"strings"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
)

const officialAgentRepository = "oci://ghcr.io/ks-tool/vpn-agent"

func (s *Service) ListRegistrySources() ([]models.RegistrySource, error) {
	sources, err := s.store.RegistrySources().List()
	if err != nil {
		return nil, err
	}
	for i := range sources {
		scrubRegistrySource(&sources[i])
	}
	return sources, nil
}

func (s *Service) CreateRegistrySource(address, sourceType, username, password string, plainHTTP bool) (*models.RegistrySource, error) {
	address, err := normalizeRegistrySourceURL(address, sourceType)
	if err != nil {
		return nil, err
	}
	if err := validateRegistryCredentials(username, password); err != nil {
		return nil, err
	}
	source := &models.RegistrySource{
		ID: uuid.New(), URL: address, Type: sourceType,
		Username: strings.TrimSpace(username), Password: password, PlainHTTP: plainHTTP,
	}
	if err := s.store.RegistrySources().Set(source); err != nil {
		return nil, err
	}
	return publicRegistrySource(source), nil
}

func (s *Service) UpdateRegistrySource(id, address, sourceType, username, password string, plainHTTP bool) (*models.RegistrySource, error) {
	sourceID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	existing, err := s.store.RegistrySources().Get(sourceID)
	if err != nil {
		return nil, err
	}
	address, err = normalizeRegistrySourceURL(address, sourceType)
	if err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if len(password) == 0 && len(username) > 0 {
		password = existing.Password
	}
	if err := validateRegistryCredentials(username, password); err != nil {
		return nil, err
	}
	updated := &models.RegistrySource{
		ID: sourceID, URL: address, Type: sourceType,
		Username: username, Password: password, PlainHTTP: plainHTTP,
	}
	if err := s.store.RegistrySources().Set(updated); err != nil {
		return nil, err
	}
	return publicRegistrySource(updated), nil
}

func (s *Service) DeleteRegistrySource(id string) error {
	sourceID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	return s.store.RegistrySources().Delete(sourceID)
}

func (s *Service) resolveRegistry(registry models.Registry) (models.Registry, error) {
	if len(registry.SourceID) == 0 {
		return registry, validateRegistry(registry)
	}
	id, err := uuid.Parse(registry.SourceID)
	if err != nil {
		return models.Registry{}, fmt.Errorf("invalid registry source id: %w", err)
	}
	source, err := s.store.RegistrySources().Get(id)
	if err != nil {
		return models.Registry{}, err
	}
	if !strings.HasPrefix(source.URL, "oci://") {
		return models.Registry{}, fmt.Errorf("local source %q has no OCI registry credentials", source.URL)
	}
	return models.Registry{
		SourceID: registry.SourceID, Username: source.Username,
		Password: source.Password, PlainHTTP: source.PlainHTTP,
	}, nil
}

func publicRegistrySource(source *models.RegistrySource) *models.RegistrySource {
	out := *source
	scrubRegistrySource(&out)
	return &out
}

func scrubRegistrySource(source *models.RegistrySource) {
	source.HasPassword = len(source.Password) > 0
	source.Password = ""
}

func normalizeRegistrySourceURL(address, sourceType string) (string, error) {
	address = strings.TrimSpace(address)
	if sourceType != models.RegistrySourceAgent && sourceType != models.RegistrySourceProvider {
		return "", fmt.Errorf("source type must be %q or %q", models.RegistrySourceAgent, models.RegistrySourceProvider)
	}
	if strings.HasPrefix(address, "oci://") {
		ref, err := parseOCIRepository(address)
		if err != nil {
			return "", err
		}
		return ref.withoutTag(), nil
	}
	if !filepath.IsAbs(address) {
		return "", fmt.Errorf("source must be an OCI URL or an absolute local binary path")
	}
	return filepath.Clean(address), nil
}

func validateRegistryCredentials(username, password string) error {
	if (len(username) == 0) != (len(password) == 0) {
		return fmt.Errorf("registry username and password must be provided together")
	}
	return nil
}
