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

package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ks-tool/vpn-agent/internal/resource"
	"github.com/ks-tool/vpn-agent/models"
	"github.com/ks-tool/vpn-agent/storage"

	"github.com/horchestra/pack/sdk/packschema"
	packschemavalidator "github.com/horchestra/pack/sdk/schema"
)

type Provider struct {
	adapter      Adapter
	version      string
	resources    resource.Store
	secrets      *secretStore
	processes    *processManager
	firewall     *listenerFirewall
	definitions  []models.ResourceDefinition
	ifaceChecker *packschemavalidator.Compiled
	peerChecker  *packschemavalidator.Compiled
}

type clientConfigArguments struct {
	Endpoint string `json:"endpoint" jsonschema:"required,minLength=1"`
}

type secretArguments struct {
	Secret string `json:"secret" jsonschema:"required,minLength=1,format=password"`
}

func New(adapter Adapter, version string, resources resource.Store, root, core string) (*Provider, error) {
	secrets, err := newSecretStore(filepath.Join(root, "secrets"))
	if err != nil {
		return nil, err
	}
	processes, err := newProcessManager(core, filepath.Join(root, "cores"))
	if err != nil {
		return nil, err
	}
	interfaceSchema, interfaceChecker, err := definitionSchema(adapter.InterfaceSchema(), "urn:vpn:provider:"+adapter.Name()+":interface")
	if err != nil {
		return nil, err
	}
	peerSchema, peerChecker, err := definitionSchema(PeerSpec{}, "urn:vpn:provider:"+adapter.Name()+":peer")
	if err != nil {
		return nil, err
	}
	clientSchema, err := actionSchema(clientConfigArguments{})
	if err != nil {
		return nil, err
	}
	secretSchema, err := actionSchema(secretArguments{})
	if err != nil {
		return nil, err
	}
	emptySchema := json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`)
	interfaceDefinition := models.ResourceDefinition{
		APIVersion: adapter.APIVersion(), Kind: "Interface", SpecSchema: interfaceSchema,
		UISchema: adapter.InterfaceUISchema(), SchemaHash: schemaHash(interfaceSchema),
	}
	peerDefinition := models.ResourceDefinition{
		APIVersion: adapter.APIVersion(), Kind: "Peer", SpecSchema: peerSchema, SchemaHash: schemaHash(peerSchema),
		UISchema: json.RawMessage(`{"ui:order":["name","interfaceRef","userRef","publicKey","disabled"]}`),
		Actions: []models.ActionDefinition{
			{Name: "renderClientConfig", Title: "Render client connection URI", ArgumentsSchema: clientSchema, ResultType: models.ActionResultFile, TimeoutSeconds: 30},
			{Name: "rotateSecret", Title: "Rotate client credentials", ArgumentsSchema: emptySchema, ResultType: models.ActionResultJSON, TimeoutSeconds: 30, Mutating: true, RequiresConfirmation: true},
			{Name: "revoke", Title: "Revoke client access", ArgumentsSchema: emptySchema, ResultType: models.ActionResultJSON, TimeoutSeconds: 30, Mutating: true, RequiresConfirmation: true},
			{Name: "importSecret", Title: "Import client credentials", ArgumentsSchema: secretSchema, ResultType: models.ActionResultJSON, TimeoutSeconds: 30, Mutating: true, RequiresConfirmation: true},
		},
	}
	return &Provider{
		adapter: adapter, version: version, resources: resources, secrets: secrets, processes: processes, firewall: newListenerFirewall(),
		definitions: []models.ResourceDefinition{interfaceDefinition, peerDefinition}, ifaceChecker: interfaceChecker, peerChecker: peerChecker,
	}, nil
}

func (p *Provider) Name() string                             { return p.adapter.Name() }
func (p *Provider) Version() string                          { return p.version }
func (p *Provider) Definitions() []models.ResourceDefinition { return p.definitions }
func (p *Provider) Close() error                             { return p.processes.close() }

func (p *Provider) Validate(_ context.Context, item models.Resource) error {
	switch item.Kind {
	case "Interface":
		if err := validateSchema(p.ifaceChecker, item.Spec); err != nil {
			return err
		}
		if err := p.adapter.ValidateInterface(item.Spec); err != nil {
			return err
		}
		return p.validateUniqueInterface(item)
	case "Peer":
		if err := validateSchema(p.peerChecker, item.Spec); err != nil {
			return err
		}
		spec, err := decodePeer(item.Spec)
		if err != nil {
			return err
		}
		if len(strings.TrimSpace(spec.Name)) == 0 {
			return errors.New("peer name is required")
		}
		_, _, err = p.interfaceResource(spec.InterfaceRef)
		return err
	default:
		return errors.New("unsupported resource kind")
	}
}

func (p *Provider) Reconcile(_ context.Context, item models.Resource) (json.RawMessage, error) {
	switch item.Kind {
	case "Interface":
		return p.reconcileInterface(item)
	case "Peer":
		return p.reconcilePeer(item)
	default:
		return nil, errors.New("unsupported resource kind")
	}
}

func (p *Provider) reconcileInterface(item models.Resource) (json.RawMessage, error) {
	var spec struct {
		Name     string `json:"name"`
		Port     int    `json:"port"`
		Disabled bool   `json:"disabled"`
	}
	if err := json.Unmarshal(item.Spec, &spec); err != nil {
		return nil, err
	}
	if err := p.applyInterface(item); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"name": spec.Name, "port": spec.Port, "active": !spec.Disabled})
}

func (p *Provider) reconcilePeer(item models.Resource) (json.RawMessage, error) {
	spec, err := decodePeer(item.Spec)
	if err != nil {
		return nil, err
	}
	secret, err := p.secrets.get(item.ID)
	if storage.IsNotFound(err) {
		secret, err = p.adapter.NewSecret()
		if err == nil {
			err = p.secrets.put(item.ID, secret)
		}
	}
	if err != nil {
		return nil, err
	}
	interfaceItem, _, err := p.interfaceResource(spec.InterfaceRef)
	if err != nil {
		return nil, err
	}
	if err := p.applyInterface(interfaceItem); err != nil {
		return nil, err
	}
	if len(spec.PublicKey) == 0 {
		spec.PublicKey = item.ID
	}
	return json.Marshal(map[string]any{"interfaceRef": spec.InterfaceRef, "publicKey": spec.PublicKey, "applied": true})
}

func (p *Provider) applyInterface(interfaceItem models.Resource) error {
	var desired struct {
		Port     int  `json:"port"`
		Disabled bool `json:"disabled"`
	}
	if err := json.Unmarshal(interfaceItem.Spec, &desired); err != nil {
		return err
	}
	if desired.Disabled {
		if err := p.processes.stop(interfaceItem.ID); err != nil {
			return err
		}
		return p.firewall.apply(desired.Port, p.adapter.FirewallProtocol(), false)
	}
	peers, err := p.interfacePeers(interfaceItem)
	if err != nil {
		return err
	}
	config, err := p.adapter.RenderServer(interfaceItem.Spec, peers, p.processes.resourceDir(interfaceItem.ID))
	if err != nil {
		return err
	}
	if err := p.processes.apply(interfaceItem.ID, config); err != nil {
		return err
	}
	if err := p.firewall.apply(desired.Port, p.adapter.FirewallProtocol(), true); err != nil {
		_ = p.processes.stop(interfaceItem.ID)
		return err
	}
	return nil
}

func (p *Provider) interfacePeers(interfaceItem models.Resource) ([]Peer, error) {
	items, err := p.resources.List()
	if err != nil {
		return nil, err
	}
	peers := make([]Peer, 0)
	for _, stored := range items {
		if stored.Provider != p.Name() || stored.Deleted || stored.Resource.Kind != "Peer" {
			continue
		}
		spec, err := decodePeer(stored.Resource.Spec)
		if err != nil {
			return nil, err
		}
		if !interfaceReferenceMatches(interfaceItem, spec.InterfaceRef) {
			continue
		}
		secret, err := p.secrets.get(stored.Resource.ID)
		if storage.IsNotFound(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		peers = append(peers, Peer{Resource: stored.Resource, Spec: spec, Secret: secret})
	}
	return peers, nil
}

func (p *Provider) Delete(_ context.Context, item models.Resource) error {
	switch item.Kind {
	case "Interface":
		if err := p.processes.stop(item.ID); err != nil {
			return err
		}
		var spec struct {
			Port int `json:"port"`
		}
		if err := json.Unmarshal(item.Spec, &spec); err != nil {
			return err
		}
		if err := p.firewall.apply(spec.Port, p.adapter.FirewallProtocol(), false); err != nil {
			return err
		}
		return osRemoveAll(p.processes.resourceDir(item.ID))
	case "Peer":
		spec, err := decodePeer(item.Spec)
		if err != nil {
			return err
		}
		interfaceItem, _, err := p.interfaceResource(spec.InterfaceRef)
		if err != nil {
			return err
		}
		if err := p.applyInterface(interfaceItem); err != nil {
			return err
		}
		return p.secrets.delete(item.ID)
	default:
		return errors.New("unsupported resource kind")
	}
}

func (p *Provider) Action(_ context.Context, item models.Resource, action string, arguments json.RawMessage) (models.ActionResult, error) {
	if item.Kind != "Peer" {
		return models.ActionResult{}, errors.New("actions are supported only for peers")
	}
	spec, err := decodePeer(item.Spec)
	if err != nil {
		return models.ActionResult{}, err
	}
	switch action {
	case "renderClientConfig":
		var input clientConfigArguments
		if err := json.Unmarshal(arguments, &input); err != nil {
			return models.ActionResult{}, err
		}
		secret, err := p.secrets.get(item.ID)
		if err != nil {
			return models.ActionResult{}, errors.New("client credentials are not initialized")
		}
		interfaceItem, _, err := p.interfaceResource(spec.InterfaceRef)
		if err != nil {
			return models.ActionResult{}, err
		}
		config, err := p.adapter.RenderClient(interfaceItem.Spec, Peer{Resource: item, Spec: spec, Secret: secret}, input.Endpoint)
		if err != nil {
			return models.ActionResult{}, err
		}
		data, _ := json.Marshal(config)
		return models.ActionResult{Type: models.ActionResultFile, Data: data, FileName: spec.Name + ".txt", MediaType: "text/plain"}, nil
	case "rotateSecret":
		secret, err := p.adapter.NewSecret()
		if err != nil {
			return models.ActionResult{}, err
		}
		if err := p.secrets.put(item.ID, secret); err != nil {
			return models.ActionResult{}, err
		}
		return models.ActionResult{Type: models.ActionResultJSON, Data: json.RawMessage(`{"rotated":true}`)}, nil
	case "revoke":
		spec.Disabled = true
		if err := p.updateDesired(item, spec); err != nil {
			return models.ActionResult{}, err
		}
		return models.ActionResult{Type: models.ActionResultJSON, Data: json.RawMessage(`{"revoked":true}`)}, nil
	case "importSecret":
		var input secretArguments
		if err := json.Unmarshal(arguments, &input); err != nil {
			return models.ActionResult{}, err
		}
		if err := p.adapter.ValidateSecret(input.Secret); err != nil {
			return models.ActionResult{}, err
		}
		if err := p.secrets.put(item.ID, input.Secret); err != nil {
			return models.ActionResult{}, err
		}
		return models.ActionResult{Type: models.ActionResultJSON, Data: json.RawMessage(`{"imported":true}`)}, nil
	default:
		return models.ActionResult{}, fmt.Errorf("unknown action %q", action)
	}
}

func (p *Provider) updateDesired(item models.Resource, spec PeerSpec) error {
	stored, err := p.resources.Get(p.Name(), "Peer", item.Name)
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

func (p *Provider) interfaceResource(reference string) (models.Resource, json.RawMessage, error) {
	items, err := p.resources.List()
	if err != nil {
		return models.Resource{}, nil, err
	}
	for _, item := range items {
		if item.Provider == p.Name() && !item.Deleted && item.Resource.Kind == "Interface" && interfaceReferenceMatches(item.Resource, reference) {
			return item.Resource, item.Resource.Spec, nil
		}
	}
	return models.Resource{}, nil, errors.New("referenced interface was not found")
}

func interfaceReferenceMatches(item models.Resource, reference string) bool {
	if reference == item.ID || reference == item.Name {
		return true
	}
	var spec struct {
		Name string `json:"name"`
	}
	return json.Unmarshal(item.Spec, &spec) == nil && reference == spec.Name
}

func (p *Provider) validateUniqueInterface(item models.Resource) error {
	var spec struct {
		Name string `json:"name"`
		Port int    `json:"port"`
	}
	if err := json.Unmarshal(item.Spec, &spec); err != nil {
		return err
	}
	items, err := p.resources.List()
	if err != nil {
		return err
	}
	for _, stored := range items {
		if stored.Provider != p.Name() || stored.Deleted || stored.Resource.Kind != "Interface" || stored.Resource.ID == item.ID {
			continue
		}
		var other struct {
			Name string `json:"name"`
			Port int    `json:"port"`
		}
		if err := json.Unmarshal(stored.Resource.Spec, &other); err != nil {
			return err
		}
		if other.Name == spec.Name {
			return fmt.Errorf("interface name %q is already in use", spec.Name)
		}
		if other.Port == spec.Port {
			return fmt.Errorf("listen port %d is already in use by interface %q", spec.Port, other.Name)
		}
	}
	return nil
}

func decodePeer(raw json.RawMessage) (PeerSpec, error) {
	var spec PeerSpec
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return PeerSpec{}, err
	}
	return spec, nil
}

func definitionSchema(value any, id string) (json.RawMessage, *packschemavalidator.Compiled, error) {
	reflected, err := packschema.Reflect(value, packschema.Options{})
	if err != nil {
		return nil, nil, err
	}
	raw, err := json.Marshal(reflected)
	if err != nil {
		return nil, nil, err
	}
	parsed, err := packschemavalidator.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	checker, err := parsed.Compile(id)
	return raw, checker, err
}

func actionSchema(value any) (json.RawMessage, error) {
	reflected, err := packschema.Reflect(value, packschema.Options{})
	if err != nil {
		return nil, err
	}
	return json.Marshal(reflected)
}

func validateSchema(checker *packschemavalidator.Compiled, raw json.RawMessage) error {
	var document any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if violations := checker.Validate(document); len(violations) > 0 {
		messages := make([]string, 0, len(violations))
		for _, violation := range violations {
			messages = append(messages, violation.String())
		}
		return errors.New(strings.Join(messages, "; "))
	}
	return nil
}

func schemaHash(raw json.RawMessage) string {
	hash := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(hash[:])
}

func osRemoveAll(path string) error {
	return os.RemoveAll(path)
}
