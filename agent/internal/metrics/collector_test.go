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
	"path/filepath"
	"testing"
	"time"
)

// These tests exercise the collector <-> ring buffer store wiring directly
// (write/query), without going through collectSystem/collectPeers (which
// read /proc and call wgctrl — Linux-only, irrelevant to this part).

func newTestCollector(t *testing.T) *Collector {
	t.Helper()
	return NewCollector(nil, nil, time.Minute)
}

func TestCollectorWriteAndQuerySystemMetrics(t *testing.T) {
	c := newTestCollector(t)
	now := time.Now()

	const usedBytes, totalBytes = 1024 * 1024 * 1024, 2048 * 1024 * 1024

	c.store.write(systemDB, metricCPUPercent, now, 42.5)
	c.store.write(systemDB, metricMemUsedBytes, now, usedBytes)
	c.store.write(systemDB, metricMemTotalBytes, now, totalBytes)

	snap := c.Snapshot()
	if snap.System.CPUPercent != 42.5 {
		t.Errorf("CPUPercent = %v, want 42.5", snap.System.CPUPercent)
	}
	if snap.System.MemUsedBytes != usedBytes {
		t.Errorf("MemUsedBytes = %v, want %v", snap.System.MemUsedBytes, usedBytes)
	}
	if snap.System.MemTotalBytes != totalBytes {
		t.Errorf("MemTotalBytes = %v, want %v", snap.System.MemTotalBytes, totalBytes)
	}
}

func TestCollectorWriteAndQueryPeerMetrics(t *testing.T) {
	c := newTestCollector(t)
	now := time.Now()

	const iface = "wg0"
	const key = "deadbeef"
	const rxBytes, txBytes = 1000 * 1024 * 1024, 2000 * 1024 * 1024
	handshake := now.Add(-30 * time.Second).Unix()

	c.store.write(iface, key+"/rx", now, rxBytes)
	c.store.write(iface, key+"/tx", now, txBytes)
	c.store.write(iface, key+"/handshake", now, float64(handshake))

	snap := c.Snapshot()
	if len(snap.Interfaces) != 1 {
		t.Fatalf("Interfaces = %d, want 1: %+v", len(snap.Interfaces), snap.Interfaces)
	}
	got := snap.Interfaces[0]
	if got.Interface != iface {
		t.Errorf("Interface = %q, want %q", got.Interface, iface)
	}
	if len(got.Peers) != 1 {
		t.Fatalf("Peers = %d, want 1: %+v", len(got.Peers), got.Peers)
	}
	peer := got.Peers[0]
	if peer.PublicKey != key {
		t.Errorf("PublicKey = %q, want %q", peer.PublicKey, key)
	}
	if peer.RxBytes != rxBytes {
		t.Errorf("RxBytes = %d, want %d", peer.RxBytes, rxBytes)
	}
	if peer.TxBytes != txBytes {
		t.Errorf("TxBytes = %d, want %d", peer.TxBytes, txBytes)
	}
	if peer.LastHandshake.Unix() != handshake {
		t.Errorf("LastHandshake = %v, want unix %d", peer.LastHandshake, handshake)
	}
}

