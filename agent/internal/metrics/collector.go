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

// Package metrics periodically samples host (CPU/RAM/load/network) and
// WireGuard peer (rx/tx/last handshake) stats and keeps the last 48h of
// them in an in-process ring buffer per metric (see store.go) — no
// external TSDB. A third-party embeddable engine (github.com/aymanhs/
// nanotdb) was tried first, but its storage engine lives under its own
// internal/ import path and Go's internal-package visibility rule blocks
// using it as a library from outside that module, despite its own
// EMBEDDING.md describing exactly that. Vendoring that engine's source was
// possible but pulled in WAL/crash-recovery/rollup/compaction machinery
// this agent doesn't need just to keep 48h of 30-60s samples; a plain ring
// buffer covers the actual requirement in a few hundred lines with no
// third-party code or extra dependencies. To survive an agent restart, the
// whole store is checkpointed to a single JSON file under the agent's data
// directory (see Collector.SaveHistory / LoadHistory) — periodically and on
// graceful shutdown — and reloaded on startup, still without a full on-disk
// TSDB.
//
// Samples are exposed read-only over HTTP via Snapshot (see
// agent/internal/api's "/metrics" handler), as JSON by default or
// Prometheus text exposition format with "?fmt=prom" (see prometheus.go).
package metrics

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/agent/storage"

	"github.com/Jipok/wgctrl-go"
	atomicfile "github.com/natefinch/atomic"
	"github.com/rs/zerolog/log"
)

const (
	systemDB = "system"

	metricCPUPercent    = "cpu_percent"
	metricMemUsedBytes  = "mem_used_bytes"
	metricMemTotalBytes = "mem_total_bytes"
	metricLoad1         = "load1"
	metricLoad5         = "load5"
	metricLoad15        = "load15"
	metricNetRxBytes    = "net_rx_bytes"
	metricNetTxBytes    = "net_tx_bytes"
)

// retentionPeriod is how much history each metric's ring buffer covers.
const retentionPeriod = 48 * time.Hour

// HistoryFilename is the default name of the metrics-history checkpoint the
// agent writes under its data directory (AWG_AGENT_DB). It deliberately has no
// ".json" extension so agent/storage/fs (which treats every *.json file in
// that directory as an interface config) and its file watcher both ignore it.
const HistoryFilename = "metrics-history"

// persistInterval is how often Run checkpoints the store to disk while
// running, on top of the flush on graceful shutdown. Bounds how much history a
// hard crash can lose without re-writing the whole file every sample; clamped
// up to the sampling interval when that is larger.
const persistInterval = 5 * time.Minute

// Collector periodically samples host and peer metrics into an in-memory
// ring buffer per metric and serves the latest values back out via
// Snapshot.
type Collector struct {
	store *store
	awg   *wgctrl.Client
	agent storage.Storage

	interval time.Duration
	enabled  atomic.Bool

	// persistPath is where the store is checkpointed to survive restarts;
	// empty keeps the old in-memory-only behavior.
	persistPath string

	mu      sync.Mutex
	prevCPU cpuTotals
	haveCPU bool
	prevNet netStats
	haveNet bool
}

// NewCollector returns a Collector that samples every interval (the task
// this was built for calls for 30-60s) and retains retentionPeriod (48h) of
// history per metric, sized from those two numbers. Persistence is opt-in via
// SetPersistence (off by default); see the package doc comment.
func NewCollector(awg *wgctrl.Client, agentStore storage.Storage, interval time.Duration) *Collector {
	capacity := int(retentionPeriod/interval) + 1
	c := &Collector{store: newStore(capacity), awg: awg, agent: agentStore, interval: interval}
	c.enabled.Store(true)
	return c
}

// SetEnabled turns sampling on/off at runtime (awg-admin's "disable
// monitoring" toggle, see internal/agentclient.SetMetricsEnabled). Disabling
// stops new samples from being collected; Snapshot keeps serving whatever
// was last recorded rather than going blank. The enabled flag itself is not
// persisted across agent restarts — sampling always resumes enabled (unlike
// the sample history, which is checkpointed to disk; see SaveHistory).
func (c *Collector) SetEnabled(enabled bool) { c.enabled.Store(enabled) }

