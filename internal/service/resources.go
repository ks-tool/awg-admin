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
	"fmt"
	"os"
	"sort"
	"sync"
	"time"
	"uuid"

	"github.com/ks-tool/vpn-admin/internal/agentclient"
	"github.com/ks-tool/vpn-admin/models"
)

var ErrProviderNotFound = errors.New("provider not found in agent manifest")

type schemaCacheKey struct {
	serverID string
	provider string
}

type schemaCacheEntry struct {
	version    string
	definition models.ProviderDefinition
}

type providerRegistry struct {
	mu         sync.RWMutex
	schemas    map[schemaCacheKey]schemaCacheEntry
	refreshing map[string]bool
}

func newProviderRegistry() *providerRegistry {
	return &providerRegistry{schemas: make(map[schemaCacheKey]schemaCacheEntry), refreshing: make(map[string]bool)}
}

func (r *providerRegistry) cached(serverID, provider, version string) (models.ProviderDefinition, bool) {
	key := schemaCacheKey{serverID: serverID, provider: provider}
	r.mu.RLock()
	entry, ok := r.schemas[key]
	r.mu.RUnlock()
	if !ok || entry.version != version {
		return models.ProviderDefinition{}, false
	}
	return entry.definition, true
}

func (r *providerRegistry) store(serverID, version string, definition models.ProviderDefinition) {
	r.mu.Lock()
	r.schemas[schemaCacheKey{serverID: serverID, provider: definition.Name}] = schemaCacheEntry{version: version, definition: definition}
	r.mu.Unlock()
}

