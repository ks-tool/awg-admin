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

package logbuffer

import (
	"fmt"
	"sync"
	"testing"
)

func TestWriteReportsFullLengthAndStripsNewline(t *testing.T) {
	b := New(10)
	p := []byte("hello\n")
	n, err := b.Write(p)
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	// Must report the whole slice as written, or zerolog treats it as a short
	// write and errors.
	if n != len(p) {
		t.Fatalf("Write reported %d bytes, want %d", n, len(p))
	}
	lines := b.Lines()
	if len(lines) != 1 || lines[0] != "hello" {
		t.Fatalf("Lines = %#v, want [hello]", lines)
	}
}

func TestRingDropsOldest(t *testing.T) {
	b := New(3)
	for i := 0; i < 5; i++ {
		if _, err := fmt.Fprintf(b, "line-%d\n", i); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}
	got := b.Lines()
	want := []string{"line-2", "line-3", "line-4"}
	if len(got) != len(want) {
		t.Fatalf("Lines len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Lines = %v, want %v", got, want)
		}
	}
}

func TestLinesReturnsSnapshotCopy(t *testing.T) {
	b := New(10)
	_, _ = b.Write([]byte("a\n"))
	snap := b.Lines()
	// Mutating the returned slice must not affect the buffer's own state.
	snap[0] = "mutated"
	if again := b.Lines(); again[0] != "a" {
		t.Fatalf("Lines snapshot was not a copy: %v", again)
	}
}

func TestConcurrentWritesAreRaceFree(t *testing.T) {
	b := New(100)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				_, _ = fmt.Fprintf(b, "w%d-%d\n", id, j)
			}
		}(i)
	}
	wg.Wait()
	// Buffer is capped, so it must never exceed max regardless of interleaving.
	if got := len(b.Lines()); got != 100 {
		t.Fatalf("Lines len = %d, want 100", got)
	}
}
