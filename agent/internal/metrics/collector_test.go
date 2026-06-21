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