func (c *Collector) Enabled() bool { return c.enabled.Load() }

// Run samples metrics every c.interval until ctx is cancelled. Errors from
// a single collection cycle are logged and don't stop the loop — a
// transient /proc read failure or unreachable wgctrl device shouldn't take
// monitoring down entirely. When persistence is enabled (see SetPersistence),
// it also checkpoints the store to disk periodically and once more when ctx is
// cancelled, so a restart resumes with its history intact.
func (c *Collector) Run(ctx context.Context) {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	// A no-op tick channel when persistence is off, so the select below stays
	// the same either way.
	var persistC <-chan time.Time
	if c.persistPath != "" {
		every := persistInterval
		if c.interval > every {
			every = c.interval
		}
		pt := time.NewTicker(every)
		defer pt.Stop()
		persistC = pt.C
	}

	c.collectOnce()
	for {
		select {
		case <-ctx.Done():
			c.persist("shutdown")
			return
		case <-ticker.C:
			c.collectOnce()
		case <-persistC:
			c.persist("checkpoint")
		}
	}
}

// SetPersistence enables checkpointing the metrics history to path so it
// survives an agent restart (reloaded by LoadHistory). An empty path (the
// default) keeps the old in-memory-only behavior. Call before Run.
func (c *Collector) SetPersistence(path string) { c.persistPath = path }

// persist writes the current history to disk, logging (but not failing on) an
// error — a full or read-only data directory must not take monitoring down.
func (c *Collector) persist(reason string) {
	if c.persistPath == "" {
		return
	}
	if err := c.SaveHistory(); err != nil {
		log.Warn().Err(err).Str("reason", reason).Msg("failed to persist metrics history")
	}
}

// SaveHistory writes the current metrics history to the configured path via an
// atomic replace, so a crash mid-write can't corrupt it. No-op when
// persistence is disabled.
//
// It first prunes samples past the retention window, which also evicts the
// series of any removed peer (the collector stops writing to it, so its newest
// sample eventually ages out) — keeping both memory and the on-disk snapshot
// from growing without bound on a server whose peers churn.
func (c *Collector) SaveHistory() error {
	if c.persistPath == "" {
		return nil
	}
	c.store.prune(time.Now().Add(-retentionPeriod))
	buf, err := json.Marshal(c.store.export())
	if err != nil {
		return fmt.Errorf("marshal metrics history: %w", err)
	}
	return atomicfile.WriteFile(c.persistPath, bytes.NewReader(buf))
}

// LoadHistory restores previously persisted samples from the configured path,
// dropping any older than the retention window. A missing file (first run) is
// not an error. No-op when persistence is disabled. Call before Run.
func (c *Collector) LoadHistory() error {
	if c.persistPath == "" {
		return nil
	}
	buf, err := os.ReadFile(c.persistPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("read metrics history: %w", err)
	}

	var data persistedStore
	if err := json.Unmarshal(buf, &data); err != nil {
		return fmt.Errorf("unmarshal metrics history: %w", err)
	}
	c.store.restore(data, time.Now().Add(-retentionPeriod))
	return nil
}

func (c *Collector) collectOnce() {
	if !c.Enabled() {
		return
	}

	now := time.Now()
	if err := c.collectSystem(now); err != nil {
		log.Warn().Err(err).Msg("failed to collect system metrics")
	}
	c.collectPeers(now)
}

