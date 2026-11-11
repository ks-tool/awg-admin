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
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ks-tool/vpn-agent/models"
)

const (
	latencyWindow       = 15 * time.Minute
	latencyProbeTimeout = 2 * time.Second
	latencyConcurrency  = 8
)

type latencySample struct {
	ts time.Time
	ms float64
}

type LatencyCollector struct {
	providerURL string
	interval    time.Duration
	client      *http.Client
	probe       func(context.Context, string, string) (time.Duration, error)
	enabled     atomic.Bool

	mu     sync.RWMutex
	series map[string]map[string][]latencySample
}

func NewLatencyCollector(providerURL string, interval time.Duration) *LatencyCollector {
	c := &LatencyCollector{
		providerURL: strings.TrimRight(providerURL, "/"),
		interval:    interval,
		client:      &http.Client{Timeout: 5 * time.Second},
		probe:       probeLatency,
		series:      make(map[string]map[string][]latencySample),
	}
	c.enabled.Store(true)
	return c
}

func (c *LatencyCollector) SetEnabled(enabled bool) { c.enabled.Store(enabled) }

func (c *LatencyCollector) Run(ctx context.Context) {
	if len(c.providerURL) == 0 {
		return
	}
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()
	c.collectOnce(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.collectOnce(ctx)
		}
	}
}

func (c *LatencyCollector) collectOnce(ctx context.Context) {
	if !c.enabled.Load() || len(c.providerURL) == 0 {
		return
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.providerURL+"/metrics", nil)
	if err != nil {
		return
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return
	}
	var snapshot models.MetricsSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snapshot); err != nil {
		return
	}

	sem := make(chan struct{}, latencyConcurrency)
	var wg sync.WaitGroup
	for _, iface := range snapshot.Interfaces {
		for _, peer := range iface.Peers {
			if len(peer.ProbeAddress) == 0 {
				continue
			}
			wg.Add(1)
			go func(iface, key, address string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				probeCtx, cancel := context.WithTimeout(ctx, latencyProbeTimeout)
				defer cancel()
				rtt, err := c.probe(probeCtx, iface, address)
				if err != nil {
					return
				}
				c.record(iface, key, time.Now(), float64(rtt)/float64(time.Millisecond))
			}(iface.Interface, peer.PublicKey, peer.ProbeAddress)
		}
	}
	wg.Wait()
	c.retain(snapshot.Interfaces)
}

func (c *LatencyCollector) retain(interfaces []models.InterfaceSnapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for iface, peers := range c.series {
		found := false
		for _, current := range interfaces {
			if current.Interface != iface {
				continue
			}
			found = true
			live := make(map[string]struct{}, len(current.Peers))
			for _, peer := range current.Peers {
				live[peer.PublicKey] = struct{}{}
			}
			for key := range peers {
				if _, ok := live[key]; !ok {
					delete(peers, key)
				}
			}
			break
		}
		if !found || len(peers) == 0 {
			delete(c.series, iface)
		}
	}
}

func (c *LatencyCollector) record(iface, key string, ts time.Time, ms float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	peers := c.series[iface]
	if peers == nil {
		peers = make(map[string][]latencySample)
		c.series[iface] = peers
	}
	series := pruneLatencySamples(peers[key], ts.Add(-latencyWindow))
	peers[key] = append(series, latencySample{ts: ts, ms: ms})
}

func (c *LatencyCollector) Apply(snapshot *models.InterfaceSnapshot) {
	cutoff := time.Now().Add(-latencyWindow)
	c.mu.Lock()
	defer c.mu.Unlock()
	peers := c.series[snapshot.Interface]
	for i := range snapshot.Peers {
		peer := &snapshot.Peers[i]
		series := pruneLatencySamples(peers[peer.PublicKey], cutoff)
		if len(series) == 0 {
			peer.LatencyP95Ms = nil
			continue
		}
		peers[peer.PublicKey] = series
		value := latencyPercentile(series, 95)
		peer.LatencyP95Ms = &value
	}
}

func pruneLatencySamples(samples []latencySample, cutoff time.Time) []latencySample {
	first := 0
	for first < len(samples) && samples[first].ts.Before(cutoff) {
		first++
	}
	return samples[first:]
}

func latencyPercentile(samples []latencySample, percentile float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	values := make([]float64, len(samples))
	for i, sample := range samples {
		values[i] = sample.ms
	}
	sort.Float64s(values)
	rank := int(math.Ceil(percentile / 100 * float64(len(values))))
	if rank < 1 {
		rank = 1
	}
	return values[rank-1]
}
