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

package mux

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}

func TestMethodHelpers_DispatchOnlyMatchingMethod(t *testing.T) {
	cases := []struct {
		name     string
		register func(m *Mux, path string, h func(http.ResponseWriter, *http.Request))
		method   string
	}{
		{"GET", (*Mux).GET, http.MethodGet},
		{"HEAD", (*Mux).HEAD, http.MethodHead},
		{"POST", (*Mux).POST, http.MethodPost},
		{"PUT", (*Mux).PUT, http.MethodPut},
		{"DELETE", (*Mux).DELETE, http.MethodDelete},
		{"PATCH", (*Mux).PATCH, http.MethodPatch},
		{"OPTIONS", (*Mux).OPTIONS, http.MethodOptions},
	}

	otherMethods := []string{
		http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodDelete, http.MethodPatch, http.MethodOptions,
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := NewServeMux()
			tc.register(m, "/things", okHandler)

			req := httptest.NewRequest(tc.method, "/things", nil)
			rec := httptest.NewRecorder()
			m.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected %s /things to match its own handler, got status %d", tc.method, rec.Code)
			}

			for _, other := range otherMethods {
				if other == tc.method {
					continue
				}
				// net/http.ServeMux (Go 1.22+) falls back to a registered
				// GET handler for HEAD requests when no HEAD handler exists.
				if tc.method == http.MethodGet && other == http.MethodHead {
					continue
				}
				req := httptest.NewRequest(other, "/things", nil)
				rec := httptest.NewRecorder()
				m.ServeHTTP(rec, req)
				if rec.Code == http.StatusOK {
					t.Fatalf("expected %s /things to NOT match the %s-only handler, but it did", other, tc.method)
				}
			}
		})
	}
}

func TestHandleFuncMethod_PathPattern(t *testing.T) {
	m := NewServeMux()
	var captured string
	m.GET("/interfaces/{name}", func(w http.ResponseWriter, r *http.Request) {
		captured = r.PathValue("name")
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/interfaces/wg0", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if captured != "wg0" {
		t.Fatalf("expected path value %q, got %q", "wg0", captured)
	}
}

func TestHandle_UnknownRoute404(t *testing.T) {
	m := NewServeMux()
	m.GET("/interfaces", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unregistered route, got %d", rec.Code)
	}
}

// TestMiddlewaresAreApplied verifies that middlewares passed to NewServeMux
// actually wrap every registered handler. This currently fails: Handle()
// registers the handler directly on the embedded http.ServeMux without
// running it through m.middlewares.
func TestMiddlewaresAreApplied(t *testing.T) {
	var calls []string

	tag := func(name string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls = append(calls, name)
				next.ServeHTTP(w, r)
			})
		}
	}

	m := NewServeMux(tag("first"), tag("second"))
	m.GET("/interfaces", okHandler)

	req := httptest.NewRequest(http.MethodGet, "/interfaces", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	want := []string{"first", "second"}
	if len(calls) != len(want) {
		t.Fatalf("expected middlewares %v to run, but got %v", want, calls)
	}
	for i, name := range want {
		if calls[i] != name {
			t.Fatalf("expected middleware order %v, got %v", want, calls)
		}
	}
}

func TestHandleFunc_RegistersPlainHandlerFunc(t *testing.T) {
	m := NewServeMux()
	called := false
	m.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	rec := httptest.NewRecorder()
	m.ServeHTTP(rec, req)

	if !called {
		t.Fatal("expected handler registered via HandleFunc to be invoked")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