func (c *Collector) collectSystem(now time.Time) error {
	cpu, err := readCPUTotals()
	if err != nil {
		return fmt.Errorf("read cpu stats: %w", err)
	}

	c.mu.Lock()
	prev, haveCPU := c.prevCPU, c.haveCPU
	c.prevCPU, c.haveCPU = cpu, true
	c.mu.Unlock()

	if haveCPU {
		c.store.write(systemDB, metricCPUPercent, now, cpuPercent(prev, cpu))
	}

	mem, err := readMemStats()
	if err != nil {
		return fmt.Errorf("read mem stats: %w", err)
	}
	c.store.write(systemDB, metricMemUsedBytes, now, float64(mem.usedBytes))
	c.store.write(systemDB, metricMemTotalBytes, now, float64(mem.totalBytes))

	la, err := readLoadAvg()
	if err != nil {
		return fmt.Errorf("read load average: %w", err)
	}
	c.store.write(systemDB, metricLoad1, now, la.load1)
	c.store.write(systemDB, metricLoad5, now, la.load5)
	c.store.write(systemDB, metricLoad15, now, la.load15)

	net, err := readNetStats()
	if err != nil {
		return fmt.Errorf("read net stats: %w", err)
	}

	c.mu.Lock()
	prevNet, haveNet := c.prevNet, c.haveNet
	c.prevNet, c.haveNet = net, true
	c.mu.Unlock()

	// /proc/net/dev's counters are cumulative since the interface came up,
	// not since the last sample — charting them as-is just shows an
	// ever-climbing line that's only useful read as "total since boot",
	// not as actual network load. Store the delta against the previous
	// sample instead, same as CPU above: bytes transferred *during this
	// collection interval*, which is what's actually useful for traffic
	// accounting. The very first sample has no previous reading to diff
	// against, so it's recorded as 0 (consistent with every other
	// per-tick metric always having an entry — see SystemHistory's
	// index-zipped points) rather than skipped outright.
	var rxDelta, txDelta uint64
	if haveNet {
		rxDelta = deltaUint64(prevNet.rxBytes, net.rxBytes)
		txDelta = deltaUint64(prevNet.txBytes, net.txBytes)
	}
	c.store.write(systemDB, metricNetRxBytes, now, float64(rxDelta))
	c.store.write(systemDB, metricNetTxBytes, now, float64(txDelta))
	return nil
}

// deltaUint64 returns cur-prev, clamped to 0 when cur < prev — the counter
// went backwards (interface recreated/driver reload resetting it to 0),
// not an actual negative amount of traffic.
func deltaUint64(prev, cur uint64) uint64 {
	if cur < prev {
		return 0
	}
	return cur - prev
}

// collectPeers samples rx/tx/last-handshake for every peer of every
// interface this agent manages (agent/cmd/dump.go is the same idea as a
// one-shot CLI). Each interface gets its own "database" within the store,
// named after the interface.
func (c *Collector) collectPeers(now time.Time) {
	ifaces, err := c.agent.List()
	if err != nil {
		log.Warn().Err(err).Msg("failed to list interfaces for metrics")
		return
	}

	// Reconcile the store to the interfaces that actually exist: evict any
	// database (interface) that's no longer present, so a deleted interface's
	// peers can't linger in /metrics or /metrics/history — whether resurrected
	// from the on-disk checkpoint after an ungraceful restart, or re-created by
	// a sample that raced the delete handler's ForgetInterface. systemDB holds
	// host stats (not an interface), so it's always kept.
	keep := make(map[string]struct{}, len(ifaces)+1)
	keep[systemDB] = struct{}{}
	for _, cfg := range ifaces {
		keep[cfg.Interface] = struct{}{}
	}
	c.store.retainOnly(keep)

	for _, cfg := range ifaces {
		dev, err := c.awg.Device(cfg.Interface)
		if err != nil {
			log.Warn().Err(err).Str("interface", cfg.Interface).Msg("failed to read wireguard device for metrics")
			continue
		}

		db := cfg.Interface
		livePeers := make(map[string]struct{}, len(dev.Peers))
		for _, peer := range dev.Peers {
			key := hex.EncodeToString(peer.PublicKey[:])
			livePeers[key] = struct{}{}

			c.store.write(db, key+"/rx", now, float64(peer.ReceiveBytes))
			c.store.write(db, key+"/tx", now, float64(peer.TransmitBytes))

			handshake := peer.LastHandshakeTime.Unix()
			if handshake < 0 {
				handshake = 0
			}
			c.store.write(db, key+"/handshake", now, float64(handshake))
		}
		// Evict metrics for peers no longer on the device (deleted) so they stop
		// showing in the peer metrics/history, instead of lingering until they
		// age past the retention window. Backstop to the immediate drop in the
		// apply path (RetainPeers); also covers peers removed directly on the box.
		c.store.retainPeers(db, livePeers)
	}
}

