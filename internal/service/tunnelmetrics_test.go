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

package service

import (
	"encoding/hex"
	"testing"

	agentmodels "github.com/ks-tool/vpn-agent/v2/models"
)

func TestGetTunnelMetricsMatchesDestinationPublicKey(t *testing.T) {
	svc := newTestService(t)
	sourceServer, sourceAgent := newLinkTestServer(t, svc, "a")
	targetServer, _ := newLinkTestServer(t, svc, "b")
	source, err := svc.CreateInterface(sourceServer.ID.String(), agentmodels.InterfaceConfig{Interface: "awg0", Address: "10.0.0.1/24", ListenPort: 51820})
	if err != nil {
		t.Fatal(err)
	}
	target, err := svc.CreateInterface(targetServer.ID.String(), agentmodels.InterfaceConfig{Interface: "awg0", Address: "10.0.0.2/24", ListenPort: 51820})
	if err != nil {
		t.Fatal(err)
	}
	connection, err := svc.CreateNetworkConnection(source.ID.String(), target.ID.String())
	if err != nil {
		t.Fatal(err)
	}
	p95 := 87.5
	targetKey := target.PrivateKey.PublicKey()
	sourceAgent.mu.Lock()
	sourceAgent.metrics[source.Interface] = agentmodels.InterfaceSnapshot{
		Interface: source.Interface,
		Peers:     []agentmodels.PeerSnapshot{{PublicKey: hex.EncodeToString(targetKey[:]), LatencyP95Ms: &p95}},
	}
	sourceAgent.mu.Unlock()

	metrics, err := svc.GetTunnelMetrics()
	if err != nil {
		t.Fatal(err)
	}
	if len(metrics) != 1 || metrics[0].ConnectionID != connection.ID.String() {
		t.Fatalf("unexpected tunnel metrics: %+v", metrics)
	}
	if metrics[0].LatencyP95Ms == nil || *metrics[0].LatencyP95Ms != p95 {
		t.Fatalf("latency p95 = %v, want %v", metrics[0].LatencyP95Ms, p95)
	}
}
