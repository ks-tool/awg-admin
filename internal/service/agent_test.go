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
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"testing"

	"github.com/ks-tool/awg-admin/internal/agentclient"
)

// TestTunnelDropped pins down which failures callAgent treats as a dead tunnel
// worth reconnecting for. The retryable cases mirror the exact shape of the
// real reported errors: agentclient wraps the http.Client error (a *url.Error
// whose inner error is io.EOF from the dead SSH mux, or context.DeadlineExceeded
// when the request hung on a silently-dead socket) with fmt.Errorf("%w").
func TestTunnelDropped(t *testing.T) {
	reportedEOF := fmt.Errorf("PUT /interfaces: %w", &url.Error{
		Op:  "Put",
		URL: "http://127.0.0.1:8080/interfaces",
		Err: io.EOF,
	})
	reportedTimeout := fmt.Errorf("GET /interfaces/: %w", &url.Error{
		Op:  "Get",
		URL: "http://127.0.0.1:8080/interfaces/",
		Err: context.DeadlineExceeded,
	})

	retryable := []struct {
		name string
		err  error
	}{
		{"reported wrapped EOF", reportedEOF},
		{"bare EOF", io.EOF},
		{"unexpected EOF", io.ErrUnexpectedEOF},
		{"closed conn", fmt.Errorf("dial: %w", net.ErrClosed)},
		{"reported wrapped deadline", reportedTimeout},
		{"bare context deadline", fmt.Errorf("PUT /interfaces: %w", context.DeadlineExceeded)},
	}
	for _, tc := range retryable {
		if !tunnelDropped(tc.err) {
			t.Errorf("%s: tunnelDropped = false, want true", tc.name)
		}
	}

	notRetryable := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"agent 404", &agentclient.NotFoundError{Interface: "awg1"}},
		{"agent 4xx/5xx body", errors.New("agent returned 400 Bad Request: bad config")},
	}
	for _, tc := range notRetryable {
		if tunnelDropped(tc.err) {
			t.Errorf("%s: tunnelDropped = true, want false", tc.name)
		}
	}
}
