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

package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ks-tool/awg-admin/internal/service"
	"github.com/ks-tool/awg-admin/storage/boltdb"
)

func newTestServiceForAPI(t *testing.T) *service.Service {
	t.Helper()
	db, err := boltdb.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return service.New(db)
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestBasicAuthMiddlewarePassesThroughWhenDisabled(t *testing.T) {
	svc := newTestServiceForAPI(t)
	sessions := newSessionStore()
	handler := BasicAuthMiddleware(svc, sessions)(okHandler())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (basic auth is off by default)", w.Code)
	}
}

func TestBasicAuthMiddlewareChallengesAnonymousRequestsWhenEnabled(t *testing.T) {
	svc := newTestServiceForAPI(t)
	if err := svc.SetBasicAuthEnabled(true); err != nil {
		t.Fatalf("SetBasicAuthEnabled: %v", err)
	}
	sessions := newSessionStore()
	handler := BasicAuthMiddleware(svc, sessions)(okHandler())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for an anonymous request with no session and no basic auth creds", w.Code)
	}
	if w.Header().Get("WWW-Authenticate") == "" {
		t.Error("expected a WWW-Authenticate challenge header")
	}
}

func TestBasicAuthMiddlewareAcceptsCorrectCredentialsWhenEnabled(t *testing.T) {
	svc := newTestServiceForAPI(t)
	if err := svc.SetBasicAuthEnabled(true); err != nil {
		t.Fatalf("SetBasicAuthEnabled: %v", err)
	}
	sessions := newSessionStore()
	handler := BasicAuthMiddleware(svc, sessions)(okHandler())

	req := httptest.NewRequest("GET", "/", nil)
	req.SetBasicAuth("admin", "admin") // storage/boltdb seeds admin/admin on first run
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with the seeded admin/admin credentials", w.Code)
	}
}

// TestBasicAuthMiddlewareSkipsCheckForAValidSession is the regression test
// for the reported bug: once basic auth is enabled, an admin who's already
// logged in via the normal session-cookie flow got challenged by the
// browser's native Basic Auth popup on every request — including just to
// reach the Settings page and turn it back off. A request carrying a
// valid session cookie must bypass the basic auth check entirely.
func TestBasicAuthMiddlewareSkipsCheckForAValidSession(t *testing.T) {
	svc := newTestServiceForAPI(t)
	if err := svc.SetBasicAuthEnabled(true); err != nil {
		t.Fatalf("SetBasicAuthEnabled: %v", err)
	}
	sessions := newSessionStore()
	token := sessions.create()
	handler := BasicAuthMiddleware(svc, sessions)(okHandler())

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a valid session cookie should skip the basic auth challenge entirely", w.Code)
	}
}

func TestBasicAuthMiddlewareRejectsStaleSessionCookie(t *testing.T) {
	svc := newTestServiceForAPI(t)
	if err := svc.SetBasicAuthEnabled(true); err != nil {
		t.Fatalf("SetBasicAuthEnabled: %v", err)
	}
	sessions := newSessionStore()
	handler := BasicAuthMiddleware(svc, sessions)(okHandler())

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "not-a-real-token"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — an invalid/stale session cookie must not bypass basic auth", w.Code)
	}
}
