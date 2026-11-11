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
	"strings"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
)

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

func (s *Service) CreateRegistrySource(name, address, username, password string, plainHTTP bool) (*models.RegistrySource, error) {
	address, err := normalizeRegistryAddress(address)
	if err != nil {
		return nil, err
	}
	if err := validateRegistryCredentials(username, password); err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(name)) == 0 {
		return nil, fmt.Errorf("registry source name is required")
	}
	source := &models.RegistrySource{
		ID: uuid.New(), Name: strings.TrimSpace(name), URL: address,
		Username: strings.TrimSpace(username), Password: password, PlainHTTP: plainHTTP,
	}
	if err := s.store.RegistrySources().Set(source); err != nil {
		return nil, err
	}
	return publicRegistrySource(source), nil
}

func (s *Service) UpdateRegistrySource(id, name, address, username, password string, plainHTTP bool) (*models.RegistrySource, error) {
	sourceID, err := uuid.Parse(id)
	if err != nil {
		return nil, err
	}
	existing, err := s.store.RegistrySources().Get(sourceID)
	if err != nil {
		return nil, err
	}
	address, err = normalizeRegistryAddress(address)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(name)) == 0 {
		return nil, fmt.Errorf("registry source name is required")
	}
	username = strings.TrimSpace(username)
	if len(password) == 0 && len(username) > 0 {
		password = existing.Password
	}
	if err := validateRegistryCredentials(username, password); err != nil {
		return nil, err
	}
	updated := &models.RegistrySource{
		ID: sourceID, Name: strings.TrimSpace(name), URL: address,
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

func normalizeRegistryAddress(address string) (string, error) {
	address = strings.TrimSpace(address)
	address = strings.TrimPrefix(address, "https://")
	address = strings.TrimPrefix(address, "http://")
	address = strings.TrimPrefix(address, "oci://")
	address = strings.TrimSuffix(address, "/")
	if len(address) == 0 || strings.ContainsAny(address, "/@?# \t\r\n") {
		return "", fmt.Errorf("registry address must be a host with an optional port, without a repository path")
	}
	return "oci://" + address, nil
}

func validateRegistryCredentials(username, password string) error {
	if (len(username) == 0) != (len(password) == 0) {
		return fmt.Errorf("registry username and password must be provided together")
	}
	return nil
}
