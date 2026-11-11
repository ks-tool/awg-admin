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

package awg

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/storage"

	"github.com/horchestra/pack/sdk/packschema"
	packschemavalidator "github.com/horchestra/pack/sdk/schema"
)

type PeerSpec struct {
	Name             string   `json:"name" jsonschema:"required,minLength=1"`
	InterfaceRef     string   `json:"interfaceRef" jsonschema:"required,minLength=1"`
	UserRef          string   `json:"userRef,omitempty"`
	PublicKey        string   `json:"publicKey,omitempty" jsonschema:"readOnly=true"`
	AllowedIPs       []string `json:"allowedIPs" jsonschema:"required,minItems=1"`
	DNS              []string `json:"dns,omitempty"`
	Disabled         bool     `json:"disabled,omitempty"`
	KeepaliveSeconds int      `json:"keepaliveSeconds,omitempty" jsonschema:"minimum=0,maximum=65535"`
}

type clientConfigArguments struct {
	Endpoint   string   `json:"endpoint" jsonschema:"required,minLength=1"`
	AllowedIPs []string `json:"allowedIPs,omitempty"`
}

type secretArguments struct {
	PrivateKey   string `json:"privateKey" jsonschema:"required,minLength=1,format=password"`
	PresharedKey string `json:"presharedKey,omitempty" jsonschema:"format=password"`
}

func newPeerDefinition() (models.ResourceDefinition, *packschemavalidator.Compiled, error) {
	reflected, err := packschema.Reflect(PeerSpec{}, packschema.Options{})
	if err != nil {
		return models.ResourceDefinition{}, nil, err
	}
	raw, err := json.Marshal(reflected)
	if err != nil {
		return models.ResourceDefinition{}, nil, err
	}
	parsed, err := packschemavalidator.Parse(raw)
	if err != nil {
		return models.ResourceDefinition{}, nil, err
	}
	validator, err := parsed.Compile("urn:vpn:provider:awg:peer")
	if err != nil {
		return models.ResourceDefinition{}, nil, err
	}
	clientSchema, err := actionSchema(clientConfigArguments{})
	if err != nil {
		return models.ResourceDefinition{}, nil, err
	}
	secretSchema, err := actionSchema(secretArguments{})
	if err != nil {
		return models.ResourceDefinition{}, nil, err
	}
	emptySchema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	hash := sha256.Sum256(raw)
	return models.ResourceDefinition{
		APIVersion: APIVersion, Kind: "Peer", SpecSchema: raw, SchemaHash: "sha256:" + hex.EncodeToString(hash[:]),
		UISchema: json.RawMessage(`{"ui:order":["name","interfaceRef","userRef","allowedIPs","dns","disabled","keepaliveSeconds","publicKey"]}`),
		Actions: []models.ActionDefinition{
			{Name: "renderClientConfig", Title: "Render client configuration", ArgumentsSchema: clientSchema, ResultType: models.ActionResultFile, TimeoutSeconds: 30},
			{Name: "rotateKeys", Title: "Rotate peer keys", ArgumentsSchema: emptySchema, ResultType: models.ActionResultJSON, TimeoutSeconds: 30, Mutating: true, RequiresConfirmation: true},
			{Name: "revoke", Title: "Revoke peer access", ArgumentsSchema: emptySchema, ResultType: models.ActionResultJSON, TimeoutSeconds: 30, Mutating: true, RequiresConfirmation: true},
			{Name: "importSecret", Title: "Import existing private key", ArgumentsSchema: secretSchema, ResultType: models.ActionResultJSON, TimeoutSeconds: 30, Mutating: true, RequiresConfirmation: true},
		},
	}, validator, nil
}

