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
	"fmt"
	"time"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage"
	agentmodels "github.com/ks-tool/vpn-agent/models"
)

func legacyInterfaceSpec(iface *models.Interface) (json.RawMessage, error) {
	data, err := json.Marshal(iface.InterfaceConfig)
	if err != nil {
		return nil, err
	}
	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		return nil, err
	}
	for _, field := range []string{"pk", "peers", "preUp", "postUp", "preDown", "postDown"} {
		delete(spec, field)
	}
	return json.Marshal(spec)
}

func legacyPeerSpec(user *models.User, peer *models.Peer, iface *models.Interface) (json.RawMessage, agentmodels.InterfacePeer, error) {
	publicKey := peerPublicKey(peer)
	var configured agentmodels.InterfacePeer
	for _, candidate := range iface.Peers {
		if candidate.Key.String() == publicKey {
			configured = candidate
			break
		}
	}
	spec, err := json.Marshal(map[string]any{
		"name": peer.Name, "interfaceRef": iface.ID.String(), "userRef": user.ID.String(), "publicKey": publicKey,
		"allowedIPs": configured.AllowedIPs, "dns": peer.DNS, "disabled": peer.Disabled || user.Disabled,
		"keepaliveSeconds": int(configured.KeepaliveInterval.Seconds()),
	})
	return spec, configured, err
}

func (s *Service) pushProviderInterface(serverID uuid.UUID, iface *models.Interface) (bool, error) {
	local, err := s.store.Resources(serverID).Get(iface.ID.String())
	if err != nil && !storage.IsNotFound(err) {
		return true, err
	}
	if storage.IsNotFound(err) {
		if _, schemaErr := s.GetProviderSchema(serverID.String(), "awg"); schemaErr != nil {
			return false, nil
		}
		spec, specErr := legacyInterfaceSpec(iface)
		if specErr != nil {
			return true, specErr
		}
		local = &models.Resource{
			ID: iface.ID.String(), ServerID: serverID.String(), APIVersion: "awg/v1alpha1", Kind: "Interface", Name: iface.Interface,
			Generation: 1, Spec: spec,
			Status: models.ResourceStatus{Phase: models.ResourcePending, Conditions: []models.Condition{{
				Type: "Secrets", Status: models.ConditionFalse, Reason: "SecretMigrationPending", LastTransitionTime: time.Now().UTC(),
			}}},
		}
		if err := s.store.Resources(serverID).Set(local); err != nil {
			return true, err
		}
	}

	spec, err := legacyInterfaceSpec(iface)
	if err != nil {
		return true, err
	}
	if _, err := s.PutProviderResource(serverID.String(), "awg", "Interface", local.Name, spec); err != nil {
		return true, err
	}
	users, err := s.store.Users().List()
	if err != nil {
		return true, err
	}
	expected := make(map[string]struct{})
	for userIndex := range users {
		user := &users[userIndex]
		for peerIndex := range user.Peers {
			peer := &user.Peers[peerIndex]
			if peer.InterfaceId != iface.ID {
				continue
			}
			spec, configured, err := legacyPeerSpec(user, peer, iface)
			if err != nil {
				return true, err
			}
			publicKey := peerPublicKey(peer)
			expected[publicKey] = struct{}{}
			name := fmt.Sprintf("%s-%s-%s", user.Name, peer.Name, models.NewDeterministicUUID(user.ID.String() + ":" + iface.ID.String() + ":" + publicKey).String()[:8])
			existing := s.findProviderPeer(serverID, iface.ID, publicKey)
			if _, err := s.PutProviderResource(serverID.String(), "awg", "Peer", name, spec); err != nil {
				return true, err
			}
			if existing == nil {
				if err := s.importLegacyPeerSecret(serverID.String(), name, peer, configured); err != nil {
					return true, err
				}
			}
		}
	}
	localResources, err := s.localProviderResources(serverID.String())
	if err != nil {
		return true, err
	}
	for _, item := range localResources {
		if item.Kind != "Peer" {
			continue
		}
		var spec struct {
			InterfaceRef string `json:"interfaceRef"`
			PublicKey    string `json:"publicKey"`
		}
		if err := json.Unmarshal(item.Spec, &spec); err != nil {
			return true, err
		}
		if spec.InterfaceRef != iface.ID.String() {
			continue
		}
		if _, ok := expected[spec.PublicKey]; !ok {
			if err := s.DeleteProviderResource(serverID.String(), "awg", item.Kind, item.Name); err != nil {
				return true, err
			}
		}
	}
	return true, nil
}

