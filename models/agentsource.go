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

import "github.com/google/uuid"

// AgentSource is a named, reusable place to fetch the awg-agent binary
// from when deploying it to a server (see Service.DeployAgent) — shown as
// a preset in the "Deploy agent" UI instead of typing a URL every time.
type AgentSource struct {
	ID   uuid.UUID `json:"id"`
	Name string    `json:"name"`
	URL  string    `json:"url,omitempty"`
	// Path, when set instead of URL, is a path to the agent binary already
	// present on the filesystem of the machine running awg-admin (desktop:
	// the user's own machine; standalone server: that server's host) — read
	// directly and uploaded over SSH the same way a CacheLocally URL preset
	// is, without ever downloading anything. Exactly one of URL or Path is
	// set; CacheLocally is meaningless here (there's nothing to cache) and
	// is always false for a Path-based source.
	Path string `json:"path,omitempty"`
	// CacheLocally, when false (the default), has the *managed server*
	// download URL itself over SSH (curl/wget — see internal/sshclient's
	// DownloadFile) so the binary's bytes never pass through the machine
	// running awg-admin. When true, awg-admin downloads URL once into a
	// local cache (keyed by ID) and uploads it the traditional way on
	// every deploy that uses this source — for servers without outbound
	// internet access to URL, at the cost of awg-admin's own bandwidth.
	CacheLocally bool `json:"cacheLocally,omitempty"`
}