func actionSchema(value any) (json.RawMessage, error) {
	reflected, err := packschema.Reflect(value, packschema.Options{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(reflected)
}

func (p *Provider) validatePeer(_ context.Context, item models.Resource) error {
	spec, err := p.decodePeer(item.Spec)
	if err != nil {
		return err
	}
	var document any
	decoder := json.NewDecoder(bytes.NewReader(item.Spec))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if violations := p.peerValidator.Validate(document); len(violations) > 0 {
		messages := make([]string, 0, len(violations))
		for _, violation := range violations {
			messages = append(messages, violation.String())
		}
		return errors.New(strings.Join(messages, "; "))
	}
	if strings.TrimSpace(spec.Name) == "" {
		return errors.New("peer name is required")
	}
	if spec.PublicKey != "" {
		if _, err := models.ParseKey(spec.PublicKey); err != nil {
			return fmt.Errorf("invalid peer public key: %w", err)
		}
		if p.resources != nil {
			items, err := p.resources.List()
			if err != nil {
				return err
			}
			for _, existing := range items {
				if existing.Provider != "awg" || existing.Deleted || existing.Resource.Kind != "Peer" || existing.Resource.ID == item.ID {
					continue
				}
				other, err := p.decodePeer(existing.Resource.Spec)
				if err != nil {
					return err
				}
				if other.PublicKey == spec.PublicKey {
					return fmt.Errorf("peer public key is already managed by resource %q", existing.Resource.Name)
				}
			}
		}
	}
	for _, address := range spec.AllowedIPs {
		if _, _, err := net.ParseCIDR(address); err != nil {
			return fmt.Errorf("invalid peer allowed IP %q", address)
		}
	}
	for _, address := range spec.DNS {
		if net.ParseIP(address) == nil {
			return fmt.Errorf("invalid DNS address %q", address)
		}
	}
	_, _, err = p.peerInterface(spec.InterfaceRef)
	return err
}

func (p *Provider) reconcilePeer(_ context.Context, item models.Resource) (json.RawMessage, error) {
	spec, err := p.decodePeer(item.Spec)
	if err != nil {
		return nil, err
	}
	_, interfaceSpec, err := p.peerInterface(spec.InterfaceRef)
	if err != nil {
		return nil, err
	}
	state, stateErr := p.secrets.get(item.ID)
	if stateErr != nil && !storage.IsNotFound(stateErr) {
		return nil, stateErr
	}
	privateKey := state.PrivateKey
	publicKeyValue := spec.PublicKey
	if publicKeyValue == "" && !models.IsEmpty(privateKey) {
		publicKeyValue = publicKeyString(privateKey)
	}
	if publicKeyValue == "" {
		privateKey, err = models.GeneratePrivateKey()
		if err != nil {
			return nil, err
		}
		publicKeyValue = publicKeyString(privateKey)
	}
	publicKey, err := models.ParseKey(publicKeyValue)
	if err != nil {
		return nil, err
	}
	if !models.IsEmpty(privateKey) && privateKey.PublicKey() != publicKey {
		return nil, errors.New("stored private key does not match peer public key")
	}
	old, err := p.store.Get(interfaceSpec.Interface)
	if err != nil {
		return nil, err
	}
	cfg := *old
	cfg.Peers = make([]models.InterfacePeer, 0, len(old.Peers))
	for _, existing := range old.Peers {
		if state.PreviousPublicKey == "" || existing.Key.String() != state.PreviousPublicKey {
			cfg.Peers = append(cfg.Peers, existing)
		}
	}
	peer := models.InterfacePeer{
		Key: publicKey, AllowedIPs: spec.AllowedIPs, Disabled: spec.Disabled,
		KeepaliveInterval: time.Duration(spec.KeepaliveSeconds) * time.Second,
	}
	index := -1
	for i := range cfg.Peers {
		if cfg.Peers[i].Key == publicKey {
			index = i
			peer.PresharedKey = cfg.Peers[i].PresharedKey
			peer.Endpoint = cfg.Peers[i].Endpoint
			break
		}
	}
	if state.PresharedKey != nil {
		peer.PresharedKey = state.PresharedKey
	}
	if index >= 0 {
		cfg.Peers[index] = peer
	} else {
		cfg.Peers = append(cfg.Peers, peer)
	}
	if err := p.network.One(old, cfg); err != nil {
		return nil, err
	}
	if err := p.store.Set(&cfg); err != nil {
		return nil, err
	}
	if !models.IsEmpty(privateKey) {
		if err := p.secrets.put(item.ID, secretRecord{Interface: interfaceSpec.Interface, PrivateKey: privateKey, PresharedKey: state.PresharedKey}); err != nil {
			return nil, err
		}
	}
	if spec.PublicKey == "" {
		spec.PublicKey = publicKeyValue
		if err := p.updatePeerDesired(item, spec); err != nil {
			return nil, err
		}
	}
	return json.Marshal(map[string]any{
		"interface": interfaceSpec.Interface, "publicKey": publicKeyValue,
		"secretMigrationPending": models.IsEmpty(privateKey),
	})
}

func (p *Provider) deletePeer(_ context.Context, item models.Resource) error {
	spec, err := p.decodePeer(item.Spec)
	if err != nil {
		return err
	}
	_, interfaceSpec, err := p.peerInterface(spec.InterfaceRef)
	if err != nil {
		return err
	}
	old, err := p.store.Get(interfaceSpec.Interface)
	if err != nil {
		return err
	}
	cfg := *old
	cfg.Peers = make([]models.InterfacePeer, 0, len(old.Peers))
	for _, peer := range old.Peers {
		if peer.Key.String() != spec.PublicKey {
			cfg.Peers = append(cfg.Peers, peer)
		}
	}
	if err := p.network.One(old, cfg); err != nil {
		return err
	}
	if err := p.store.Set(&cfg); err != nil {
		return err
	}
	return p.secrets.delete(item.ID)
}

func (p *Provider) Action(_ context.Context, item models.Resource, action string, arguments json.RawMessage) (models.ActionResult, error) {
	if item.Kind == "Interface" && action == "importSecret" {
		return p.importInterfaceSecret(item, arguments)
	}
	if item.Kind != "Peer" {
		return models.ActionResult{}, errors.New("action is supported only for peers")
	}
	switch action {
	case "renderClientConfig":
		return p.renderClientConfig(item, arguments)
	case "rotateKeys":
		return p.rotatePeerKeys(item)
	case "revoke":
		return p.revokePeer(item)
	case "importSecret":
		return p.importPeerSecret(item, arguments)
	default:
		return models.ActionResult{}, errors.New("unknown peer action")
	}
}

func (p *Provider) renderClientConfig(item models.Resource, rawArguments json.RawMessage) (models.ActionResult, error) {
	var arguments clientConfigArguments
	if err := json.Unmarshal(rawArguments, &arguments); err != nil {
		return models.ActionResult{}, err
	}
	if arguments.Endpoint == "" {
		return models.ActionResult{}, errors.New("endpoint is required")
	}
	spec, err := p.decodePeer(item.Spec)
	if err != nil {
		return models.ActionResult{}, err
	}
	state, err := p.secrets.get(item.ID)
	if err != nil || models.IsEmpty(state.PrivateKey) {
		return models.ActionResult{}, errors.New("peer private key is not available: secret migration is pending")
	}
	_, interfaceSpec, err := p.peerInterface(spec.InterfaceRef)
	if err != nil {
		return models.ActionResult{}, err
	}
	iface, err := p.store.Get(interfaceSpec.Interface)
	if err != nil {
		return models.ActionResult{}, err
	}
	var peer *models.InterfacePeer
	for i := range iface.Peers {
		if iface.Peers[i].Key.String() == spec.PublicKey {
			peer = &iface.Peers[i]
			break
		}
	}
	if peer == nil {
		return models.ActionResult{}, errors.New("peer is not applied to the interface")
	}
	var config strings.Builder
	fmt.Fprintf(&config, "[Interface]\nPrivateKey = %s\nAddress = %s\n", state.PrivateKey.String(), strings.Join(spec.AllowedIPs, ", "))
	dns := spec.DNS
	if len(dns) == 0 {
		dns = iface.DNS
	}
	if len(dns) > 0 {
		fmt.Fprintf(&config, "DNS = %s\n", strings.Join(dns, ", "))
	}
	fmt.Fprintf(&config, "\n[Peer]\nPublicKey = %s\n", publicKeyString(iface.PrivateKey))
	if peer.PresharedKey != nil {
		fmt.Fprintf(&config, "PresharedKey = %s\n", peer.PresharedKey.String())
	}
	allowedIPs := arguments.AllowedIPs
	if len(allowedIPs) == 0 {
		allowedIPs = []string{"0.0.0.0/0"}
	}
	fmt.Fprintf(&config, "AllowedIPs = %s\nEndpoint = %s\n", strings.Join(allowedIPs, ", "), arguments.Endpoint)
	if spec.KeepaliveSeconds > 0 {
		fmt.Fprintf(&config, "PersistentKeepalive = %d\n", spec.KeepaliveSeconds)
	}
	data, _ := json.Marshal(config.String())
	return models.ActionResult{Type: models.ActionResultFile, Data: data, FileName: spec.Name + ".conf", MediaType: "text/plain"}, nil
}

func (p *Provider) rotatePeerKeys(item models.Resource) (models.ActionResult, error) {
	privateKey, err := models.GeneratePrivateKey()
	if err != nil {
		return models.ActionResult{}, err
	}
	spec, err := p.decodePeer(item.Spec)
	if err != nil {
		return models.ActionResult{}, err
	}
	previousPublicKey := spec.PublicKey
	spec.PublicKey = publicKeyString(privateKey)
	if err := p.secrets.put(item.ID, secretRecord{Interface: spec.InterfaceRef, PrivateKey: privateKey, PreviousPublicKey: previousPublicKey}); err != nil {
		return models.ActionResult{}, err
	}
	if err := p.updatePeerDesired(item, spec); err != nil {
		return models.ActionResult{}, err
	}
	data, _ := json.Marshal(map[string]string{"publicKey": spec.PublicKey})
	return models.ActionResult{Type: models.ActionResultJSON, Data: data}, nil
}

func (p *Provider) revokePeer(item models.Resource) (models.ActionResult, error) {
	spec, err := p.decodePeer(item.Spec)
	if err != nil {
		return models.ActionResult{}, err
	}
	spec.Disabled = true
	if err := p.updatePeerDesired(item, spec); err != nil {
		return models.ActionResult{}, err
	}
	return models.ActionResult{Type: models.ActionResultJSON, Data: json.RawMessage(`{"revoked":true}`)}, nil
}

func (p *Provider) importPeerSecret(item models.Resource, rawArguments json.RawMessage) (models.ActionResult, error) {
	var arguments secretArguments
	if err := json.Unmarshal(rawArguments, &arguments); err != nil {
		return models.ActionResult{}, err
	}
	privateKey, err := models.ParseKey(arguments.PrivateKey)
	if err != nil {
		return models.ActionResult{}, err
	}
	spec, err := p.decodePeer(item.Spec)
	if err != nil {
		return models.ActionResult{}, err
	}
	publicKeyValue := publicKeyString(privateKey)
	if spec.PublicKey != "" && spec.PublicKey != publicKeyValue {
		return models.ActionResult{}, errors.New("private key does not match peer public key")
	}
	spec.PublicKey = publicKeyValue
	var presharedKey *models.Key
	if arguments.PresharedKey != "" {
		parsed, err := models.ParseKey(arguments.PresharedKey)
		if err != nil {
			return models.ActionResult{}, err
		}
		presharedKey = &parsed
	}
	if err := p.secrets.put(item.ID, secretRecord{Interface: spec.InterfaceRef, PrivateKey: privateKey, PresharedKey: presharedKey}); err != nil {
		return models.ActionResult{}, err
	}
	if err := p.updatePeerDesired(item, spec); err != nil {
		return models.ActionResult{}, err
	}
	return models.ActionResult{Type: models.ActionResultJSON, Data: json.RawMessage(`{"imported":true}`)}, nil
}

func (p *Provider) updatePeerDesired(item models.Resource, spec PeerSpec) error {
	stored, err := p.resources.Get("awg", "Peer", item.Name)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	stored.Resource.Spec = raw
	stored.Resource.Generation++
	stored.Resource.Status = models.ResourceStatus{Phase: models.ResourcePending}
	return p.resources.Put(stored)
}

func (p *Provider) peerInterface(reference string) (resource.StoredResource, InterfaceSpec, error) {
	if p.resources == nil {
		return resource.StoredResource{}, InterfaceSpec{}, errors.New("resource inventory is not configured")
	}
	items, err := p.resources.List()
	if err != nil {
		return resource.StoredResource{}, InterfaceSpec{}, err
	}
	for _, item := range items {
		if item.Provider != "awg" || item.Deleted || item.Resource.Kind != "Interface" {
			continue
		}
		spec, err := p.decode(item.Resource.Spec)
		if err != nil {
			return resource.StoredResource{}, InterfaceSpec{}, err
		}
		if reference == item.Resource.ID || reference == item.Resource.Name || reference == spec.Interface {
			return item, spec, nil
		}
	}
	return resource.StoredResource{}, InterfaceSpec{}, errors.New("referenced interface was not found")
}

func (p *Provider) decodePeer(raw json.RawMessage) (PeerSpec, error) {
	var spec PeerSpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return PeerSpec{}, err
	}
	return spec, nil
}
