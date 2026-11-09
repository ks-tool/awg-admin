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

package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
)

type Config struct {
	ProtocolVersion string                        `json:"protocolVersion"`
	Providers       []models.ProviderRegistration `json:"providers"`
}

func RegistrationsDir(dir, legacyFile string) ([]models.ProviderRegistration, error) {
	registrations, err := Registrations(legacyFile)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return registrations, nil
	}
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		extension := filepath.Ext(entry.Name())
		if entry.IsDir() || (extension != ".json" && extension != ".conf") {
			continue
		}
		registered, err := Registrations(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		registrations = append(registrations, registered...)
	}
	return registrations, nil
}

func Registrations(path string) ([]models.ProviderRegistration, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var config Config
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}
	if config.ProtocolVersion != models.ProtocolVersion {
		return nil, errors.New("unsupported provider registry protocol version")
	}
	return config.Providers, nil
}

func LoadDir(dir, legacyFile string, client *http.Client) ([]resource.Provider, error) {
	registrations, err := RegistrationsDir(dir, legacyFile)
	if err != nil {
		return nil, err
	}
	return loadRegistrations(registrations, client)
}

func Load(path string, client *http.Client) ([]resource.Provider, error) {
	registrations, err := Registrations(path)
	if err != nil {
		return nil, err
	}
	return loadRegistrations(registrations, client)
}

func loadRegistrations(registrations []models.ProviderRegistration, client *http.Client) ([]resource.Provider, error) {
	providers := make([]resource.Provider, 0, len(registrations))
	for _, registration := range registrations {
		definition := registration.Provider
		var err error
		if registration.Discover {
			definition, err = discover(registration.Address, definition.Name, client)
			if err != nil {
				return nil, err
			}
		}
		provider, err := NewProvider(registration.Address, definition, client)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	return providers, nil
}

func discover(address, name string, client *http.Client) (models.ProviderDefinition, error) {
	var last error
	for attempt := 0; attempt < 25; attempt++ {
		definition, err := discoverOnce(address, name, client)
		if err == nil {
			return definition, nil
		}
		last = err
		if attempt < 24 {
			time.Sleep(200 * time.Millisecond)
		}
	}
	return models.ProviderDefinition{}, last
}

func discoverOnce(address, name string, client *http.Client) (models.ProviderDefinition, error) {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	response, err := client.Get(strings.TrimSuffix(address, "/") + "/v1/manifest")
	if err != nil {
		return models.ProviderDefinition{}, fmt.Errorf("discover remote provider %s: %w", name, err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return models.ProviderDefinition{}, fmt.Errorf("discover remote provider %s: %s", name, response.Status)
	}
	var manifest models.Manifest
	if err := json.NewDecoder(response.Body).Decode(&manifest); err != nil {
		return models.ProviderDefinition{}, err
	}
	if manifest.ProtocolVersion != models.ProtocolVersion {
		return models.ProviderDefinition{}, errors.New("unsupported remote provider protocol version")
	}
	for _, definition := range manifest.Providers {
		if definition.Name == name {
			return definition, nil
		}
	}
	return models.ProviderDefinition{}, fmt.Errorf("remote provider manifest does not contain %s", name)
}
