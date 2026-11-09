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
	"time"

	"github.com/ks-tool/vpn-admin/internal/agentclient"
	"github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage"

	"github.com/google/uuid"
)

type resolvedEndpoint struct {
	provider   string
	definition models.ResourceDefinition
	port       models.PortDefinition
	endpoint   models.LinkEndpoint
}

func (s *Service) ListProviderLinks() ([]models.Link, error) {
	return s.store.Links().List()
}

func (s *Service) CreateProviderLink(sourceID, targetID string) (*models.Link, error) {
	source, err := s.resolveEndpoint(sourceID)
	if err != nil {
		return nil, err
	}
	target, err := s.resolveEndpoint(targetID)
	if err != nil {
		return nil, err
	}
	kind, err := compatibleLinkKind(source, target)
	if err != nil {
		return nil, err
	}
	existing, err := s.store.Links().List()
	if err != nil {
		return nil, err
	}
	if hasLinkCycle(existing, source.endpoint.ResourceID, target.endpoint.ResourceID) {
		return nil, invalidInput("link would create a cycle")
	}
	link := &models.Link{
		ID: uuid.NewString(), Kind: kind, Generation: 1,
		Source: source.endpoint, Target: target.endpoint, Spec: json.RawMessage(`{}`),
		Status: models.LinkStatus{Phase: models.LinkPreparing},
	}
	if err := s.store.Links().Set(link); err != nil {
		return nil, err
	}
	return link, s.coordinateLink(link, source, target)
}

func (s *Service) ApplyProviderLink(id string) (*models.Link, error) {
	link, err := s.store.Links().Get(id)
	if err != nil {
		return nil, err
	}
	source, err := s.resolveEndpoint(link.Source.ResourceID)
	if err != nil {
		return nil, err
	}
	target, err := s.resolveEndpoint(link.Target.ResourceID)
	if err != nil {
		return nil, err
	}
	link.Generation++
	link.Status.Phase = models.LinkPreparing
	if err := s.store.Links().Set(link); err != nil {
		return nil, err
	}
	return link, s.coordinateLink(link, source, target)
}

func (s *Service) DeleteProviderLink(id string) error {
	link, err := s.store.Links().Get(id)
	if err != nil {
		if storage.IsNotFound(err) {
			return nil
		}
		return err
	}
	source, err := s.resolveEndpoint(link.Source.ResourceID)
	if err != nil {
		return err
	}
	target, err := s.resolveEndpoint(link.Target.ResourceID)
	if err != nil {
		return err
	}
	var sourceOffer, targetOffer *models.LinkOffer
	if err := s.withAgent(link.Source.ServerID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		sourceOffer, err = client.PrepareLink(ctx, source.provider, models.LinkRequest{Link: *link, Endpoint: source.endpoint})
		return err
	}); err != nil {
		return s.failLink(link, err)
	}
	if err := s.withAgent(link.Target.ServerID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		targetOffer, err = client.PrepareLink(ctx, target.provider, models.LinkRequest{Link: *link, Endpoint: target.endpoint})
		return err
	}); err != nil {
		return s.failLink(link, err)
	}
	var deleteErrors []error
	if err := s.withAgent(link.Source.ServerID, func(ctx context.Context, client *agentclient.Client) error {
		return client.DeleteLink(ctx, source.provider, models.LinkRequest{Link: *link, Endpoint: source.endpoint, RemoteOffer: targetOffer})
	}); err != nil {
		deleteErrors = append(deleteErrors, err)
	}
	if err := s.withAgent(link.Target.ServerID, func(ctx context.Context, client *agentclient.Client) error {
		return client.DeleteLink(ctx, target.provider, models.LinkRequest{Link: *link, Endpoint: target.endpoint, RemoteOffer: sourceOffer})
	}); err != nil {
		deleteErrors = append(deleteErrors, err)
	}
	if len(deleteErrors) > 0 {
		err := errors.Join(deleteErrors...)
		_ = s.failLink(link, err)
		return err
	}
	return s.store.Links().Delete(id)
}