func TestCollectorPeersHistory(t *testing.T) {
	c := newTestCollector(t)
	base := time.Now()

	const iface = "wg0"
	// Two peers, three ticks each, written the way collectPeers does (rx/tx/
	// handshake together per tick). "aaaa" < "bbbb" so the sorted output order
	// is deterministic.
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Minute)
		c.store.write(iface, "bbbb/rx", ts, float64(100+i))
		c.store.write(iface, "bbbb/tx", ts, float64(200+i))
		c.store.write(iface, "bbbb/handshake", ts, float64(ts.Unix()))
		c.store.write(iface, "aaaa/rx", ts, float64(10+i))
		c.store.write(iface, "aaaa/tx", ts, float64(20+i))
		c.store.write(iface, "aaaa/handshake", ts, float64(ts.Unix()))
	}

	hist := c.SystemHistory()
	if len(hist.Interfaces) != 1 {
		t.Fatalf("Interfaces = %d, want 1: %+v", len(hist.Interfaces), hist.Interfaces)
	}
	ih := hist.Interfaces[0]
	if ih.Interface != iface {
		t.Errorf("Interface = %q, want %q", ih.Interface, iface)
	}
	if len(ih.Peers) != 2 {
		t.Fatalf("Peers = %d, want 2: %+v", len(ih.Peers), ih.Peers)
	}
	// Sorted by public key: aaaa first.
	if ih.Peers[0].PublicKey != "aaaa" || ih.Peers[1].PublicKey != "bbbb" {
		t.Fatalf("peer order = [%q %q], want [aaaa bbbb]", ih.Peers[0].PublicKey, ih.Peers[1].PublicKey)
	}

	aaaa := ih.Peers[0]
	if len(aaaa.Points) != 3 {
		t.Fatalf("aaaa points = %d, want 3", len(aaaa.Points))
	}
	for i, p := range aaaa.Points {
		if p.RxBytes != uint64(10+i) || p.TxBytes != uint64(20+i) {
			t.Errorf("aaaa point %d = rx %d/tx %d, want rx %d/tx %d", i, p.RxBytes, p.TxBytes, 10+i, 20+i)
		}
		wantTS := base.Add(time.Duration(i) * time.Minute)
		if p.LastHandshake.Unix() != wantTS.Unix() {
			t.Errorf("aaaa point %d handshake = %d, want %d", i, p.LastHandshake.Unix(), wantTS.Unix())
		}
	}
}

// TestCollectorPeersHistoryExcludesSystem confirms the host-level "system"
// database never leaks into the per-peer Interfaces of the history response.
func TestCollectorPeersHistoryExcludesSystem(t *testing.T) {
	c := newTestCollector(t)
	c.store.write(systemDB, metricCPUPercent, time.Now(), 50)

	if hist := c.SystemHistory(); len(hist.Interfaces) != 0 {
		t.Fatalf("expected no interfaces, got %+v", hist.Interfaces)
	}
}

func TestCollectorPersistRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFilename)
	now := time.Now()

	c1 := NewCollector(nil, nil, time.Minute)
	c1.SetPersistence(path)
	c1.store.write(systemDB, metricCPUPercent, now, 55.5)
	c1.store.write(systemDB, metricNetRxBytes, now, 1000)
	c1.store.write("wg0", "peerkey/rx", now, 4242)
	c1.store.write("wg0", "peerkey/tx", now, 2424)
	c1.store.write("wg0", "peerkey/handshake", now, float64(now.Unix()))

	if err := c1.SaveHistory(); err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}

	// A fresh collector must recover the samples from the file alone.
	c2 := NewCollector(nil, nil, time.Minute)
	c2.SetPersistence(path)
	if err := c2.LoadHistory(); err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}

	snap := c2.Snapshot()
	if snap.System.CPUPercent != 55.5 {
		t.Errorf("CPUPercent = %v, want 55.5", snap.System.CPUPercent)
	}
	if len(snap.Interfaces) != 1 || len(snap.Interfaces[0].Peers) != 1 {
		t.Fatalf("interfaces/peers = %+v", snap.Interfaces)
	}
	if peer := snap.Interfaces[0].Peers[0]; peer.RxBytes != 4242 || peer.TxBytes != 2424 {
		t.Errorf("peer rx/tx = %d/%d, want 4242/2424", peer.RxBytes, peer.TxBytes)
	}
	if pts := c2.SystemHistory().Points; len(pts) != 1 {
		t.Errorf("history points = %d, want 1", len(pts))
	}
	if ph := c2.SystemHistory().Interfaces; len(ph) != 1 || len(ph[0].Peers) != 1 || len(ph[0].Peers[0].Points) != 1 {
		t.Errorf("per-peer history not restored: %+v", ph)
	}
}

