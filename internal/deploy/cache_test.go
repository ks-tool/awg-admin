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
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// TestFetchCached covers the one part of internal/deploy that doesn't need
// a real *ssh.Client: the rest of the package (ToAgent, sshclient.Dial/
// UploadFile/DownloadFile) talks to an actual SSH server and has no cheap
// fake to substitute, so it's verified manually instead (see TODO.md).
func TestFetchCached(t *testing.T) {
	var requests int
	body := []byte("fake-binary-content")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	src := models.AgentSource{ID: uuid.New(), Name: "test", URL: srv.URL, CacheLocally: true}
	t.Cleanup(func() { _ = DeleteCache(src.ID) })

	data, err := fetchCached(context.Background(), src)
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if string(data) != string(body) {
		t.Fatalf("got %q, want %q", data, body)
	}
	if requests != 1 {
		t.Fatalf("expected 1 request to the source, got %d", requests)
	}

	path, err := cachePath(src.ID)
	if err != nil {
		t.Fatalf("cachePath: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected cache file to exist: %v", err)
	}

	// Second fetch must hit the cache, not the source again.
	data2, err := fetchCached(context.Background(), src)
	if err != nil {
		t.Fatalf("second fetch (should use cache): %v", err)
	}
	if string(data2) != string(body) {
		t.Fatalf("cached read got %q, want %q", data2, body)
	}
	if requests != 1 {
		t.Fatalf("expected the second fetch to reuse the cache, got %d requests", requests)
	}

	if err := DeleteCache(src.ID); err != nil {
		t.Fatalf("DeleteCache: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected cache file to be removed, stat err: %v", err)
	}

	// Deleting a never-cached ID's cache must be a no-op, not an error.
	if err := DeleteCache(uuid.New()); err != nil {
		t.Fatalf("DeleteCache on missing file: %v", err)
	}
}
