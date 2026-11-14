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
	"errors"
	"fmt"
	"maps"
	"net"
	"strconv"
	"strings"
	"uuid"

	awgmodels "github.com/ks-tool/vpn-agent/v2/providers/awg/models"

	"github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage"
)

func providerFromAPIVersion(apiVersion string) string {
	namespace, _, _ := strings.Cut(apiVersion, "/")
	name, _, _ := strings.Cut(namespace, ".")
	return name
}

func (s *Service) providerInterface(serverID uuid.UUID, interfaceID uuid.UUID) (*models.Resource, error) {
	resources, err := s.localProviderResources(serverID.String())
	if err != nil {
		return nil, err
	}
	for index := range resources {
		if resources[index].Kind == "Interface" && resources[index].ID == interfaceID.String() {
			return &resources[index], nil
		}
	}
	return nil, storage.ErrNotFound
}

func (s *Service) addProviderPeer(userID uuid.UUID, in AddPeerInput, server *models.Server, interfaceResource *models.Resource) (*models.User, error) {
	provider := providerFromAPIVersion(interfaceResource.APIVersion)
	identifier, err := awgmodels.GenerateKey()
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if len(name) == 0 {
		name = "peer"
	}
	resourceName := fmt.Sprintf("%s-%s-%s", name, provider, models.NewDeterministicUUID(userID.String() + ":" + in.InterfaceID.String() + ":" + identifier.String()).String()[:8])
	spec := make(map[string]any, len(in.Spec)+5)
	maps.Copy(spec, in.Spec)
	user, err := s.store.Users().Get(userID)
	if err != nil {
		return nil, err
	}
	spec["name"] = name
	spec["interfaceRef"] = in.InterfaceID.String()
	spec["userRef"] = userID.String()
	spec["publicKey"] = identifier.String()
	spec["disabled"] = user.Disabled
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}
	created, err := s.PutProviderResource(server.ID.String(), provider, "Peer", resourceName, raw)
	if err != nil {
		return nil, err
	}
	if err := s.waitProviderResourcesReady(server.ID.String(), provider, []models.Resource{*created}); err != nil {
		_ = s.DeleteProviderResource(server.ID.String(), provider, "Peer", resourceName)
		return nil, err
	}
	peer := &models.Peer{
		Name: name, PrivateKey: identifier, InterfaceId: in.InterfaceID, Provider: provider, ResourceName: resourceName, Disabled: user.Disabled,
	}
	if err := s.store.Users().Peers(userID).Set(peer); err != nil {
		_ = s.DeleteProviderResource(server.ID.String(), provider, "Peer", resourceName)
		return nil, err
	}
	user, err = s.store.Users().Get(userID)
	if err != nil {
		return nil, err
	}
	sanitized := sanitizeUser(*user)
	return &sanitized, nil
}

func (s *Service) deleteProviderPeer(userID uuid.UUID, peer *models.Peer) (*models.User, error) {
	server, err := s.findServerByInterface(peer.InterfaceId)
	if err != nil {
		if len(peer.Provider) == 0 && len(peer.ResourceName) == 0 {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	provider, resourceName := peer.Provider, peer.ResourceName
	if len(provider) == 0 || len(resourceName) == 0 {
		resource := s.findProviderPeer(server.ID, peer.InterfaceId, peerPublicKey(peer))
		if resource == nil {
			return nil, storage.ErrNotFound
		}
		provider, resourceName = providerFromAPIVersion(resource.APIVersion), resource.Name
	}
	if err := s.DeleteProviderResource(server.ID.String(), provider, "Peer", resourceName); err != nil {
		return nil, err
	}
	user, err := s.store.Users().Get(userID)
	if err != nil {
		return nil, err
	}
	sanitized := sanitizeUser(*user)
	return &sanitized, nil
}

func (s *Service) setProviderPeerDisabled(userID uuid.UUID, peer *models.Peer, disabled bool) (*models.User, error) {
	server, err := s.findServerByInterface(peer.InterfaceId)
	if err != nil {
		return nil, err
	}
	resources, err := s.localProviderResources(server.ID.String())
	if err != nil {
		return nil, err
	}
	for _, item := range resources {
		if item.Kind != "Peer" || item.Name != peer.ResourceName || providerFromAPIVersion(item.APIVersion) != peer.Provider {
			continue
		}
		var spec map[string]any
		if err := json.Unmarshal(item.Spec, &spec); err != nil {
			return nil, err
		}
		spec["disabled"] = disabled
		raw, err := json.Marshal(spec)
		if err != nil {
			return nil, err
		}
		if _, err := s.PutProviderResource(server.ID.String(), peer.Provider, "Peer", item.Name, raw); err != nil {
			return nil, err
		}
		peer.Disabled = disabled
		if err := s.store.Users().Peers(userID).Set(peer); err != nil {
			return nil, err
		}
		user, err := s.store.Users().Get(userID)
		if err != nil {
			return nil, err
		}
		sanitized := sanitizeUser(*user)
		return &sanitized, nil
	}
	return nil, errors.New("provider peer resource was not found")
}

func (s *Service) renderProviderPeerConfig(peer *models.Peer) (string, error) {
	server, err := s.findServerByInterface(peer.InterfaceId)
	if err != nil {
		return "", err
	}
	iface, err := s.store.Servers().Interfaces(server.ID).Get(peer.InterfaceId)
	if err != nil {
		return "", err
	}
	if iface.ListenPort == 0 {
		return "", errors.New("provider interface has no public listen port")
	}
	arguments, _ := json.Marshal(map[string]string{"endpoint": net.JoinHostPort(server.SSH.Host, strconv.Itoa(int(iface.ListenPort)))})
	result, err := s.RunProviderResourceAction(server.ID.String(), peer.Provider, "Peer", peer.ResourceName, "renderClientConfig", arguments)
	if err != nil {
		return "", err
	}
	var config string
	if err := json.Unmarshal(result.Data, &config); err != nil {
		return "", err
	}
	return config, nil
}
