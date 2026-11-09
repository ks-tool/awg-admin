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
	"sync"

	"github.com/ks-tool/vpn-admin/internal/agentclient"
	"github.com/ks-tool/vpn-admin/models"

	"github.com/google/uuid"
)

type schemaCacheKey struct {
	agentID  string
	provider string
}

type schemaCacheEntry struct {
	version    string
	definition models.ProviderDefinition
}

type providerRegistry struct {
	mu      sync.RWMutex
	schemas map[schemaCacheKey]schemaCacheEntry
}

func newProviderRegistry() *providerRegistry {
	return &providerRegistry{schemas: make(map[schemaCacheKey]schemaCacheEntry)}
}

func (r *providerRegistry) cached(agentID, provider, version string) (models.ProviderDefinition, bool) {
	key := schemaCacheKey{agentID: agentID, provider: provider}
	r.mu.RLock()
	entry, ok := r.schemas[key]
	r.mu.RUnlock()
	if !ok || entry.version != version {
		return models.ProviderDefinition{}, false
	}
	return entry.definition, true
}

func (r *providerRegistry) store(agentID, version string, definition models.ProviderDefinition) {
	r.mu.Lock()
	r.schemas[schemaCacheKey{agentID: agentID, provider: definition.Name}] = schemaCacheEntry{version: version, definition: definition}
	r.mu.Unlock()
}

func (s *Service) GetProviderManifest(serverID string) (*models.Manifest, error) {
	var manifest *models.Manifest
	err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		manifest, err = client.Manifest(ctx)
		return err
	})
	return manifest, err
}

func (s *Service) GetProviderSchema(serverID, providerName string) (*models.ProviderDefinition, error) {
	manifest, err := s.GetProviderManifest(serverID)
	if err != nil {
		return nil, err
	}
	var provider *models.ProviderDefinition
	for index := range manifest.Providers {
		if manifest.Providers[index].Name == providerName {
			provider = &manifest.Providers[index]
			break
		}
	}
	if provider == nil {
		return nil, errors.New("provider not found in agent manifest")
	}
	if cached, ok := s.providerRegistry.cached(manifest.AgentID, providerName, provider.Version); ok && definitionsEqual(cached.Resources, provider.Resources) {
		return &cached, nil
	}
	var definition *models.ProviderDefinition
	err = s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		definition, err = client.ProviderSchema(ctx, providerName)
		return err
	})
	if err == nil {
		s.providerRegistry.store(manifest.AgentID, provider.Version, *definition)
	}
	return definition, err
}

func (s *Service) ListProviderResources(serverID, provider string) ([]models.Resource, error) {
	definition, err := s.GetProviderSchema(serverID, provider)
	if err != nil {
		return nil, err
	}
	supported := make(map[string]string)
	for _, item := range definition.Resources {
		supported[item.Kind] = item.APIVersion
	}
	var resources []models.Resource
	if err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		resources, err = client.ListResources(ctx, provider)
		return err
	}); err != nil {
		return nil, err
	}
	result := append([]models.Resource(nil), resources...)
	remote := make(map[string]struct{}, len(resources))
	for _, item := range resources {
		remote[item.Kind+"\x00"+item.Name] = struct{}{}
		if err := s.saveProviderResource(serverID, item); err != nil {
			return nil, err
		}
	}
	local, err := s.localProviderResources(serverID)
	if err != nil {
		return nil, err
	}
	for _, item := range local {
		apiVersion, ok := supported[item.Kind]
		if !ok {
			continue
		}
		if _, exists := remote[item.Kind+"\x00"+item.Name]; exists {
			continue
		}
		item.APIVersion = apiVersion
		var created *models.Resource
		if err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
			var err error
			created, err = client.PutResource(ctx, provider, item.Kind, item.Name, item)
			return err
		}); err != nil {
			return nil, err
		}
		if err := s.migrateProviderResourceSecret(serverID, definition, item); err != nil {
			return nil, err
		}
		if err := s.saveProviderResource(serverID, *created); err != nil {
			return nil, err
		}
		result = append(result, *created)
	}
	return result, nil
}

