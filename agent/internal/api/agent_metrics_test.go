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

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ks-tool/vpn-agent/internal/metrics"
	"github.com/ks-tool/vpn-agent/models"
)

func TestAgentSeparatesSystemAndInterfaceMetrics(t *testing.T) {
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/metrics":
			_ = json.NewEncoder(w).Encode(models.MetricsSnapshot{
				System:     models.SystemSnapshot{CPUPercent: 99},
				Interfaces: []models.InterfaceSnapshot{{Interface: "awg0", Peers: []models.PeerSnapshot{{PublicKey: "peer", RxBytes: 123}}}},
			})
		case "/metrics/history":
			_ = json.NewEncoder(w).Encode(models.SystemHistory{Interfaces: []models.InterfaceHistory{{Interface: "awg0", Peers: []models.PeerHistory{{PublicKey: "peer"}}}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()

	handler, err := NewAgent(models.HostInfo{}, nil, provider.URL, metrics.NewSystemCollector(time.Minute), metrics.NewLatencyCollector(provider.URL, time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	response := request(t, http.MethodGet, server.URL+"/metrics", nil)
	var system models.SystemSnapshot
	decodeResponse(t, response, &system)
	if system.CPUPercent == 99 {
		t.Fatal("agent system metrics leaked from AWG provider")
	}

	response = request(t, http.MethodGet, server.URL+"/metrics/awg0", nil)
	var iface models.InterfaceSnapshot
	decodeResponse(t, response, &iface)
	if iface.Interface != "awg0" || len(iface.Peers) != 1 || iface.Peers[0].RxBytes != 123 {
		t.Fatalf("unexpected interface metrics: %+v", iface)
	}

	response = request(t, http.MethodGet, server.URL+"/metrics/awg0/history", nil)
	var history models.InterfaceHistory
	decodeResponse(t, response, &history)
	if history.Interface != "awg0" || len(history.Peers) != 1 {
		t.Fatalf("unexpected interface history: %+v", history)
	}

	response = request(t, http.MethodGet, server.URL+"/metrics/missing", nil)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("missing interface status = %s, want 404", response.Status)
	}
}
