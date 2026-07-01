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

package models

import "time"

// These types are the JSON wire format of GET /metrics (see
// agent/internal/metrics and agent/internal/api's "metrics" handler) — kept
// here rather than in agent/internal/metrics so the root module's
// internal/agentclient can decode them without crossing the agent module's
// internal/ package boundary (Go disallows importing another module's
// internal/... packages even when, as with internal/metrics, the docs of a
// vendored dependency might suggest otherwise — see internal/metrics's
// package doc comment for the story that prompted this split).

// SystemSnapshot is the latest recorded host-level sample.
type SystemSnapshot struct {
	Timestamp     time.Time `json:"timestamp"`
	CPUPercent    float64   `json:"cpuPercent"`
	MemUsedBytes  uint64    `json:"memUsedBytes"`
	MemTotalBytes uint64    `json:"memTotalBytes"`
	Load1         float64   `json:"load1"`
	Load5         float64   `json:"load5"`
	Load15        float64   `json:"load15"`
	// NetRxBytes/NetTxBytes are bytes transferred *during this one
	// collection interval* (e.g. the last 45s, see
	// AWG_AGENT_METRICS_INTERVAL), not the host's cumulative since-boot
	// counters from /proc/net/dev — those only ever climb and aren't
	// useful for "how much traffic is this server actually moving" (see
	// agent/internal/metrics/collector.go's collectSystem).
	NetRxBytes uint64 `json:"netRxBytes"`
	NetTxBytes uint64 `json:"netTxBytes"`
}

// PeerSnapshot is the latest recorded sample for one peer of one interface.
type PeerSnapshot struct {
	PublicKey     string    `json:"publicKey"`
	RxBytes       uint64    `json:"rx"`
	TxBytes       uint64    `json:"tx"`
	LastHandshake time.Time `json:"lastHandshake"`
}

// InterfaceSnapshot groups peer snapshots under their interface name.
type InterfaceSnapshot struct {
	Interface string         `json:"interface"`
	Peers     []PeerSnapshot `json:"peers"`
}

// MetricsSnapshot is the full GET /metrics response: the latest known
// value of every metric the agent has collected.
type MetricsSnapshot struct {
	System     SystemSnapshot      `json:"system"`
	Interfaces []InterfaceSnapshot `json:"interfaces"`
}

// SystemHistoryPoint is one retained host-level sample, aligned across
// metrics by timestamp (see Collector.SystemHistory). Like SystemSnapshot,
// NetRxBytes/NetTxBytes are per-interval deltas, not cumulative counters.
type SystemHistoryPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	CPUPercent    float64   `json:"cpuPercent"`
	MemUsedBytes  uint64    `json:"memUsedBytes"`
	MemTotalBytes uint64    `json:"memTotalBytes"`
	NetRxBytes    uint64    `json:"netRxBytes"`
	NetTxBytes    uint64    `json:"netTxBytes"`
}

// SystemHistory is the full GET /metrics/history response: every host-level
// sample (Points) plus every per-peer sample (Interfaces) still retained in
// the agent's in-memory ring buffers (up to 48h, oldest first — see
// agent/internal/metrics's package doc comment), for rendering charts instead
// of just the latest value. History is served only through this one endpoint;
// the per-peer series ride along here rather than on a separate route. Points
// stays the top-level field it always was, so existing consumers that only
// read host history are unaffected.
type SystemHistory struct {
	Points     []SystemHistoryPoint `json:"points"`
	Interfaces []InterfaceHistory   `json:"interfaces"`
}

// PeerHistoryPoint is one retained sample for a single peer. Unlike
// SystemHistoryPoint's per-interval network deltas, RxBytes/TxBytes are the
// cumulative byte counters WireGuard reports for the peer (same as
// PeerSnapshot); differencing consecutive points yields per-interval traffic.
// LastHandshake is the peer's last successful handshake at that sample (zero
// value if it had never completed one yet).
type PeerHistoryPoint struct {
	Timestamp     time.Time `json:"timestamp"`
	RxBytes       uint64    `json:"rx"`
	TxBytes       uint64    `json:"tx"`
	LastHandshake time.Time `json:"lastHandshake"`
}

// PeerHistory is every retained sample for one peer, oldest first.
type PeerHistory struct {
	PublicKey string             `json:"publicKey"`
	Points    []PeerHistoryPoint `json:"points"`
}

// InterfaceHistory groups per-peer history under their interface name, as
// carried in SystemHistory.Interfaces.
type InterfaceHistory struct {
	Interface string        `json:"interface"`
	Peers     []PeerHistory `json:"peers"`
}