func (s *Service) PutProviderResource(serverID, provider, kind, name string, spec json.RawMessage) (*models.Resource, error) {
	if name == "" || !json.Valid(spec) {
		return nil, invalidInput("resource name and valid JSON spec are required")
	}
	definition, err := s.GetProviderSchema(serverID, provider)
	if err != nil {
		return nil, err
	}
	var resourceDefinition *models.ResourceDefinition
	for index := range definition.Resources {
		if definition.Resources[index].Kind == kind {
			resourceDefinition = &definition.Resources[index]
			break
		}
	}
	if resourceDefinition == nil {
		return nil, invalidInput("resource kind is not supported by provider")
	}
	desired := models.Resource{ServerID: serverID, APIVersion: resourceDefinition.APIVersion, Kind: kind, Name: name, Spec: spec}
	resources, err := s.ListProviderResources(serverID, provider)
	if err != nil {
		return nil, err
	}
	for _, existing := range resources {
		if existing.Kind == kind && existing.Name == name {
			desired.ID = existing.ID
			desired.Generation = existing.Generation
			break
		}
	}
	var result *models.Resource
	err = s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		result, err = client.PutResource(ctx, provider, kind, name, desired)
		return err
	})
	if err != nil {
		return nil, err
	}
	return result, s.saveProviderResource(serverID, *result)
}

func (s *Service) DeleteProviderResource(serverID, provider, kind, name string) error {
	if err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		return client.DeleteResource(ctx, provider, kind, name)
	}); err != nil {
		return err
	}
	local, err := s.localProviderResources(serverID)
	if err != nil {
		return err
	}
	for _, item := range local {
		if item.Kind == kind && item.Name == name {
			id, _ := uuid.Parse(serverID)
			return s.store.Resources(id).Delete(item.ID)
		}
	}
	return nil
}

func (s *Service) migrateProviderResourceSecret(serverID string, definition *models.ProviderDefinition, item models.Resource) error {
	pending := false
	for _, condition := range item.Status.Conditions {
		if condition.Type == "Secrets" && condition.Reason == "SecretMigrationPending" {
			pending = true
			break
		}
	}
	if !pending {
		return nil
	}
	actionAvailable := false
	for _, resourceDefinition := range definition.Resources {
		if resourceDefinition.Kind != item.Kind {
			continue
		}
		for _, action := range resourceDefinition.Actions {
			if action.Name == "importSecret" {
				actionAvailable = true
			}
		}
	}
	if !actionAvailable {
		return nil
	}
	var spec struct {
		UserRef   string `json:"userRef"`
		PublicKey string `json:"publicKey"`
	}
	if err := json.Unmarshal(item.Spec, &spec); err != nil {
		return err
	}
	userID, err := uuid.Parse(spec.UserRef)
	if err != nil {
		return nil
	}
	user, err := s.store.Users().Get(userID)
	if err != nil {
		return err
	}
	for _, peer := range user.Peers {
		publicKey := peer.PrivateKey.PublicKey()
		if publicKey.String() != spec.PublicKey {
			continue
		}
		arguments, err := json.Marshal(map[string]string{"privateKey": peer.PrivateKey.String()})
		if err != nil {
			return err
		}
		_, err = s.RunProviderResourceAction(serverID, definition.Name, item.Kind, item.Name, "importSecret", arguments)
		return err
	}
	return nil
}

func (s *Service) localProviderResources(serverID string) ([]models.Resource, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, invalidInput("invalid server id")
	}
	return s.store.Resources(id).List()
}

func (s *Service) saveProviderResource(serverID string, resource models.Resource) error {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return invalidInput("invalid server id")
	}
	resource.ServerID = serverID
	return s.store.Resources(id).Set(&resource)
}

func (s *Service) GetProviderResourceStatus(serverID, provider, kind, name string) (*models.ResourceStatus, error) {
	var status *models.ResourceStatus
	err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		status, err = client.ResourceStatus(ctx, provider, kind, name)
		return err
	})
	return status, err
}

func (s *Service) RunProviderResourceAction(serverID, provider, kind, name, action string, arguments json.RawMessage) (*models.ActionResult, error) {
	if len(arguments) == 0 {
		arguments = json.RawMessage(`{}`)
	}
	if !json.Valid(arguments) {
		return nil, invalidInput("action arguments must be valid JSON")
	}
	var result *models.ActionResult
	err := s.withAgentOnce(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		result, err = client.RunResourceAction(ctx, provider, kind, name, action, arguments)
		return err
	})
	return result, err
}

func (s *Service) withAgent(serverID string, operation func(context.Context, *agentclient.Client) error) error {
	server, err := s.agentServer(serverID)
	if err != nil {
		return err
	}
	return s.callAgent(server, operation)
}

func (s *Service) withAgentOnce(serverID string, operation func(context.Context, *agentclient.Client) error) error {
	server, err := s.agentServer(serverID)
	if err != nil {
		return err
	}
	client, _, err := s.agentClientFor(server)
	if err != nil {
		return err
	}
	return callOnce(client, pushTimeout, operation)
}

func (s *Service) agentServer(serverID string) (*models.Server, error) {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return nil, invalidInput("invalid server id")
	}
	return s.store.Servers().Get(id)
}

func definitionsEqual(left, right []models.ResourceDefinition) bool {
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	return string(leftJSON) == string(rightJSON)
}
