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

package agentclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/ks-tool/vpn-admin/models"
)

func (c *Client) Manifest(ctx context.Context) (*models.Manifest, error) {
	var manifest models.Manifest
	if err := c.resourceRequest(ctx, http.MethodGet, "/v1/manifest", nil, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func (c *Client) ProviderSchema(ctx context.Context, provider string) (*models.ProviderDefinition, error) {
	var definition models.ProviderDefinition
	path := "/v1/providers/" + url.PathEscape(provider) + "/schema"
	if err := c.resourceRequest(ctx, http.MethodGet, path, nil, &definition); err != nil {
		return nil, err
	}
	return &definition, nil
}

func (c *Client) ListResources(ctx context.Context, provider string) ([]models.Resource, error) {
	var resources []models.Resource
	path := "/v1/providers/" + url.PathEscape(provider) + "/resources"
	if err := c.resourceRequest(ctx, http.MethodGet, path, nil, &resources); err != nil {
		return nil, err
	}
	return resources, nil
}

func (c *Client) PutResource(ctx context.Context, provider, kind, name string, resource models.Resource) (*models.Resource, error) {
	var result models.Resource
	path := resourcePath(provider, kind, name)
	if err := c.resourceRequest(ctx, http.MethodPut, path, resource, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (c *Client) DeleteResource(ctx context.Context, provider, kind, name string) error {
	return c.resourceRequest(ctx, http.MethodDelete, resourcePath(provider, kind, name), nil, nil)
}

func (c *Client) ResourceStatus(ctx context.Context, provider, kind, name string) (*models.ResourceStatus, error) {
	var status models.ResourceStatus
	path := resourcePath(provider, kind, name) + "/status"
	if err := c.resourceRequest(ctx, http.MethodGet, path, nil, &status); err != nil {
		return nil, err
	}
	return &status, nil
}

func (c *Client) RunResourceAction(ctx context.Context, provider, kind, name, action string, arguments json.RawMessage) (*models.ActionResult, error) {
	var result models.ActionResult
	path := resourcePath(provider, kind, name) + "/actions/" + url.PathEscape(action)
	request := models.ActionRequest{Arguments: arguments}
	if err := c.resourceRequest(ctx, http.MethodPost, path, request, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func resourcePath(provider, kind, name string) string {
	return "/v1/providers/" + url.PathEscape(provider) + "/resources/" + url.PathEscape(kind) + "/" + url.PathEscape(name)
}

func (c *Client) resourceRequest(ctx context.Context, method, path string, input, output any) error {
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
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if method == http.MethodPut || method == http.MethodDelete {
		markIdempotent(req)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return responseError(resp, "")
	}
	if output == nil || resp.StatusCode == http.StatusNoContent {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(output); err != nil {
		return fmt.Errorf("decode %s %s: %w", method, path, err)
	}
	return nil
}

func (c *Client) PrepareLink(ctx context.Context, provider string, request models.LinkRequest) (*models.LinkOffer, error) {
	var offer models.LinkOffer
	path := "/v1/providers/" + url.PathEscape(provider) + "/links/prepare"
	if err := c.resourceRequest(ctx, http.MethodPost, path, request, &offer); err != nil {
		return nil, err
	}
	return &offer, nil
}

func (c *Client) CommitLink(ctx context.Context, provider string, request models.LinkRequest) error {
	path := "/v1/providers/" + url.PathEscape(provider) + "/links/commit"
	return c.resourceRequest(ctx, http.MethodPost, path, request, nil)
}

func (c *Client) DeleteLink(ctx context.Context, provider string, request models.LinkRequest) error {
	path := "/v1/providers/" + url.PathEscape(provider) + "/links/delete"
	return c.resourceRequest(ctx, http.MethodPost, path, request, nil)
}
