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
	"fmt"
	"strings"
	"time"

	"github.com/ks-tool/awg-admin/internal/agentclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// validProfileKinds is the allowlist of pprof profile names the admin will
// fetch from an agent (a subset of net/http/pprof). It both documents the
// supported kinds and stops an arbitrary path being appended to the agent's
// /debug/pprof/ URL. "profile" is the CPU profile; "trace" is an execution
// trace; the rest are instantaneous snapshots.
var validProfileKinds = map[string]bool{
	"goroutine":    true,
	"heap":         true,
	"allocs":       true,
	"threadcreate": true,
	"block":        true,
	"mutex":        true,
	"profile":      true,
	"trace":        true,
}

// profileMaxSeconds bounds a requested CPU/trace sampling window.
const profileMaxSeconds = 60

// ProfileDump is a fetched pprof dump plus a suggested download filename,
// returned by GetServerProfile. Data marshals as base64 over the Wails/JSON
// boundary, but the desktop path (App.SaveServerProfile) writes it as raw bytes
// and the HTTP path streams it, so that never matters in practice.
type ProfileDump struct {
	Filename string `json:"filename"`
	Data     []byte `json:"data"`
}

// SetServerProfiling turns the agent's Go runtime profiling (the /debug/pprof
// endpoints) on/off for serverID and persists the desired state on the server
// record so SyncServer re-applies it (a freshly (re)deployed agent starts with
// profiling off). Mirrors SetServerMonitoring.
func (s *Service) SetServerProfiling(serverID string, enabled bool) (*models.Server, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	if err := s.callAgent(srv, func(ctx context.Context, c *agentclient.Client) error {
		return c.SetProfilingEnabled(ctx, enabled)
	}); err != nil {
		return nil, err
	}

	srv.Agent.ProfilingEnabled = enabled
	if err := s.store.Servers().Set(srv); err != nil {
		return nil, err
	}
	return srv, nil
}

// GetServerProfile fetches a Go runtime profiling dump (pprof) from serverID's
// agent. name must be one of validProfileKinds; seconds (clamped to
// 1..profileMaxSeconds) sets the sampling window for the CPU "profile"/"trace"
// kinds and is ignored for the instantaneous ones. The agent answers 403 unless
// profiling is enabled there (SetServerProfiling), surfaced as an error here.
func (s *Service) GetServerProfile(serverID, name string, seconds int) (*ProfileDump, error) {
	if !validProfileKinds[name] {
		return nil, invalidInput("unknown profile kind %q", name)
	}
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return nil, err
	}

	timed := name == "profile" || name == "trace"
	if timed {
		if seconds <= 0 {
			seconds = 30
		}
		if seconds > profileMaxSeconds {
			seconds = profileMaxSeconds
		}
	} else {
		seconds = 0
	}

	// A CPU/trace profile blocks for its whole sampling window, which the
	// default 15s push budget would cut short — give it the window plus a
	// margin. Instantaneous dumps use the normal timeout.
	timeout := pushTimeout
	if timed {
		timeout = time.Duration(seconds+15) * time.Second
	}

	// For a timed profile, don't let a deadline (the profile overrunning its
	// window) be mistaken for a dead tunnel and re-sampled — see callAgentTimeout.
	// The instantaneous kinds are fast, so a timeout there does mean a dead socket.
	var data []byte
	if err := s.callAgentTimeout(srv, timeout, !timed, func(ctx context.Context, c *agentclient.Client) error {
		var e error
		data, e = c.Profile(ctx, name, seconds)
		return e
	}); err != nil {
		return nil, err
	}

	ext := "pprof"
	if name == "trace" {
		ext = "trace"
	}
	filename := fmt.Sprintf("%s-%s-%d.%s", profileFilePrefix(srv.Name), name, time.Now().Unix(), ext)
	return &ProfileDump{Filename: filename, Data: data}, nil
}

// profileFilePrefix turns a server name into a safe filename fragment (keeps
// alphanumerics, dot and dash; everything else becomes '-'), so the suggested
// download name — and the Content-Disposition header built from it — can't be
// broken by an odd server name. Falls back to "agent" when nothing survives.
func profileFilePrefix(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-.")
	if out == "" {
		return "agent"
	}
	return out
}
