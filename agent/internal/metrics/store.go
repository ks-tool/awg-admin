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

package metrics

import (
	"sync"
	"time"
)

// point is one (timestamp, value) sample.
type point struct {
	ts    time.Time
	value float64
}

// ringBuffer holds up to cap samples for one metric, oldest overwritten
// first — that's the entire retention mechanism: once full, writing a new
// sample implicitly drops the oldest one, so the buffer never holds more
// than cap*interval worth of history. Not persisted to disk: an agent
// restart starts history over, which is an acceptable trade-off for
// monitoring data sampled every 30-60s (see store_test.go and the package
// doc comment for the full reasoning).
type ringBuffer struct {
	mu   sync.Mutex
	buf  []point
	next int // index the next write lands on
	size int // number of valid entries (<= len(buf))
}

func newRingBuffer(capacity int) *ringBuffer {
	return &ringBuffer{buf: make([]point, capacity)}
}

func (r *ringBuffer) add(ts time.Time, value float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.next] = point{ts: ts, value: value}
	r.next = (r.next + 1) % len(r.buf)
	if r.size < len(r.buf) {
		r.size++
	}
}

// last returns the most recently added sample, if any.
func (r *ringBuffer) last() (point, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size == 0 {
		return point{}, false
	}
	idx := r.next - 1
	if idx < 0 {
		idx = len(r.buf) - 1
	}
	return r.buf[idx], true
}

// series returns every retained sample, oldest first.
func (r *ringBuffer) series() []point {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]point, r.size)
	start := (r.next - r.size + len(r.buf)) % len(r.buf)
	for i := 0; i < r.size; i++ {
		out[i] = r.buf[(start+i)%len(r.buf)]
	}
	return out
}

// store is a registry of ring buffers grouped by database (the "system"
// host-stats group, or an interface name for its peers' stats) and metric
// name within that database — mirrors the db/metric two-level naming
// collector.go uses, without needing an actual TSDB engine for it.
type store struct {
	capacity int

	mu  sync.Mutex
	dbs map[string]map[string]*ringBuffer
}

func newStore(capacity int) *store {
	return &store{capacity: capacity, dbs: make(map[string]map[string]*ringBuffer)}
}

func (s *store) buffer(db, metric string) *ringBuffer {
	s.mu.Lock()
	defer s.mu.Unlock()

	metrics, ok := s.dbs[db]
	if !ok {
		metrics = make(map[string]*ringBuffer)
		s.dbs[db] = metrics
	}
	rb, ok := metrics[metric]
	if !ok {
		rb = newRingBuffer(s.capacity)
		metrics[metric] = rb
	}
	return rb
}

func (s *store) write(db, metric string, ts time.Time, value float64) {
	s.buffer(db, metric).add(ts, value)
}

func (s *store) last(db, metric string) (point, bool) {
	s.mu.Lock()
	metrics, ok := s.dbs[db]
	if ok {
		var rb *ringBuffer
		rb, ok = metrics[metric]
		s.mu.Unlock()
		if ok {
			return rb.last()
		}
		return point{}, false
	}
	s.mu.Unlock()
	return point{}, false
}

// series returns every retained sample for db/metric, oldest first, or nil
// if nothing has been recorded for it yet.
func (s *store) series(db, metric string) []point {
	s.mu.Lock()
	metrics, ok := s.dbs[db]
	if ok {
		var rb *ringBuffer
		rb, ok = metrics[metric]
		s.mu.Unlock()
		if ok {
			return rb.series()
		}
		return nil
	}
	s.mu.Unlock()
	return nil
}

// databases returns every database name with at least one metric.
func (s *store) databases() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.dbs))
	for db := range s.dbs {
		out = append(out, db)
	}
	return out
}

// metricNames returns every metric name recorded under db.
func (s *store) metricNames(db string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	metrics, ok := s.dbs[db]
	if !ok {
		return nil
	}
	out := make([]string, 0, len(metrics))
	for name := range metrics {
		out = append(out, name)
	}
	return out
}
