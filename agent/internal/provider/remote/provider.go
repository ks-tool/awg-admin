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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ks-tool/vpn-admin/agent/internal/resource"
	"github.com/ks-tool/vpn-admin/agent/models"
)

type Provider struct {
	address           string
	definition        models.ProviderDefinition
	client            *http.Client
	protocolMu        sync.Mutex
	protocolValidated bool
}

func NewProvider(address string, definition models.ProviderDefinition, client *http.Client) (*Provider, error) {
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		return nil, errors.New("remote provider address must use HTTP or HTTPS")
	}
	if definition.Name == "" || definition.Version == "" {
		return nil, errors.New("remote provider name and version are required")
	}
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &Provider{address: strings.TrimSuffix(address, "/"), definition: definition, client: client}, nil
}

func (p *Provider) Name() string    { return p.definition.Name }
func (p *Provider) Version() string { return p.definition.Version }
func (p *Provider) Definitions() []models.ResourceDefinition {
	return p.definition.Resources
}

func (p *Provider) Validate(context.Context, models.Resource) error { return nil }

func (p *Provider) Reconcile(ctx context.Context, item models.Resource) (json.RawMessage, error) {
	var result models.Resource
	path := resourcePath(p.definition.Name, item.Kind, item.Name)
	if err := p.do(ctx, http.MethodPut, path, item, &result); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		var status models.ResourceStatus
		if err := p.do(ctx, http.MethodGet, path+"/status", nil, &status); err != nil {
			return nil, err
		}
		if status.ObservedGeneration >= result.Generation {
			switch status.Phase {
			case models.ResourceReady:
				return status.Provider, nil
			case models.ResourceDegraded:
				return nil, errors.New(statusMessage(status))
			}
		}
		if time.Now().After(deadline) {
			return nil, errors.New("remote provider reconciliation timed out")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (p *Provider) Delete(ctx context.Context, item models.Resource) error {
	return p.do(ctx, http.MethodDelete, resourcePath(p.definition.Name, item.Kind, item.Name), nil, nil)
}

func (p *Provider) Action(ctx context.Context, item models.Resource, action string, arguments json.RawMessage) (models.ActionResult, error) {
	var result models.ActionResult
	path := resourcePath(p.definition.Name, item.Kind, item.Name) + "/actions/" + url.PathEscape(action)
	err := p.do(ctx, http.MethodPost, path, models.ActionRequest{Arguments: arguments}, &result)
	return result, err
}

func (p *Provider) PrepareLink(ctx context.Context, link models.Link, endpoint models.LinkEndpoint) (models.LinkOffer, error) {
	var offer models.LinkOffer
	request := models.LinkRequest{Link: link, Endpoint: endpoint}
	err := p.do(ctx, http.MethodPost, "/v1/providers/"+url.PathEscape(p.definition.Name)+"/links/prepare", request, &offer)
	return offer, err
}

func (p *Provider) CommitLink(ctx context.Context, link models.Link, endpoint models.LinkEndpoint, remote models.LinkOffer) error {
	request := models.LinkRequest{Link: link, Endpoint: endpoint, RemoteOffer: &remote}
	return p.do(ctx, http.MethodPost, "/v1/providers/"+url.PathEscape(p.definition.Name)+"/links/commit", request, nil)
}

func (p *Provider) DeleteLink(ctx context.Context, link models.Link, endpoint models.LinkEndpoint, remote *models.LinkOffer) error {
	request := models.LinkRequest{Link: link, Endpoint: endpoint, RemoteOffer: remote}
	return p.do(ctx, http.MethodPost, "/v1/providers/"+url.PathEscape(p.definition.Name)+"/links/delete", request, nil)
}

func resourcePath(provider, kind, name string) string {
	return "/v1/providers/" + url.PathEscape(provider) + "/resources/" + url.PathEscape(kind) + "/" + url.PathEscape(name)
}

func (p *Provider) do(ctx context.Context, method, path string, input, output any) error {
	if path != "/v1/manifest" {
		if err := p.ensureProtocol(ctx); err != nil {
			return err
		}
	}
	var body *bytes.Reader
	if input == nil {
		body = bytes.NewReader(nil)
	} else {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, p.address+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("remote provider %s: %w", p.definition.Name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("remote provider %s returned %s: %s", p.definition.Name, resp.Status, strings.TrimSpace(string(message)))
	}
	if output == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(output)
}

func (p *Provider) ensureProtocol(ctx context.Context) error {
	p.protocolMu.Lock()
	validated := p.protocolValidated
	p.protocolMu.Unlock()
	if validated {
		return nil
	}
	if err := p.checkProtocol(ctx); err != nil {
		return err
	}
	p.protocolMu.Lock()
	p.protocolValidated = true
	p.protocolMu.Unlock()
	return nil
}

func (p *Provider) checkProtocol(ctx context.Context) error {
	var manifest models.Manifest
	if err := p.do(ctx, http.MethodGet, "/v1/manifest", nil, &manifest); err != nil {
		return err
	}
	if manifest.ProtocolVersion != models.ProtocolVersion {
		return fmt.Errorf("remote provider %s uses protocol %q", p.definition.Name, manifest.ProtocolVersion)
	}
	for _, provider := range manifest.Providers {
		if provider.Name == p.definition.Name && provider.Version == p.definition.Version {
			return nil
		}
	}
	return fmt.Errorf("remote provider manifest does not contain %s@%s", p.definition.Name, p.definition.Version)
}

func statusMessage(status models.ResourceStatus) string {
	if len(status.Conditions) > 0 && status.Conditions[0].Message != "" {
		return status.Conditions[0].Message
	}
	return "remote provider reconciliation failed"
}

var _ resource.Provider = (*Provider)(nil)
var _ resource.ActionProvider = (*Provider)(nil)
var _ resource.LinkProvider = (*Provider)(nil)
