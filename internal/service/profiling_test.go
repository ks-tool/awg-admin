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
	"errors"
	"testing"

	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// TestUpdateServerPreservesAgentDesiredStateFlags guards against an unrelated
// server edit silently wiping the agent desired-state flags (MonitoringDisabled,
// ProfilingEnabled) — which the edit form never round-trips, so in.Agent always
// carries their zero value and a wholesale srv.Agent = in.Agent would reset them.
func TestUpdateServerPreservesAgentDesiredStateFlags(t *testing.T) {
	svc := newTestService(t)

	srv, err := svc.CreateServer(ServerInput{
		Name: "s1",
		SSH:  models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
	})
	if err != nil {
		t.Fatalf("CreateServer: %v", err)
	}

	// Simulate the dedicated toggles having set both desired-state flags.
	stored, err := svc.store.Servers().Get(srv.ID)
	if err != nil {
		t.Fatalf("get server: %v", err)
	}
	stored.Agent.MonitoringDisabled = true
	stored.Agent.ProfilingEnabled = true
	if err := svc.store.Servers().Set(stored); err != nil {
		t.Fatalf("set server: %v", err)
	}

	// An unrelated edit (rename) via the form path — in.Agent omits the flags.
	updated, err := svc.UpdateServer(srv.ID.String(), ServerInput{
		Name:  "s1-renamed",
		SSH:   models.SSHConfig{Host: "example.invalid", User: "root", Password: "x"},
		Agent: models.Agent{Address: stored.Agent.Address},
	})
	if err != nil {
		t.Fatalf("UpdateServer: %v", err)
	}
	if !updated.Agent.MonitoringDisabled {
		t.Error("MonitoringDisabled was wiped by an unrelated server edit")
	}
	if !updated.Agent.ProfilingEnabled {
		t.Error("ProfilingEnabled was wiped by an unrelated server edit")
	}
}

// TestGetServerProfileRejectsUnknownKind is a guard against an arbitrary path
// being appended to the agent's /debug/pprof/ URL: an unknown kind must be
// rejected as a ValidationError (HTTP 400) before any agent call is attempted.
func TestGetServerProfileRejectsUnknownKind(t *testing.T) {
	svc := newTestService(t)

	_, err := svc.GetServerProfile(uuid.NewString(), "../secrets", 0)
	if err == nil {
		t.Fatal("expected an error for an unknown profile kind")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected a *ValidationError (→ HTTP 400), got %T: %v", err, err)
	}
}

// TestProfileFilePrefix checks the download-name sanitizer keeps safe characters
// and can't be broken (or used to inject a Content-Disposition) by an odd server
// name, falling back to "agent" when nothing usable survives.
func TestProfileFilePrefix(t *testing.T) {
	cases := map[string]string{
		"srv-1":        "srv-1",
		"my server!":   "my-server",
		"a/b\\c":       "a-b-c",
		`ev"il`:        "ev-il",
		"  ":           "agent",
		"...":          "agent",
		"exit.node-01": "exit.node-01",
	}
	for in, want := range cases {
		if got := profileFilePrefix(in); got != want {
			t.Errorf("profileFilePrefix(%q) = %q, want %q", in, got, want)
		}
	}
}
