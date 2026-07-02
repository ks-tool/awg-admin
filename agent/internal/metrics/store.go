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
// than cap*interval worth of history. The buffers live in memory; the
// collector snapshots the whole store to a single JSON file so history
// survives an agent restart (see Collector.SaveHistory / export / restore),
// rather than running a full on-disk TSDB (see the package doc comment).
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

// pruneBefore drops every sample older than cutoff and reports whether the
// buffer is now empty (its newest sample was itself older than cutoff — e.g. a
// peer that was removed and has had no new samples for the whole retention
// window). Called only when snapshotting (see store.prune), so the rebuild
// cost is irrelevant.
func (r *ringBuffer) pruneBefore(cutoff time.Time) (empty bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.size == 0 {
		return true
	}
	kept := make([]point, 0, r.size)
	start := (r.next - r.size + len(r.buf)) % len(r.buf)
	for i := 0; i < r.size; i++ {
		if p := r.buf[(start+i)%len(r.buf)]; !p.ts.Before(cutoff) {
			kept = append(kept, p)
		}
	}
	for i := range r.buf {
		r.buf[i] = point{}
	}
	copy(r.buf, kept)
	r.size = len(kept)
	r.next = r.size % len(r.buf)
	return r.size == 0
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

// dropDB removes a whole database (all its metrics and their series) at once —
// used to forget a deleted interface's peer metrics immediately instead of
// waiting for prune to age each series past the retention window. No-op for a
// database that was never recorded.
func (s *store) dropDB(db string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.dbs, db)
}

// retainOnly drops every database whose name is not in keep. The collector runs
// it each tick with the set of currently-existing interfaces (plus systemDB), so
// metrics for an interface that no longer exists get evicted regardless of how
// they got there — a delete whose dropDB was lost to an ungraceful restart (the
// on-disk checkpoint is reloaded into memory on start), or a sample that raced
// the delete handler and re-created the database. This bounds such staleness to
// one collection interval instead of the full retention window.
func (s *store) retainOnly(keep map[string]struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for db := range s.dbs {
		if _, ok := keep[db]; !ok {
			delete(s.dbs, db)
		}
	}
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

// persistedPoint / persistedStore are the on-disk JSON shape of the whole
// store, used to carry the ring buffers across an agent restart (see
// Collector.SaveHistory / LoadHistory).
type persistedPoint struct {
	TS    time.Time `json:"ts"`
	Value float64   `json:"value"`
}

type persistedStore struct {
	DBs map[string]map[string][]persistedPoint `json:"dbs"`
}

// export snapshots every retained sample, grouped by database then metric
// (oldest first). It goes through the same locked accessors reads use, so it
// never holds a lock across the whole store; a sample written concurrently may
// or may not be included, which is fine for monitoring data.
func (s *store) export() persistedStore {
	out := persistedStore{DBs: make(map[string]map[string][]persistedPoint)}
	for _, db := range s.databases() {
		metrics := make(map[string][]persistedPoint)
		for _, name := range s.metricNames(db) {
			series := s.series(db, name)
			pts := make([]persistedPoint, len(series))
			for i, p := range series {
				pts[i] = persistedPoint{TS: p.ts, Value: p.value}
			}
			metrics[name] = pts
		}
		if len(metrics) > 0 {
			out.DBs[db] = metrics
		}
	}
	return out
}

// restore replays persisted samples back into the ring buffers, skipping any
// older than cutoff (so a long-stopped agent doesn't reload history past the
// retention window). Samples go through the normal oldest-first write path, so
// an over-capacity series keeps only its newest entries.
func (s *store) restore(data persistedStore, cutoff time.Time) {
	for db, metrics := range data.DBs {
		for name, pts := range metrics {
			for _, p := range pts {
				if p.TS.Before(cutoff) {
					continue
				}
				s.write(db, name, p.TS, p.Value)
			}
		}
	}
}

// prune drops samples older than cutoff and removes any metric (and any then
// empty database) whose buffer is left empty. This is what actually evicts the
// series of a removed peer — the collector stops writing to it, so within one
// retention window its newest sample ages past cutoff and it's dropped here,
// instead of lingering in memory (and every snapshot) forever. Run from
// Collector.SaveHistory before each snapshot.
func (s *store) prune(cutoff time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for db, metrics := range s.dbs {
		for name, rb := range metrics {
			if rb.pruneBefore(cutoff) {
				delete(metrics, name)
			}
		}
		if len(metrics) == 0 {
			delete(s.dbs, db)
		}
	}
}