func TestCollectorLoadHistoryDropsStale(t *testing.T) {
	path := filepath.Join(t.TempDir(), HistoryFilename)

	c1 := NewCollector(nil, nil, time.Minute)
	c1.SetPersistence(path)
	stale := time.Now().Add(-retentionPeriod - time.Hour)
	fresh := time.Now().Add(-time.Minute)
	c1.store.write(systemDB, metricNetRxBytes, stale, 1)
	c1.store.write(systemDB, metricNetRxBytes, fresh, 2)
	if err := c1.SaveHistory(); err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}

	c2 := NewCollector(nil, nil, time.Minute)
	c2.SetPersistence(path)
	if err := c2.LoadHistory(); err != nil {
		t.Fatalf("LoadHistory: %v", err)
	}

	series := c2.store.series(systemDB, metricNetRxBytes)
	if len(series) != 1 {
		t.Fatalf("series = %d, want 1 (stale sample past retention dropped): %+v", len(series), series)
	}
	if series[0].value != 2 {
		t.Errorf("kept value = %v, want 2 (the fresh sample)", series[0].value)
	}
}

func TestCollectorPersistenceDisabledIsNoOp(t *testing.T) {
	c := newTestCollector(t) // no SetPersistence
	if err := c.SaveHistory(); err != nil {
		t.Errorf("SaveHistory with persistence off: %v", err)
	}
	if err := c.LoadHistory(); err != nil {
		t.Errorf("LoadHistory with persistence off: %v", err)
	}
}

func TestCollectorLoadHistoryMissingFileOK(t *testing.T) {
	c := newTestCollector(t)
	c.SetPersistence(filepath.Join(t.TempDir(), "does-not-exist"))
	if err := c.LoadHistory(); err != nil {
		t.Errorf("LoadHistory on missing file should be a no-op, got: %v", err)
	}
}

func TestSaveHistoryEvictsDeadPeers(t *testing.T) {
	c := newTestCollector(t)
	c.SetPersistence(filepath.Join(t.TempDir(), HistoryFilename))

	stale := time.Now().Add(-retentionPeriod - time.Hour) // a removed peer's last sample
	fresh := time.Now().Add(-time.Minute)

	c.store.write(systemDB, metricCPUPercent, fresh, 42)
	c.store.write("wg0", "livepeer/rx", fresh, 100) // still being sampled
	c.store.write("wg0", "deadpeer/rx", stale, 1)   // removed peer, only stale

	// SaveHistory prunes past the retention window before snapshotting.
	if err := c.SaveHistory(); err != nil {
		t.Fatalf("SaveHistory: %v", err)
	}

	if _, ok := c.store.last("wg0", "deadpeer/rx"); ok {
		t.Errorf("dead peer series was not evicted")
	}
	if _, ok := c.store.last("wg0", "livepeer/rx"); !ok {
		t.Errorf("live peer series was wrongly evicted")
	}
	if _, ok := c.store.last(systemDB, metricCPUPercent); !ok {
		t.Errorf("system series was wrongly evicted")
	}
}

func TestStorePruneRemovesEmptyDatabase(t *testing.T) {
	c := newTestCollector(t)
	c.store.write("wg9", "deadpeer/rx", time.Now().Add(-retentionPeriod-time.Hour), 1)

	c.store.prune(time.Now().Add(-retentionPeriod))

	for _, db := range c.store.databases() {
		if db == "wg9" {
			t.Fatalf("database with only dead peers was not removed: %v", c.store.databases())
		}
	}
}

func TestStorePruneDropsStaleKeepsFresh(t *testing.T) {
	c := newTestCollector(t)
	stale := time.Now().Add(-retentionPeriod - time.Hour)
	fresh := time.Now().Add(-time.Minute)
	c.store.write(systemDB, metricNetRxBytes, stale, 1)
	c.store.write(systemDB, metricNetRxBytes, fresh, 2)

	c.store.prune(time.Now().Add(-retentionPeriod))

	if series := c.store.series(systemDB, metricNetRxBytes); len(series) != 1 || series[0].value != 2 {
		t.Fatalf("series = %+v, want a single fresh sample (value 2)", series)
	}
}

