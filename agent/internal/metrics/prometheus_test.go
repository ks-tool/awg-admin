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
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/ks-tool/awg-admin/agent/models"
)

func TestWritePrometheus(t *testing.T) {
	snap := models.MetricsSnapshot{
		System: models.SystemSnapshot{
			Timestamp:  time.Unix(1700000000, 0),
			CPUPercent: 12.5,
			Load1:      0.5,
		},
		Interfaces: []models.InterfaceSnapshot{
			{
				Interface: "wg0",
				Peers: []models.PeerSnapshot{
					{PublicKey: "deadbeef", RxBytes: 1024, TxBytes: 2048, LastHandshake: time.Unix(1699999000, 0)},
				},
			},
		},
	}

	var buf bytes.Buffer
	if err := WritePrometheus(&buf, snap); err != nil {
		t.Fatalf("WritePrometheus: %v", err)
	}
	out := buf.String()

	for _, want := range []string{
		"# TYPE awg_agent_cpu_percent gauge",
		"awg_agent_cpu_percent 12.5 1700000000000",
		`awg_agent_peer_rx_bytes{interface="wg0",pubkey="deadbeef"} 1024`,
		`awg_agent_peer_tx_bytes{interface="wg0",pubkey="deadbeef"} 2048`,
		`awg_agent_peer_last_handshake_seconds{interface="wg0",pubkey="deadbeef"} 1699999000`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}
}
