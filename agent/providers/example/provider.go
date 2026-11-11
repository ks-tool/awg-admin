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

package example

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/ks-tool/vpn-agent/models"

	"github.com/horchestra/pack/sdk/packschema"
	packschemavalidator "github.com/horchestra/pack/sdk/schema"
)

type EndpointSpec struct {
	Title    string `json:"title" jsonschema:"required,minLength=1"`
	Address  string `json:"address" jsonschema:"required,minLength=1"`
	Disabled bool   `json:"disabled,omitempty"`
}

type Provider struct {
	definition models.ResourceDefinition
	validator  *packschemavalidator.Compiled
	mu         sync.Mutex
	links      map[string]models.LinkOffer
}

func New() (*Provider, error) {
	reflected, err := packschema.Reflect(EndpointSpec{}, packschema.Options{})
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
	validator, err := parsed.Compile("urn:vpn:provider:example:endpoint")
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(raw)
	return &Provider{
		definition: models.ResourceDefinition{
			APIVersion: "example.ks-tool.dev/v1", Kind: "Endpoint", SpecSchema: raw, SchemaHash: "sha256:" + hex.EncodeToString(hash[:]),
			Ports:   []models.PortDefinition{{Name: "example", LinkKinds: []string{"example"}, RemotePorts: []string{"example"}}},
			Actions: []models.ActionDefinition{{Name: "inspect", Title: "Inspect endpoint", ResultType: models.ActionResultJSON, ArgumentsSchema: json.RawMessage(`{"type":"object","properties":{}}`)}},
		},
		validator: validator,
		links:     make(map[string]models.LinkOffer),
	}, nil
}

func (p *Provider) Name() string    { return "example" }
func (p *Provider) Version() string { return "0.1.0" }
func (p *Provider) Definitions() []models.ResourceDefinition {
	return []models.ResourceDefinition{p.definition}
}

func (p *Provider) Validate(_ context.Context, item models.Resource) error {
	if item.Kind != "Endpoint" {
		return errors.New("unsupported resource kind")
	}
	var spec EndpointSpec
	decoder := json.NewDecoder(bytes.NewReader(item.Spec))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&spec); err != nil {
		return err
	}
	var document any
	decoder = json.NewDecoder(bytes.NewReader(item.Spec))
	decoder.UseNumber()
	if err := decoder.Decode(&document); err != nil {
		return err
	}
	if violations := p.validator.Validate(document); len(violations) > 0 {
		return errors.New(violations[0].String())
	}
	return nil
}

func (p *Provider) Reconcile(_ context.Context, item models.Resource) (json.RawMessage, error) {
	var spec EndpointSpec
	if err := json.Unmarshal(item.Spec, &spec); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"address": spec.Address, "active": !spec.Disabled})
}

func (p *Provider) Delete(context.Context, models.Resource) error { return nil }

func (p *Provider) Action(_ context.Context, item models.Resource, action string, _ json.RawMessage) (models.ActionResult, error) {
	if action != "inspect" {
		return models.ActionResult{}, fmt.Errorf("unknown action %q", action)
	}
	return models.ActionResult{Type: models.ActionResultJSON, Data: item.Spec}, nil
}

func (p *Provider) PrepareLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint) (models.LinkOffer, error) {
	if link.Kind != "example" {
		return models.LinkOffer{}, errors.New("unsupported link kind")
	}
	data, _ := json.Marshal(map[string]string{"endpoint": endpoint.ResourceID})
	return models.LinkOffer{LinkID: link.ID, Endpoint: endpoint, Data: data}, nil
}

func (p *Provider) CommitLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint, remote models.LinkOffer) error {
	if link.Kind != "example" || remote.LinkID != link.ID {
		return errors.New("invalid example link")
	}
	p.mu.Lock()
	p.links[link.ID+endpoint.ResourceID] = remote
	p.mu.Unlock()
	return nil
}

func (p *Provider) DeleteLink(_ context.Context, link models.Link, endpoint models.LinkEndpoint, _ *models.LinkOffer) error {
	p.mu.Lock()
	delete(p.links, link.ID+endpoint.ResourceID)
	p.mu.Unlock()
	return nil
}
