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

package deploy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// cacheDir returns the directory awg-admin caches downloaded agent
// binaries in for AgentSource presets with CacheLocally set, creating it
// (and its parents) if missing.
//
// This is a *sibling* of "$HOME/.awg-admin", not a child of it —
// "$HOME/.awg-admin" is the boltdb file itself (see storage/boltdb.Open),
// a plain file, not a directory, so anything under it would fail to
// create with ENOTDIR.
func cacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".awg-admin-cache", "agent-binaries")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("create agent binary cache dir: %w", err)
	}
	return dir, nil
}

func cachePath(id uuid.UUID) (string, error) {
	dir, err := cacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, id.String()), nil
}

// fetchCached returns src's binary, downloading it from src.URL and
// caching it on disk (keyed by src.ID) the first time it's used; later
// calls for the same preset reuse the cached file instead of
// re-downloading. Only meaningful for src.CacheLocally == true presets —
// ToAgent decides whether to call this at all.
func fetchCached(ctx context.Context, src models.AgentSource) ([]byte, error) {
	path, err := cachePath(src.ID)
	if err != nil {
		return nil, err
	}

	if data, err := os.ReadFile(path); err == nil {
		return data, nil
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("read cached agent binary: %w", err)
	}

	data, err := downloadURL(ctx, src.URL)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("cache agent binary: %w", err)
	}
	return data, nil
}

// RefreshCache re-downloads src.URL unconditionally, replacing any
// previously cached copy — for "rolling" URLs whose content changes over
// time, where fetchCached's normal "reuse what's already on disk" behavior
// would otherwise keep deploying whatever was cached the first time
// forever. Only meaningful for src.CacheLocally == true presets.
func RefreshCache(ctx context.Context, src models.AgentSource) ([]byte, error) {
	path, err := cachePath(src.ID)
	if err != nil {
		return nil, err
	}

	data, err := downloadURL(ctx, src.URL)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, fmt.Errorf("cache agent binary: %w", err)
	}
	return data, nil
}

// DeleteCache removes id's cached binary, if any. Called when an
// AgentSource preset with CacheLocally set is deleted, so cache files
// don't accumulate on disk forever (see Service.DeleteAgentSource).
func DeleteCache(id uuid.UUID) error {
	path, err := cachePath(id)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove cached agent binary: %w", err)
	}
	return nil
}

func downloadURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download %s: unexpected status %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}