func (s *Service) coordinateLink(link *models.Link, source, target resolvedEndpoint) error {
	var sourceOffer, targetOffer *models.LinkOffer
	if err := s.withAgent(link.Source.ServerID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		sourceOffer, err = client.PrepareLink(ctx, source.provider, models.LinkRequest{Link: *link, Endpoint: source.endpoint})
		return err
	}); err != nil {
		return s.failLink(link, err)
	}
	if err := s.withAgent(link.Target.ServerID, func(ctx context.Context, client *agentclient.Client) error {
		var err error
		targetOffer, err = client.PrepareLink(ctx, target.provider, models.LinkRequest{Link: *link, Endpoint: target.endpoint})
		return err
	}); err != nil {
		return s.failLink(link, err)
	}
	if err := s.withAgent(link.Source.ServerID, func(ctx context.Context, client *agentclient.Client) error {
		return client.CommitLink(ctx, source.provider, models.LinkRequest{Link: *link, Endpoint: source.endpoint, RemoteOffer: targetOffer})
	}); err != nil {
		return s.failLink(link, err)
	}
	if err := s.withAgent(link.Target.ServerID, func(ctx context.Context, client *agentclient.Client) error {
		return client.CommitLink(ctx, target.provider, models.LinkRequest{Link: *link, Endpoint: target.endpoint, RemoteOffer: sourceOffer})
	}); err != nil {
		return s.failLink(link, err)
	}
	link.Status = models.LinkStatus{
		ObservedGeneration: link.Generation, Phase: models.LinkReady,
		Conditions: []models.Condition{{Type: "Ready", Status: models.ConditionTrue, Reason: "Reconciled", Message: "both link endpoints are committed", ObservedGeneration: link.Generation, LastTransitionTime: time.Now().UTC()}},
	}
	return s.store.Links().Set(link)
}

func (s *Service) failLink(link *models.Link, cause error) error {
	link.Status.Phase = models.LinkDegraded
	link.Status.Conditions = []models.Condition{{Type: "Ready", Status: models.ConditionFalse, Reason: "ApplyFailed", Message: cause.Error(), LastTransitionTime: time.Now().UTC()}}
	if err := s.store.Links().Set(link); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (s *Service) resolveEndpoint(resourceID string) (resolvedEndpoint, error) {
	servers, err := s.store.Servers().List()
	if err != nil {
		return resolvedEndpoint{}, err
	}
	for _, server := range servers {
		resource, err := s.store.Resources(server.ID).Get(resourceID)
		if err != nil {
			continue
		}
		manifest, err := s.GetProviderManifest(server.ID.String())
		if err != nil {
			return resolvedEndpoint{}, err
		}
		for _, provider := range manifest.Providers {
			for _, definition := range provider.Resources {
				if definition.APIVersion != resource.APIVersion || definition.Kind != resource.Kind {
					continue
				}
				for _, port := range definition.Ports {
					if len(port.LinkKinds) > 0 {
						return resolvedEndpoint{
							provider: provider.Name, definition: definition, port: port,
							endpoint: models.LinkEndpoint{ServerID: server.ID.String(), ResourceID: resource.ID, APIVersion: resource.APIVersion, Kind: resource.Kind, Name: resource.Name, Port: port.Name, Address: server.SSH.Host},
						}, nil
					}
				}
			}
		}
		return resolvedEndpoint{}, errors.New("resource does not expose link ports")
	}
	return resolvedEndpoint{}, storage.ErrNotFound
}

func compatibleLinkKind(source, target resolvedEndpoint) (string, error) {
	for _, kind := range source.port.LinkKinds {
		for _, targetKind := range target.port.LinkKinds {
			if kind != targetKind {
				continue
			}
			if len(source.port.RemotePorts) == 0 || len(target.port.RemotePorts) == 0 || containsString(source.port.RemotePorts, target.port.Name) || containsString(target.port.RemotePorts, source.port.Name) {
				return kind, nil
			}
		}
	}
	return "", invalidInput("resource ports are not compatible")
}

func containsString(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func hasLinkCycle(links []models.Link, sourceID, targetID string) bool {
	if sourceID == targetID {
		return true
	}
	for _, link := range links {
		if link.Source.ResourceID == sourceID && link.Target.ResourceID == targetID {
			return true
		}
	}
	adjacency := make(map[string][]string)
	for _, link := range links {
		adjacency[link.Source.ResourceID] = append(adjacency[link.Source.ResourceID], link.Target.ResourceID)
	}
	visited := make(map[string]bool)
	pending := []string{targetID}
	for len(pending) > 0 {
		current := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if current == sourceID {
			return true
		}
		if visited[current] {
			continue
		}
		visited[current] = true
		pending = append(pending, adjacency[current]...)
	}
	return false
}