// RetainPeers evicts an interface's peer metrics for every peer whose hex public
// key is not in keep — called from the apply path when an interface's config is
// (re)pushed with a reduced peer set (e.g. a peer was deleted), so the removed
// peer disappears from /metrics and /metrics/history immediately instead of at
// the next collection tick. keep is the hex-encoded public keys of the peers
// that should remain. No-op if the interface has no recorded metrics.
func (c *Collector) RetainPeers(iface string, keep map[string]struct{}) {
	c.store.retainPeers(iface, keep)
}

// ForgetInterface immediately drops all retained peer metrics for the named
// interface — collectPeers stores each interface's peers under a database named
// after it (see db := cfg.Interface), so dropping that database evicts them from
// both /metrics and /metrics/history at once. Called when an interface is
// deleted so its peers stop showing in the peer metrics right away, instead of
// lingering until they age past the retention window. No-op if the interface has
// no recorded metrics.
func (c *Collector) ForgetInterface(name string) {
	c.store.dropDB(name)
}

// Snapshot returns the most recently recorded value of every metric this
// agent has collected. The wire types (models.MetricsSnapshot and friends)
// live in agent/models rather than here so the root module's
// internal/agentclient can decode them without crossing this package's
// internal/ boundary — see the package doc comment.
func (c *Collector) Snapshot() models.MetricsSnapshot {
	return models.MetricsSnapshot{
		System:     c.querySystem(),
		Interfaces: c.queryInterfaces(),
	}
}

func (c *Collector) querySystem() models.SystemSnapshot {
	var snap models.SystemSnapshot

	if p, ok := c.store.last(systemDB, metricCPUPercent); ok {
		snap.CPUPercent, snap.Timestamp = p.value, p.ts
	}
	if p, ok := c.store.last(systemDB, metricMemUsedBytes); ok {
		snap.MemUsedBytes, snap.Timestamp = uint64(p.value), p.ts
	}
	if p, ok := c.store.last(systemDB, metricMemTotalBytes); ok {
		snap.MemTotalBytes = uint64(p.value)
	}
	if p, ok := c.store.last(systemDB, metricLoad1); ok {
		snap.Load1 = p.value
	}
	if p, ok := c.store.last(systemDB, metricLoad5); ok {
		snap.Load5 = p.value
	}
	if p, ok := c.store.last(systemDB, metricLoad15); ok {
		snap.Load15 = p.value
	}
	if p, ok := c.store.last(systemDB, metricNetRxBytes); ok {
		snap.NetRxBytes = uint64(p.value)
	}
	if p, ok := c.store.last(systemDB, metricNetTxBytes); ok {
		snap.NetTxBytes = uint64(p.value)
	}
	return snap
}

// SystemHistory returns every host-level sample still retained in the ring
// buffers (up to 48h, oldest first), for charting instead of just the
// latest value (see Snapshot). NetRxBytes/NetTxBytes/MemUsedBytes/
// MemTotalBytes are written unconditionally on every collectSystem tick, so
// their series always have the same length/order and are zipped by index;
// CPUPercent is skipped on the very first tick (no previous sample to diff
// against yet — see collectSystem), so it's looked up by exact timestamp
// instead and left at zero for any point it's missing.
func (c *Collector) SystemHistory() models.SystemHistory {
	rx := c.store.series(systemDB, metricNetRxBytes)
	tx := c.store.series(systemDB, metricNetTxBytes)
	memUsed := c.store.series(systemDB, metricMemUsedBytes)
	memTotal := c.store.series(systemDB, metricMemTotalBytes)
	cpu := c.store.series(systemDB, metricCPUPercent)

	cpuByTS := make(map[int64]float64, len(cpu))
	for _, p := range cpu {
		cpuByTS[p.ts.UnixNano()] = p.value
	}

	points := make([]models.SystemHistoryPoint, len(rx))
	for i, p := range rx {
		hp := models.SystemHistoryPoint{
			Timestamp:  p.ts,
			NetRxBytes: uint64(p.value),
			CPUPercent: cpuByTS[p.ts.UnixNano()],
		}
		if i < len(tx) {
			hp.NetTxBytes = uint64(tx[i].value)
		}
		if i < len(memUsed) {
			hp.MemUsedBytes = uint64(memUsed[i].value)
		}
		if i < len(memTotal) {
			hp.MemTotalBytes = uint64(memTotal[i].value)
		}
		points[i] = hp
	}
	return models.SystemHistory{Points: points, Interfaces: c.interfaceHistories()}
}

