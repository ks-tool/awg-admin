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
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ks-tool/vpn-admin/models"
	"golang.org/x/mod/semver"
)

const (
	officialProviderRepository = "oci://ghcr.io/ks-tool/vpn-providers"
	officialProviderGitHubRepo = "ks-tool/vpn-agent"
	registryResponseLimit      = 4 << 20
	registryPageLimit          = 20
	registryRequestTimeout     = 20 * time.Second
)

var registryHTTPClient = &http.Client{Timeout: registryRequestTimeout}

// ListArtifactTags returns an OCI repository's tags, newest semantic versions
// first. Public GHCR repositories use their anonymous bearer-token flow.
func (s *Service) ListArtifactTags(repository string, registry models.Registry) ([]string, error) {
	debugOp("ListArtifactTags").Str("repository", repository).Msg("listing OCI artifact tags")
	ctx, cancel := context.WithTimeout(context.Background(), registryRequestTimeout)
	defer cancel()
	ref, err := parseOCIRepository(repository)
	if err != nil {
		return nil, err
	}
	registry, err = s.resolveRegistry(registry)
	if err != nil {
		return nil, err
	}
	endpoint := ref.endpoint(registry.PlainHTTP) + "/tags/list?n=100"
	var response struct {
		Tags []string `json:"tags"`
	}
	if err := registryGetPages(ctx, endpoint, registry, func() any {
		return &struct {
			Tags []string `json:"tags"`
		}{}
	}, func(page any) {
		response.Tags = append(response.Tags, page.(*struct {
			Tags []string `json:"tags"`
		}).Tags...)
	}); err != nil {
		return nil, fmt.Errorf("list OCI tags for %s: %w", repository, err)
	}
	return sortedTags(response.Tags), nil
}

// ListProviderRepositories discovers providers below an untagged repository
// prefix. GHCR does not expose the OCI catalog, so its official provider list is
// derived from the providers/<name>/v* release tags and each repository's tags
// are still read through the OCI API.
func (s *Service) ListProviderRepositories(repository string, registry models.Registry) ([]models.ProviderRepository, error) {
	debugOp("ListProviderRepositories").Str("repository", repository).Msg("listing OCI providers")
	ref, err := parseOCIRepository(repository)
	if err != nil {
		return nil, err
	}
	registry, err = s.resolveRegistry(registry)
	if err != nil {
		return nil, err
	}
	if repository = ref.withoutTag(); repository == officialProviderRepository {
		ctx, cancel := context.WithTimeout(context.Background(), registryRequestTimeout)
		defer cancel()
		return officialProviders(ctx)
	}

	ctx, cancel := context.WithTimeout(context.Background(), registryRequestTimeout)
	defer cancel()
	endpoint := ref.registryEndpoint(registry.PlainHTTP) + "/_catalog?n=100"
	names, err := registryCatalog(ctx, endpoint, registry)
	if err != nil {
		return nil, fmt.Errorf("list OCI provider catalog: %w", err)
	}
	prefix := ref.path + "/"
	providers := make([]models.ProviderRepository, 0)
	for _, name := range names {
		provider, ok := strings.CutPrefix(name, prefix)
		if !ok || len(provider) == 0 || strings.Contains(provider, "/") {
			continue
		}
		providers = append(providers, models.ProviderRepository{Name: provider, Repository: "oci://" + ref.host + "/" + name})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	return providers, nil
}

type ociRepository struct {
	host string
	path string
}

func parseOCIRepository(repository string) (ociRepository, error) {
	repository = strings.TrimSuffix(strings.TrimSpace(repository), "/")
	if !strings.HasPrefix(repository, "oci://") {
		return ociRepository{}, fmt.Errorf("OCI repository %q must start with oci://", repository)
	}
	ref := strings.TrimPrefix(repository, "oci://")
	host, path, ok := strings.Cut(ref, "/")
	if !ok || len(host) == 0 || len(path) == 0 || strings.ContainsAny(host, "@/?#") {
		return ociRepository{}, fmt.Errorf("OCI repository %q must include a registry and repository", repository)
	}
	if strings.Contains(path, "@") || strings.Contains(path[strings.LastIndex(path, "/")+1:], ":") {
		return ociRepository{}, fmt.Errorf("OCI repository %q must not include a tag or digest", repository)
	}
	for _, segment := range strings.Split(path, "/") {
		if len(segment) == 0 || segment == "." || segment == ".." || strings.ContainsAny(segment, " ?#") {
			return ociRepository{}, fmt.Errorf("invalid OCI repository %q", repository)
		}
	}
	return ociRepository{host: host, path: path}, nil
}

func (r ociRepository) withoutTag() string { return "oci://" + r.host + "/" + r.path }

func (r ociRepository) registryEndpoint(plainHTTP bool) string {
	scheme := "https"
	if plainHTTP {
		scheme = "http"
	}
	return scheme + "://" + r.host + "/v2"
}

func (r ociRepository) endpoint(plainHTTP bool) string {
	return r.registryEndpoint(plainHTTP) + "/" + r.path
}

func validateRegistry(registry models.Registry) error {
	if (len(registry.Username) == 0) != (len(registry.Password) == 0) {
		return fmt.Errorf("registry username and password must be provided together")
	}
	return nil
}

func registryGetPages(ctx context.Context, endpoint string, registry models.Registry, newPage func() any, appendPage func(any)) error {
	for range registryPageLimit {
		page := newPage()
		next, err := registryGet(ctx, endpoint, registry, page)
		if err != nil {
			return err
		}
		appendPage(page)
		if len(next) == 0 {
			return nil
		}
		endpoint = next
	}
	return fmt.Errorf("too many registry response pages")
}

func registryCatalog(ctx context.Context, endpoint string, registry models.Registry) ([]string, error) {
	var names []string
	err := registryGetPages(ctx, endpoint, registry, func() any {
		return &struct {
			Repositories []string `json:"repositories"`
		}{}
	}, func(page any) {
		names = append(names, page.(*struct {
			Repositories []string `json:"repositories"`
		}).Repositories...)
	})
	return names, err
}

func registryGet(ctx context.Context, endpoint string, registry models.Registry, target any) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "vpn-admin")
	setRegistryAuth(req, registry, "")
	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		challenge := resp.Header.Get("WWW-Authenticate")
		_ = resp.Body.Close()
		token, tokenErr := registryBearerToken(ctx, challenge, registry)
		if tokenErr != nil {
			return "", tokenErr
		}
		req, err = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return "", err
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("User-Agent", "vpn-admin")
		setRegistryAuth(req, registry, token)
		resp, err = registryHTTPClient.Do(req)
		if err != nil {
			return "", err
		}
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("registry request failed: %s", resp.Status)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, registryResponseLimit)).Decode(target); err != nil {
		return "", fmt.Errorf("decode registry response: %w", err)
	}
	return nextRegistryPage(resp.Header.Get("Link")), nil
}

