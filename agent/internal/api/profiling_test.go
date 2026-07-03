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
	"strings"
	"testing"
)

// TestProfilingGuardBlocksWhenDisabled is the core safety property: the pprof
// endpoints are inert (403, and the wrapped handler never runs) until profiling
// is explicitly enabled, and live once it is.
func TestProfilingGuardBlocksWhenDisabled(t *testing.T) {
	h := &Handler{} // only h.profiling is touched by the guard
	called := false
	guarded := h.guardProfiling(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	rec := httptest.NewRecorder()
	guarded(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disabled: want 403, got %d", rec.Code)
	}
	if called {
		t.Fatal("the pprof handler must not run while profiling is disabled")
	}

	h.profiling.Store(true)
	rec = httptest.NewRecorder()
	guarded(rec, httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil))
	if rec.Code != http.StatusOK || !called {
		t.Fatalf("enabled: want 200 and handler called, got code=%d called=%v", rec.Code, called)
	}
}

// TestSetAndGetProfilingState round-trips the runtime toggle.
func TestSetAndGetProfilingState(t *testing.T) {
	h := &Handler{}

	rec := httptest.NewRecorder()
	h.getProfilingState(rec, httptest.NewRequest(http.MethodGet, "/profiling", nil))
	if !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("default state should be disabled, got %q", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.setProfilingState(rec, httptest.NewRequest(http.MethodPatch, "/profiling", strings.NewReader(`{"enabled":true}`)))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Fatalf("enable: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if !h.profiling.Load() {
		t.Fatal("profiling flag not set after enable")
	}

	rec = httptest.NewRecorder()
	h.setProfilingState(rec, httptest.NewRequest(http.MethodPatch, "/profiling", strings.NewReader(`{"enabled":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("disable: code=%d", rec.Code)
	}
	if h.profiling.Load() {
		t.Fatal("profiling flag not cleared after disable")
	}
}
