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
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Jipok/wgctrl-go/wgtypes"
	"github.com/ks-tool/vpn-agent/models"
)

func TestLatencyCollectorMeasuresPeerAndCalculatesP95(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(models.MetricsSnapshot{Interfaces: []models.InterfaceSnapshot{{
			Interface: "awg0",
			Peers:     []models.PeerSnapshot{{PublicKey: "peer", ProbeAddress: "10.0.0.2"}},
		}}})
	}))
	defer provider.Close()

	collector := NewLatencyCollector(provider.URL, time.Minute)
	collector.probe = func(_ context.Context, iface, address string) (time.Duration, error) {
		if iface != "awg0" || address != "10.0.0.2" {
			t.Fatalf("unexpected probe %s/%s", iface, address)
		}
		return 42 * time.Millisecond, nil
	}
	collector.collectOnce(context.Background())

	snapshot := models.InterfaceSnapshot{Interface: "awg0", Peers: []models.PeerSnapshot{{PublicKey: "peer"}}}
	collector.Apply(&snapshot)
	if snapshot.Peers[0].LatencyP95Ms == nil || *snapshot.Peers[0].LatencyP95Ms != 42 {
		t.Fatalf("latency p95 = %v, want 42", snapshot.Peers[0].LatencyP95Ms)
	}
}

func TestLatencyPercentileAndRetention(t *testing.T) {
	now := time.Now()
	samples := make([]latencySample, 20)
	for i := range samples {
		samples[i] = latencySample{ts: now, ms: float64(i + 1)}
	}
	if got := latencyPercentile(samples, 95); got != 19 {
		t.Fatalf("p95 = %v, want 19", got)
	}
	samples[0].ts = now.Add(-latencyWindow - time.Second)
	if got := pruneLatencySamples(samples, now.Add(-latencyWindow)); len(got) != 19 {
		t.Fatalf("retained samples = %d, want 19", len(got))
	}
}

func TestTunnelProbeAddressSelectsMostSpecificIPv4(t *testing.T) {
	_, defaultRoute, _ := net.ParseCIDR("0.0.0.0/0")
	_, clientSubnet, _ := net.ParseCIDR("10.20.0.0/24")
	_, peerAddress, _ := net.ParseCIDR("10.0.0.2/32")
	peer := wgtypes.Peer{AllowedIPs: []net.IPNet{*defaultRoute, *clientSubnet, *peerAddress}}
	if got := tunnelProbeAddress(peer); got != "10.0.0.2" {
		t.Fatalf("probe address = %q, want 10.0.0.2", got)
	}
}
