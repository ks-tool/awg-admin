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
	"path/filepath"
	"regexp"
	"strings"

	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/storage"

	"github.com/horchestra/pack/sdk/packschema"
	packschemavalidator "github.com/horchestra/pack/sdk/schema"
)

var interfaceName = regexp.MustCompile(`^[a-zA-Z0-9_.-]+$`)

type Network interface {
	One(*models.InterfaceConfig, models.InterfaceConfig) error
	Delete(string) error
}

type interfaceSecretArguments struct {
	PrivateKey string   `json:"privateKey" jsonschema:"required,minLength=1,format=password"`
	PreUp      []string `json:"preUp,omitempty"`
	PostUp     []string `json:"postUp,omitempty"`
	PreDown    []string `json:"preDown,omitempty"`
	PostDown   []string `json:"postDown,omitempty"`
}

type Provider struct {
	version        string
	store          storage.Storage
	resources      resource.Store
	secrets        *secretStore
	links          *linkStore
	network        Network
	definition     models.ResourceDefinition
	validator      *packschemavalidator.Compiled
	peerDefinition models.ResourceDefinition
	peerValidator  *packschemavalidator.Compiled
}

func New(store storage.Storage, secretsRoot string, network Network, version string, inventory ...resource.Store) (*Provider, error) {
	secrets, err := newSecretStore(secretsRoot)
	if err != nil {
		return nil, err
	}
	links, err := newLinkStore(filepath.Join(filepath.Dir(secretsRoot), "links"))
	if err != nil {
		return nil, err
	}
	reflected, err := packschema.Reflect(InterfaceSpec{}, packschema.Options{})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(reflected)
	if err != nil {
		return nil, err
	}
	parsed, err := packschemavalidator.Parse(raw)
	if err != nil {
		return nil, err
	}
	validator, err := parsed.Compile("urn:vpn:provider:awg:interface")
	if err != nil {
		return nil, err
	}
	peerDefinition, peerValidator, err := newPeerDefinition()
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	uiSchema := json.RawMessage(`{"ui:order":["iface","addr","listen","mtu","dns","disabled","table","fwMark","jc","jmin","jmax","s1","s2","s3","s4","h1","h2","h3","h4","i1","i2","i3","i4","i5"]}`)
	secretSchema, err := actionSchema(interfaceSecretArguments{})
	if err != nil {
		return nil, err
	}
	var resources resource.Store
	if len(inventory) > 0 {
		resources = inventory[0]
	}
	return &Provider{
		version: version, store: store, resources: resources, secrets: secrets, links: links, network: network, validator: validator,
		peerDefinition: peerDefinition, peerValidator: peerValidator,
		definition: models.ResourceDefinition{
			APIVersion: APIVersion, Kind: "Interface", SpecSchema: raw, UISchema: uiSchema,
			SchemaHash: "sha256:" + hex.EncodeToString(hash[:]),
			Ports:      []models.PortDefinition{{Name: "tunnel", Title: "VPN tunnel", LinkKinds: []string{"awg"}, RemotePorts: []string{"tunnel"}}},
			Actions: []models.ActionDefinition{{
				Name: "importSecret", Title: "Import existing interface secrets and hooks", ArgumentsSchema: secretSchema,
				ResultType: models.ActionResultJSON, TimeoutSeconds: 30, Mutating: true, RequiresConfirmation: true,
			}},
		},
	}, nil
}

func (p *Provider) Name() string    { return "awg" }
func (p *Provider) Version() string { return p.version }
func (p *Provider) Definitions() []models.ResourceDefinition {
	return []models.ResourceDefinition{p.definition, p.peerDefinition}
}

func (p *Provider) Validate(ctx context.Context, resource models.Resource) error {
	if resource.Kind == "Peer" {
		return p.validatePeer(ctx, resource)
	}
	if resource.Kind != "" && resource.Kind != "Interface" {
		return errors.New("unsupported resource kind")
	}
	spec, err := p.decode(resource.Spec)
	if err != nil {
		return err
	}
	var document any
	decoder := json.NewDecoder(bytes.NewReader(resource.Spec))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if violations := p.validator.Validate(document); len(violations) > 0 {
		messages := make([]string, 0, len(violations))
		for _, violation := range violations {
			messages = append(messages, violation.String())
		}
		return errors.New(strings.Join(messages, "; "))
	}
	return p.validateSemantics(resource.ID, spec)
}

func (p *Provider) Reconcile(ctx context.Context, resource models.Resource) (json.RawMessage, error) {
	if resource.Kind == "Peer" {
		return p.reconcilePeer(ctx, resource)
	}
	return p.reconcileInterface(ctx, resource)
}

