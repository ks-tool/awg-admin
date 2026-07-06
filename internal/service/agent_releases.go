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
	"sort"
	"strings"
	"time"

	"github.com/ks-tool/awg-admin/models"
)

const (
	// agentReleasesRepo is the GitHub repo whose "agent/v*" releases publish the
	// awg-agent binaries (see .goreleaser.agent.yaml). The admin app itself
	// releases under plain "v*" tags on the same repo, which are filtered out.
	agentReleasesRepo = "ks-tool/awg-admin"
	// agentReleasesTagPrefix marks the agent releases apart from the admin ones.
	agentReleasesTagPrefix = "agent/v"
	// agentReleasesLimit caps how many of the newest agent releases are offered.
	agentReleasesLimit = 5
	// agentReleasesMaxBody bounds how much of the GitHub API response is read, so
	// a hostile/huge response can't exhaust memory.
	agentReleasesMaxBody = 8 << 20 // 8 MiB
	// agentReleasesTimeout bounds the whole GitHub round-trip.
	agentReleasesTimeout = 15 * time.Second
)

// ListAgentReleases fetches the newest agent releases from GitHub (tagged
// agent/v*) and flattens their downloadable awg-agent binaries into
// AgentReleaseAssets — newest release first — ready to become URL
// AgentSources via the "GitHub releases" picker. It reaches the public GitHub
// API unauthenticated (no credentials involved); a failure (offline, rate
// limit, non-200) is returned as an error the UI reports.
func (s *Service) ListAgentReleases() ([]models.AgentReleaseAsset, error) {
	debugOp("ListAgentReleases").Msg("listing agent releases from github")
	return fetchAgentReleases(context.Background())
}

// githubRelease is the subset of GitHub's release JSON we consume.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	Draft       bool   `json:"draft"`
	PublishedAt string `json:"published_at"`
	Assets      []struct {
		Name        string `json:"name"`
		Size        int64  `json:"size"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchAgentReleases(ctx context.Context) ([]models.AgentReleaseAsset, error) {
	ctx, cancel := context.WithTimeout(ctx, agentReleasesTimeout)
	defer cancel()

	url := fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=100", agentReleasesRepo)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// GitHub requires a User-Agent; the Accept header pins the API version.
	req.Header.Set("User-Agent", "awg-admin")
	req.Header.Set("Accept", "application/vnd.github+json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching github releases: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github releases request failed: %s", resp.Status)
	}

	var releases []githubRelease
	if err := json.NewDecoder(io.LimitReader(resp.Body, agentReleasesMaxBody)).Decode(&releases); err != nil {
		return nil, fmt.Errorf("decoding github releases: %w", err)
	}

	// Keep only the published agent releases, newest first (the API is already
	// roughly newest-first, but sort explicitly so ordering doesn't depend on
	// that), then take the newest few.
	agent := releases[:0:0]
	for _, r := range releases {
		if r.Draft || !strings.HasPrefix(r.TagName, agentReleasesTagPrefix) {
			continue
		}
		agent = append(agent, r)
	}
	sort.SliceStable(agent, func(i, j int) bool {
		return releasePublishedUnix(agent[i].PublishedAt) > releasePublishedUnix(agent[j].PublishedAt)
	})
	if len(agent) > agentReleasesLimit {
		agent = agent[:agentReleasesLimit]
	}

	assets := make([]models.AgentReleaseAsset, 0, len(agent)*4)
	for _, r := range agent {
		version := strings.TrimPrefix(r.TagName, "agent/") // "agent/v1.0.0" -> "v1.0.0"
		// Deterministic within-release order (userspace before kernel, amd64
		// before arm64) so the list doesn't jitter between fetches.
		sort.SliceStable(r.Assets, func(i, j int) bool { return r.Assets[i].Name < r.Assets[j].Name })
		for _, a := range r.Assets {
			if !isAgentBinaryAsset(a.Name) {
				continue
			}
			assets = append(assets, models.AgentReleaseAsset{
				Name:        a.Name,
				Version:     version,
				Arch:        assetArch(a.Name),
				Userspace:   strings.Contains(a.Name, "userspace"),
				URL:         a.DownloadURL,
				Size:        a.Size,
				PublishedAt: r.PublishedAt,
			})
		}
	}
	return assets, nil
}

// isAgentBinaryAsset reports whether a release asset is a downloadable
// awg-agent binary (as opposed to checksums.txt, signatures, etc.). It matches
// both the versionless name scheme ("awg-agent_linux_amd64") and a versioned
// one ("awg-agent_1.2.3_linux_amd64").
func isAgentBinaryAsset(name string) bool {
	if !strings.HasPrefix(name, "awg-agent") || !strings.Contains(name, "linux") {
		return false
	}
	return strings.Contains(name, "amd64") || strings.Contains(name, "arm64")
}

// assetArch extracts the CPU architecture from an asset name, or "" if absent.
func assetArch(name string) string {
	switch {
	case strings.Contains(name, "arm64"):
		return "arm64"
	case strings.Contains(name, "amd64"):
		return "amd64"
	default:
		return ""
	}
}

// releasePublishedUnix parses an RFC3339 publish time to a Unix timestamp for
// sorting; an unparseable/empty value sorts oldest (0).
func releasePublishedUnix(s string) int64 {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return 0
	}
	return t.Unix()
}
