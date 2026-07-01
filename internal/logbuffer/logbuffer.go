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

// Package logbuffer provides an in-memory, fixed-capacity ring buffer of log
// lines that satisfies io.Writer, so it can be teed into zerolog alongside the
// real stdout output. The desktop (Wails) app plugs it into the global logger
// at startup and exposes the captured lines through the Settings "Logs" modal
// (view + save to a JSON file). Like the agent's metrics history, the buffer
// is intentionally not persisted — it is scoped to the process lifetime and
// lost on restart.
package logbuffer

import (
	"bytes"
	"sync"
)

// Buffer is a thread-safe ring buffer of the most recent log lines. Each call
// to Write stores one entry (zerolog emits exactly one complete JSON object,
// newline-terminated, per write); once max entries are held, the oldest are
// dropped. Logs are written from many goroutines (deploy, sync, tunnel
// workers), so — unlike the single-user metadata store — this genuinely needs
// its own lock.
type Buffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

// New returns a Buffer that retains at most max entries (a non-positive max
// falls back to a sane default).
func New(max int) *Buffer {
	if max <= 0 {
		max = 1000
	}
	return &Buffer{max: max, lines: make([]string, 0, max)}
}

// Write records p as a single log entry (its trailing newline stripped) and
// always reports the whole slice as written, so it never trips zerolog's
// short-write handling.
func (b *Buffer) Write(p []byte) (int, error) {
	line := string(bytes.TrimRight(p, "\n"))

	b.mu.Lock()
	b.lines = append(b.lines, line)
	if len(b.lines) > b.max {
		b.lines = b.lines[len(b.lines)-b.max:]
	}
	b.mu.Unlock()

	return len(p), nil
}

// Lines returns a snapshot copy of the captured entries, oldest first.
func (b *Buffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()

	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}