func (s *Service) findProviderPeer(serverID uuid.UUID, interfaceID uuid.UUID, publicKey string) *models.Resource {
	resources, err := s.localProviderResources(serverID.String())
	if err != nil {
		return nil
	}
	for index := range resources {
		item := &resources[index]
		if item.Kind != "Peer" {
			continue
		}
		var spec struct {
			InterfaceRef string `json:"interfaceRef"`
			PublicKey    string `json:"publicKey"`
		}
		if json.Unmarshal(item.Spec, &spec) == nil && spec.InterfaceRef == interfaceID.String() && spec.PublicKey == publicKey {
			return item
		}
	}
	return nil
}

func (s *Service) importLegacyPeerSecret(serverID, name string, peer *models.Peer, configured agentmodels.InterfacePeer) error {
	arguments := map[string]string{"privateKey": peer.PrivateKey.String()}
	if configured.PresharedKey != nil {
		arguments["presharedKey"] = configured.PresharedKey.String()
	}
	raw, err := json.Marshal(arguments)
	if err != nil {
		return err
	}
	_, err = s.RunProviderResourceAction(serverID, "awg", "Peer", name, "importSecret", raw)
	return err
}

func (s *Service) removeLegacyResource(serverID string, resource models.Resource) error {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	switch resource.Kind {
	case "Interface":
		interfaceID, err := uuid.Parse(resource.ID)
		if err != nil || storage.IsNotFound(err) {
			return nil
		}
		err = s.store.Servers().Interfaces(id).Delete(interfaceID)
		if storage.IsNotFound(err) {
			return nil
		}
		return err
	case "Peer":
		var spec struct {
			UserRef   string `json:"userRef"`
			PublicKey string `json:"publicKey"`
		}
		if err := json.Unmarshal(resource.Spec, &spec); err != nil {
			return err
		}
		userID, err := uuid.Parse(spec.UserRef)
		if err != nil {
			return nil
		}
		publicKey, err := agentmodels.ParseKey(spec.PublicKey)
		if err != nil {
			return nil
		}
		err = s.store.Users().Peers(userID).Delete(publicKey)
		if storage.IsNotFound(err) {
			return nil
		}
		return err
	default:
		return nil
	}
}

func (s *Service) syncLegacyResource(serverID string, resource models.Resource) error {
	id, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	switch resource.Kind {
	case "Interface":
		interfaceID, err := uuid.Parse(resource.ID)
		if err != nil {
			return nil
		}
		iface, err := s.store.Servers().Interfaces(id).Get(interfaceID)
		if storage.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		cfg := iface.InterfaceConfig
		if err := json.Unmarshal(resource.Spec, &cfg); err != nil {
			return err
		}
		iface.InterfaceConfig = cfg
		return s.store.Servers().Interfaces(id).Set(iface)
	case "Peer":
		var spec struct {
			UserRef          string   `json:"userRef"`
			PublicKey        string   `json:"publicKey"`
			Name             string   `json:"name"`
			AllowedIPs       []string `json:"allowedIPs"`
			DNS              []string `json:"dns"`
			Disabled         bool     `json:"disabled"`
			KeepaliveSeconds int      `json:"keepaliveSeconds"`
		}
		if err := json.Unmarshal(resource.Spec, &spec); err != nil {
			return err
		}
		userID, err := uuid.Parse(spec.UserRef)
		if err != nil {
			return nil
		}
		user, err := s.store.Users().Get(userID)
		if storage.IsNotFound(err) {
			return nil
		}
		if err != nil {
			return err
		}
		for peerIndex := range user.Peers {
			peer := &user.Peers[peerIndex]
			if peerPublicKey(peer) != spec.PublicKey {
				continue
			}
			peer.Name, peer.DNS, peer.Disabled = spec.Name, spec.DNS, spec.Disabled
			iface, _, err := s.getInterfaceByID(peer.InterfaceId)
			if err != nil {
				return err
			}
			for configuredIndex := range iface.Peers {
				configured := &iface.Peers[configuredIndex]
				if configured.Key.String() == spec.PublicKey {
					configured.AllowedIPs = spec.AllowedIPs
					configured.Disabled = spec.Disabled
					configured.KeepaliveInterval = time.Duration(spec.KeepaliveSeconds) * time.Second
					peers := s.store.Users().Peers(userID)
					if extended, ok := peers.(interface {
						SetWithIfacePeer(*models.Peer, agentmodels.InterfacePeer) error
					}); ok {
						return extended.SetWithIfacePeer(peer, *configured)
					}
					return peers.Set(peer)
				}
			}
		}
		return nil
	default:
		return nil
	}
}

func peerPublicKey(peer *models.Peer) string {
	publicKey := peer.PrivateKey.PublicKey()
	return publicKey.String()
}