func setRegistryAuth(req *http.Request, registry models.Registry, bearer string) {
	if len(bearer) > 0 {
		req.Header.Set("Authorization", "Bearer "+bearer)
	} else if len(registry.Username) > 0 {
		req.SetBasicAuth(registry.Username, registry.Password)
	}
}

func registryBearerToken(ctx context.Context, challenge string, registry models.Registry) (string, error) {
	parameters := bearerChallenge(challenge)
	realm := parameters["realm"]
	if len(realm) == 0 {
		return "", fmt.Errorf("registry authentication requires a bearer realm")
	}
	tokenURL, err := url.Parse(realm)
	if err != nil || (tokenURL.Scheme != "https" && tokenURL.Scheme != "http") {
		return "", fmt.Errorf("registry returned an invalid bearer realm")
	}
	query := tokenURL.Query()
	for _, key := range []string{"service", "scope"} {
		if value := parameters[key]; len(value) > 0 {
			query.Set(key, value)
		}
	}
	tokenURL.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "vpn-admin")
	if len(registry.Username) > 0 {
		req.SetBasicAuth(registry.Username, registry.Password)
	}
	resp, err := registryHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request registry bearer token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry bearer token request failed: %s", resp.Status)
	}
	var body struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, registryResponseLimit)).Decode(&body); err != nil {
		return "", fmt.Errorf("decode registry bearer token: %w", err)
	}
	if len(body.Token) == 0 {
		body.Token = body.AccessToken
	}
	if len(body.Token) == 0 {
		return "", fmt.Errorf("registry bearer token response is empty")
	}
	return body.Token, nil
}

func bearerChallenge(challenge string) map[string]string {
	out := map[string]string{}
	scheme, rest, ok := strings.Cut(challenge, " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return out
	}
	for _, part := range strings.Split(rest, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			out[strings.ToLower(key)] = strings.Trim(value, `"`)
		}
	}
	return out
}

func nextRegistryPage(link string) string {
	for _, part := range strings.Split(link, ",") {
		section := strings.Split(strings.TrimSpace(part), ";")
		if len(section) < 2 || !strings.Contains(strings.Join(section[1:], ";"), `rel="next"`) {
			continue
		}
		return strings.Trim(strings.TrimSpace(section[0]), "<>")
	}
	return ""
}

func sortedTags(tags []string) []string {
	seen := make(map[string]bool, len(tags))
	out := make([]string, 0, len(tags))
	for _, tag := range tags {
		if len(tag) > 0 && !seen[tag] {
			seen[tag] = true
			out = append(out, tag)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if semver.IsValid(out[i]) && semver.IsValid(out[j]) {
			return semver.Compare(out[i], out[j]) > 0
		}
		return out[i] > out[j]
	})
	return out
}

func officialProviders(ctx context.Context) ([]models.ProviderRepository, error) {
	found := map[string]bool{}
	for page := 1; page <= registryPageLimit; page++ {
		endpoint := fmt.Sprintf("https://api.github.com/repos/%s/tags?per_page=100&page=%d", officialProviderGitHubRepo, page)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("User-Agent", "vpn-admin")
		resp, err := registryHTTPClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch provider release tags: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("provider release tags request failed: %s", resp.Status)
		}
		var tags []struct {
			Name string `json:"name"`
		}
		err = json.NewDecoder(io.LimitReader(resp.Body, registryResponseLimit)).Decode(&tags)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("decode provider release tags: %w", err)
		}
		for _, tag := range tags {
			name, ok := strings.CutPrefix(tag.Name, "providers/")
			if !ok {
				continue
			}
			name, _, ok = strings.Cut(name, "/v")
			if ok && len(name) > 0 && !strings.Contains(name, "/") {
				found[name] = true
			}
		}
		if len(tags) < 100 {
			break
		}
	}
	providers := make([]models.ProviderRepository, 0, len(found))
	for name := range found {
		providers = append(providers, models.ProviderRepository{
			Name: name, Repository: officialProviderRepository + "/" + name,
		})
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })
	return providers, nil
}
