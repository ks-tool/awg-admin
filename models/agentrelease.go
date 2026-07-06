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

package models

// AgentReleaseAsset is one downloadable awg-agent binary published in a GitHub
// release (tagged agent/v*), offered by the "GitHub releases" picker in the
// Add-agent-source UI so an official binary can be turned into a URL
// AgentSource without pasting a URL by hand. Assembled by
// Service.ListAgentReleases from the public GitHub releases API — it is derived,
// never persisted.
type AgentReleaseAsset struct {
	// Name is the release-relative asset filename, e.g.
	// "awg-agent-userspace_linux_amd64".
	Name string `json:"name"`
	// Version is the release version: the agent/v* tag with the "agent/" prefix
	// stripped, e.g. "v1.0.0".
	Version string `json:"version"`
	// Arch is the target CPU architecture parsed from the asset name
	// ("amd64"/"arm64"), or "" if it couldn't be determined.
	Arch string `json:"arch"`
	// Userspace is true for the userspace agent binary (awg-agent-userspace),
	// false for the kernel one (awg-agent). It maps straight onto
	// AgentSource.Userspace so a source created from this asset skips (or keeps)
	// the AmneziaWG kernel-module pre-check on deploy correctly.
	Userspace bool `json:"userspace"`
	// URL is the asset's direct download URL (GitHub's browser_download_url).
	URL string `json:"url"`
	// Size is the asset size in bytes.
	Size int64 `json:"size"`
	// PublishedAt is the owning release's publish time (RFC3339), for display
	// and ordering in the picker.
	PublishedAt string `json:"publishedAt"`
}