func TestForgetInterfaceDropsPeersFromSnapshotAndHistory(t *testing.T) {
	c := newTestCollector(t)
	now := time.Now()
	for _, iface := range []string{"wg0", "wg1"} {
		c.store.write(iface, "deadbeef/rx", now, 1)
		c.store.write(iface, "deadbeef/tx", now, 2)
		c.store.write(iface, "deadbeef/handshake", now, float64(now.Unix()))
	}

	c.ForgetInterface("wg0")

	snap := c.Snapshot()
	if len(snap.Interfaces) != 1 || snap.Interfaces[0].Interface != "wg1" {
		t.Fatalf("after ForgetInterface(wg0), snapshot Interfaces = %+v, want only wg1", snap.Interfaces)
	}
	for _, ih := range c.SystemHistory().Interfaces {
		if ih.Interface == "wg0" {
			t.Fatalf("wg0 still present in history after ForgetInterface: %+v", c.SystemHistory().Interfaces)
		}
	}
}

func TestStoreRetainOnlyDropsMissingInterfaces(t *testing.T) {
	s := newStore(10)
	now := time.Now()
	s.write(systemDB, metricCPUPercent, now, 1)
	s.write("wg0", "deadbeef/rx", now, 1)
	s.write("wg1", "deadbeef/rx", now, 1)

	// Keep only systemDB + wg0 — as collectPeers does with the live interface set.
	s.retainOnly(map[string]struct{}{systemDB: {}, "wg0": {}})

	got := make(map[string]bool)
	for _, db := range s.databases() {
		got[db] = true
	}
	if !got[systemDB] || !got["wg0"] {
		t.Fatalf("retainOnly dropped a kept database: %v", s.databases())
	}
	if got["wg1"] {
		t.Fatalf("retainOnly kept wg1, which was not in the keep set: %v", s.databases())
	}
}

func TestDeltaUint64(t *testing.T) {
	if got := deltaUint64(100, 150); got != 50 {
		t.Errorf("deltaUint64(100, 150) = %d, want 50", got)
	}
	if got := deltaUint64(100, 100); got != 0 {
		t.Errorf("deltaUint64(100, 100) = %d, want 0", got)
	}
	// cur < prev means the counter went backwards (interface recreated),
	// not a negative amount of traffic — must clamp to 0, not underflow.
	if got := deltaUint64(150, 100); got != 0 {
		t.Errorf("deltaUint64(150, 100) = %d, want 0 (clamped)", got)
	}
}

func TestCollectorSnapshotEmpty(t *testing.T) {
	c := newTestCollector(t)
	snap := c.Snapshot()
	if len(snap.Interfaces) != 0 {
		t.Fatalf("expected no interfaces, got %+v", snap.Interfaces)
	}
}

// TestRingBufferRetention confirms old samples are dropped once the buffer
// fills, rather than growing without bound.
func TestRingBufferRetention(t *testing.T) {
	rb := newRingBuffer(3)
	base := time.Now()
	for i := 0; i < 5; i++ {
		rb.add(base.Add(time.Duration(i)*time.Second), float64(i))
	}

	series := rb.series()
	if len(series) != 3 {
		t.Fatalf("series length = %d, want 3", len(series))
	}
	// Only the last 3 writes (values 2, 3, 4) should have survived.
	for i, want := range []float64{2, 3, 4} {
		if series[i].value != want {
			t.Errorf("series[%d] = %v, want %v", i, series[i].value, want)
		}
	}

	last, ok := rb.last()
	if !ok || last.value != 4 {
		t.Errorf("last() = %v, %v; want 4, true", last.value, ok)
	}
}