// interfaceHistories returns every retained per-peer sample still in the ring
// buffers (up to 48h, oldest first), grouped by interface and peer, for
// inclusion in the SystemHistory (/metrics/history) response. A peer's
// rx/tx/handshake series are all written together on every collectPeers tick,
// so they share length/order and are zipped by index.
func (c *Collector) interfaceHistories() []models.InterfaceHistory {
	var out []models.InterfaceHistory
	for _, db := range c.store.databases() {
		if db == systemDB {
			continue
		}
		out = append(out, c.peerHistoryFor(db))
	}
	return out
}

func (c *Collector) peerHistoryFor(db string) models.InterfaceHistory {
	// The store keys peer metrics as "<publicKey>/<field>" (see collectPeers);
	// recover the distinct peer keys, sorted so the output order is stable.
	keySet := make(map[string]struct{})
	for _, name := range c.store.metricNames(db) {
		if key, _, ok := strings.Cut(name, "/"); ok {
			keySet[key] = struct{}{}
		}
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	snap := models.InterfaceHistory{Interface: db}
	for _, key := range keys {
		rx := c.store.series(db, key+"/rx")
		tx := c.store.series(db, key+"/tx")
		handshake := c.store.series(db, key+"/handshake")

		points := make([]models.PeerHistoryPoint, len(rx))
		for i, p := range rx {
			hp := models.PeerHistoryPoint{Timestamp: p.ts, RxBytes: uint64(p.value)}
			if i < len(tx) {
				hp.TxBytes = uint64(tx[i].value)
			}
			if i < len(handshake) {
				hp.LastHandshake = time.Unix(int64(handshake[i].value), 0)
			}
			points[i] = hp
		}
		snap.Peers = append(snap.Peers, models.PeerHistory{PublicKey: key, Points: points})
	}
	return snap
}

func (c *Collector) queryInterfaces() []models.InterfaceSnapshot {
	var out []models.InterfaceSnapshot
	for _, db := range c.store.databases() {
		if db == systemDB {
			continue
		}
		out = append(out, c.queryInterface(db))
	}
	return out
}

func (c *Collector) queryInterface(db string) models.InterfaceSnapshot {
	snap := models.InterfaceSnapshot{Interface: db}

	peers := make(map[string]*models.PeerSnapshot)
	peerOf := func(key string) *models.PeerSnapshot {
		if p, ok := peers[key]; ok {
			return p
		}
		p := &models.PeerSnapshot{PublicKey: key}
		peers[key] = p
		return p
	}

	for _, name := range c.store.metricNames(db) {
		key, field, ok := strings.Cut(name, "/")
		if !ok {
			continue
		}
		p, ok := c.store.last(db, name)
		if !ok {
			continue
		}

		peer := peerOf(key)
		switch field {
		case "rx":
			peer.RxBytes = uint64(p.value)
		case "tx":
			peer.TxBytes = uint64(p.value)
		case "handshake":
			peer.LastHandshake = time.Unix(int64(p.value), 0)
		}
	}

	for _, p := range peers {
		snap.Peers = append(snap.Peers, *p)
	}
	return snap
}