func (p *Provider) reconcileInterface(_ context.Context, resource models.Resource) (json.RawMessage, error) {
	spec, err := p.decode(resource.Spec)
	if err != nil {
		return nil, err
	}
	state, stateErr := p.secrets.get(resource.ID)
	if stateErr != nil && !storage.IsNotFound(stateErr) {
		return nil, stateErr
	}
	if stateErr == nil && state.Interface != spec.Interface {
		return nil, errors.New("interface name is immutable; create a new resource")
	}

	var previous *models.InterfaceConfig
	old, err := p.store.Get(spec.Interface)
	if err == nil {
		previous = old
	} else if !storage.IsNotFound(err) {
		return nil, err
	}
	cfg := spec.config()
	if previous != nil {
		cfg.PrivateKey = previous.PrivateKey
		cfg.Peers = previous.Peers
		cfg.PreUp, cfg.PostUp = previous.PreUp, previous.PostUp
		cfg.PreDown, cfg.PostDown = previous.PreDown, previous.PostDown
	}
	if models.IsEmpty(cfg.PrivateKey) && stateErr == nil {
		cfg.PrivateKey = state.PrivateKey
	}
	if models.IsEmpty(cfg.PrivateKey) {
		cfg.PrivateKey, err = models.GeneratePrivateKey()
		if err != nil {
			return nil, err
		}
	}
	if err := p.network.One(previous, cfg); err != nil {
		return nil, err
	}
	if err := p.store.Set(&cfg); err != nil {
		return nil, err
	}
	if err := p.secrets.put(resource.ID, secretRecord{Interface: spec.Interface, PrivateKey: cfg.PrivateKey}); err != nil {
		return nil, err
	}
	status, err := json.Marshal(map[string]any{"interface": cfg.Interface, "publicKey": publicKeyString(cfg.PrivateKey)})
	return status, err
}

func publicKeyString(privateKey models.Key) string {
	publicKey := privateKey.PublicKey()
	return publicKey.String()
}

func (p *Provider) importInterfaceSecret(item models.Resource, rawArguments json.RawMessage) (models.ActionResult, error) {
	var arguments interfaceSecretArguments
	if err := json.Unmarshal(rawArguments, &arguments); err != nil {
		return models.ActionResult{}, err
	}
	privateKey, err := models.ParseKey(arguments.PrivateKey)
	if err != nil {
		return models.ActionResult{}, err
	}
	spec, err := p.decode(item.Spec)
	if err != nil {
		return models.ActionResult{}, err
	}
	var previous *models.InterfaceConfig
	cfg := spec.config()
	old, err := p.store.Get(spec.Interface)
	if err == nil {
		previous = old
		cfg = *old
	} else if !storage.IsNotFound(err) {
		return models.ActionResult{}, err
	}
	cfg.PrivateKey = privateKey
	cfg.PreUp = append([]string(nil), arguments.PreUp...)
	cfg.PostUp = append([]string(nil), arguments.PostUp...)
	cfg.PreDown = append([]string(nil), arguments.PreDown...)
	cfg.PostDown = append([]string(nil), arguments.PostDown...)
	if err := p.network.One(previous, cfg); err != nil {
		return models.ActionResult{}, err
	}
	if err := p.store.Set(&cfg); err != nil {
		return models.ActionResult{}, err
	}
	if err := p.secrets.put(item.ID, secretRecord{Interface: spec.Interface, PrivateKey: privateKey}); err != nil {
		return models.ActionResult{}, err
	}
	return models.ActionResult{Type: models.ActionResultJSON, Data: json.RawMessage(`{"imported":true}`)}, nil
}

func (p *Provider) Delete(ctx context.Context, resource models.Resource) error {
	if resource.Kind == "Peer" {
		return p.deletePeer(ctx, resource)
	}
	return p.deleteInterface(ctx, resource)
}

func (p *Provider) deleteInterface(_ context.Context, resource models.Resource) error {
	spec, err := p.decode(resource.Spec)
	if err != nil {
		return err
	}
	if err := p.network.Delete(spec.Interface); err != nil {
		return err
	}
	if err := p.store.Delete(spec.Interface); err != nil && !storage.IsNotFound(err) {
		return err
	}
	return p.secrets.delete(resource.ID)
}

func (p *Provider) decode(raw json.RawMessage) (InterfaceSpec, error) {
	var spec InterfaceSpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return InterfaceSpec{}, err
	}
	return spec, nil
}

func (p *Provider) validateSemantics(resourceID string, spec InterfaceSpec) error {
	if !interfaceName.MatchString(spec.Interface) {
		return errors.New("interface name may contain only letters, digits, dots, underscores and dashes")
	}
	if p.resources != nil {
		items, err := p.resources.List()
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.Provider != "awg" || item.Deleted || item.Resource.Kind != "Interface" || item.Resource.ID == resourceID {
				continue
			}
			other, err := p.decode(item.Resource.Spec)
			if err != nil {
				return err
			}
			if other.Interface == spec.Interface {
				return fmt.Errorf("interface %q is already managed by resource %q", spec.Interface, item.Resource.Name)
			}
		}
	}
	ip, subnet, err := net.ParseCIDR(spec.Address)
	if err != nil || ip == nil {
		return errors.New("address must be a CIDR with a host IP")
	}
	for _, address := range spec.DNS {
		if net.ParseIP(address) == nil {
			return fmt.Errorf("invalid DNS address %q", address)
		}
	}
	if spec.Jmin != nil && spec.Jmax != nil && *spec.Jmin > *spec.Jmax {
		return errors.New("jmin must not exceed jmax")
	}
	configs, err := p.store.List()
	if err != nil {
		return err
	}
	for _, other := range configs {
		if other.Interface == spec.Interface {
			continue
		}
		if other.ListenPort != 0 && other.ListenPort == spec.ListenPort {
			return fmt.Errorf("listen port %d is already used by interface %q", spec.ListenPort, other.Interface)
		}
		_, otherSubnet, otherErr := net.ParseCIDR(other.Address)
		if otherErr == nil && (subnet.Contains(otherSubnet.IP) || otherSubnet.Contains(subnet.IP)) {
			return fmt.Errorf("address subnet overlaps interface %q", other.Interface)
		}
	}
	return nil
}