func (s *Service) GetProviderManifest(serverID string) (*models.Manifest, error) {
	manifest, err := s.cachedProviderManifest(serverID)
	if err == nil {
		s.providerRefresh.Go(func() {
			s.refreshProviderManifest(serverID)
		})
		return manifest, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	return s.fetchProviderManifest(serverID)
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
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, providerName)
	}
	if len(provider.Resources) > 0 {
		return provider, nil
	}
	if cached, ok := s.providerRegistry.cached(serverID, providerName, provider.Version); ok && definitionsEqual(cached.Resources, provider.Resources) {
		return &cached, nil
	}
	var definition *models.ProviderDefinition
	err = s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		definition, err = client.ProviderSchema(ctx, providerName)
		return err
	})
	if err == nil {
		s.providerRegistry.store(serverID, provider.Version, *definition)
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
	local, err := s.localProviderResources(serverID)
	if err != nil {
		return nil, err
	}
	localByName := make(map[string]models.Resource, len(local))
	for _, item := range local {
		if providerFromAPIVersion(item.APIVersion) == provider {
			localByName[item.Kind+"\x00"+item.Name] = item
		}
	}
	var resources []models.Resource
	if err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		resources, err = client.ListResources(ctx, provider)
		return err
	}); err != nil {
		return nil, err
	}
	result := make([]models.Resource, 0, len(resources))
	remote := make(map[string]struct{}, len(resources))
	for _, item := range resources {
		key := item.Kind + "\x00" + item.Name
		remote[key] = struct{}{}
		if apiVersion, ok := supported[item.Kind]; ok {
			item.APIVersion = apiVersion
		}
		if localItem, ok := localByName[key]; ok {
			if err := s.migrateProviderResourceSecret(serverID, definition, localItem); err != nil {
				return nil, err
			}
		}
		if err := s.saveProviderResource(serverID, item); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	sort.SliceStable(local, func(i, j int) bool {
		if local[i].Kind == local[j].Kind {
			return local[i].Name < local[j].Name
		}
		if local[i].Kind == "Interface" {
			return true
		}
		if local[j].Kind == "Interface" {
			return false
		}
		return local[i].Kind == "Peer"
	})
	for _, item := range local {
		if providerFromAPIVersion(item.APIVersion) != provider {
			continue
		}
		apiVersion, ok := supported[item.Kind]
		if !ok {
			continue
		}
		if _, exists := remote[item.Kind+"\x00"+item.Name]; exists {
			continue
		}
		if item.Status.Phase == models.ResourceDeleting {
			if err := s.removeLegacyResource(serverID, item); err != nil {
				return nil, err
			}
			id, _ := uuid.Parse(serverID)
			if err := s.store.Resources(id).Delete(item.ID); err != nil {
				return nil, err
			}
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
		if err := s.restoreProviderResourceSecret(serverID, definition, item); err != nil {
			return nil, err
		}
		created.APIVersion = apiVersion
		if err := s.saveProviderResource(serverID, *created); err != nil {
			return nil, err
		}
		result = append(result, *created)
	}
	return result, nil
}

func (s *Service) completeServerMigration(server *models.Server) error {
	if !server.Unmanaged {
		return nil
	}
	resources, err := s.ListProviderResources(server.ID.String(), "awg")
	if err != nil {
		return err
	}
	if err := s.waitProviderResourcesReady(server.ID.String(), "awg", resources); err != nil {
		return err
	}
	links, err := s.store.Links().List()
	if err != nil {
		return err
	}
	for _, link := range links {
		if link.Source.ServerID != server.ID.String() && link.Target.ServerID != server.ID.String() {
			continue
		}
		if link.Status.Phase == models.LinkReady && link.Status.ObservedGeneration == link.Generation {
			continue
		}
		if _, err := s.ApplyProviderLink(link.ID); err != nil {
			return err
		}
	}
	server.Unmanaged = false
	if err := s.store.Servers().Set(server); err != nil {
		server.Unmanaged = true
		return err
	}
	return nil
}

func (s *Service) waitProviderResourcesReady(serverID, provider string, resources []models.Resource) error {
	deadline := time.Now().Add(pushTimeout)
	for _, resource := range resources {
		for {
			var status *models.ResourceStatus
			if err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
				var err error
				status, err = client.ResourceStatus(ctx, provider, resource.Kind, resource.Name)
				return err
			}); err != nil {
				return err
			}
			if status.ObservedGeneration >= resource.Generation {
				if status.Phase == models.ResourceReady {
					break
				}
				if status.Phase == models.ResourceDegraded {
					if len(status.Conditions) > 0 {
						return errors.New(status.Conditions[0].Message)
					}
					return errors.New("provider resource reconciliation failed")
				}
			}
			if time.Now().After(deadline) {
				return errors.New("provider resource reconciliation timed out")
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return nil
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
	local, err := s.localProviderResources(serverID)
	if err != nil {
		return err
	}
	var selected *models.Resource
	for index := range local {
		if local[index].Kind == kind && local[index].Name == name && providerFromAPIVersion(local[index].APIVersion) == provider {
			selected = &local[index]
			break
		}
	}
	if selected != nil && kind == "Interface" {
		links, err := s.store.Links().List()
		if err != nil {
			return err
		}
		for _, link := range links {
			if link.Source.ResourceID == selected.ID || link.Target.ResourceID == selected.ID {
				return invalidInput("remove provider links attached to interface %q first", name)
			}
		}
		for _, item := range local {
			if item.Kind != "Peer" {
				continue
			}
			var spec struct {
				InterfaceRef string `json:"interfaceRef"`
			}
			if err := json.Unmarshal(item.Spec, &spec); err != nil {
				return err
			}
			if spec.InterfaceRef == selected.ID {
				return invalidInput("remove peers attached to interface %q first", name)
			}
		}
	}
	if selected != nil {
		selected.Status.Phase = models.ResourceDeleting
		if err := s.saveProviderResource(serverID, *selected); err != nil {
			return err
		}
	}
	if err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
		return client.DeleteResource(ctx, provider, kind, name)
	}); err != nil {
		return err
	}

	deadline := time.Now().Add(pushTimeout)
	for {
		remaining := false
		var remoteStatus models.ResourceStatus
		if err := s.withAgent(serverID, func(ctx context.Context, client *agentclient.Client) error {
			resources, err := client.ListResources(ctx, provider)
			if err != nil {
				return err
			}
			for _, item := range resources {
				if item.Kind == kind && item.Name == name {
					remaining = true
					remoteStatus = item.Status
					break
				}
			}
			return nil
		}); err != nil {
			return err
		}
		if !remaining {
			break
		}
		if remoteStatus.Phase == models.ResourceDegraded {
			if len(remoteStatus.Conditions) > 0 {
				return errors.New(remoteStatus.Conditions[0].Message)
			}
			return errors.New("provider resource deletion failed")
		}
		if time.Now().After(deadline) {
			return errors.New("provider resource deletion timed out")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if selected != nil {
		if err := s.removeLegacyResource(serverID, *selected); err != nil {
			return err
		}
		id, _ := uuid.Parse(serverID)
		return s.store.Resources(id).Delete(selected.ID)
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
	return s.restoreProviderResourceSecret(serverID, definition, item)
}

func (s *Service) restoreProviderResourceSecret(serverID string, definition *models.ProviderDefinition, item models.Resource) error {
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
	if item.Kind == "Interface" {
		serverUUID, err := uuid.Parse(serverID)
		if err != nil {
			return err
		}
		interfaceID, err := uuid.Parse(item.ID)
		if err != nil {
			return nil
		}
		iface, err := s.store.Servers().Interfaces(serverUUID).Get(interfaceID)
		if err != nil {
			return err
		}
		arguments, err := json.Marshal(map[string]any{
			"privateKey": iface.PrivateKey.String(), "preUp": iface.PreUp, "postUp": iface.PostUp,
			"preDown": iface.PreDown, "postDown": iface.PostDown,
		})
		if err != nil {
			return err
		}
		_, err = s.RunProviderResourceAction(serverID, definition.Name, item.Kind, item.Name, "importSecret", arguments)
		return err
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
		arguments := map[string]string{"privateKey": peer.PrivateKey.String()}
		if peer.InterfaceId != uuid.Nil() {
			iface, _, err := s.getInterfaceByID(peer.InterfaceId)
			if err != nil {
				return err
			}
			for _, configured := range iface.Peers {
				if configured.Key.String() == spec.PublicKey && configured.PresharedKey != nil {
					arguments["presharedKey"] = configured.PresharedKey.String()
					break
				}
			}
		}
		raw, err := json.Marshal(arguments)
		if err != nil {
			return err
		}
		_, err = s.RunProviderResourceAction(serverID, definition.Name, item.Kind, item.Name, "importSecret", raw)
		return err
	}
	return errors.New("legacy peer secret was not found")
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
	if err := s.syncLegacyResource(serverID, resource); err != nil {
		return err
	}
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
	if err != nil || action != "rotateKeys" {
		return result, err
	}
	resources, listErr := s.localProviderResources(serverID)
	if listErr != nil {
		return result, listErr
	}
	for _, item := range resources {
		if item.Kind == kind && item.Name == name {
			return result, s.removeLegacyResource(serverID, item)
		}
	}
	return result, nil
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
