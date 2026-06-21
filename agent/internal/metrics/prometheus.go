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
	"fmt"
	"io"
	"strconv"

	"github.com/ks-tool/awg-admin/agent/models"
)

// WritePrometheus renders snap in Prometheus text exposition format, for
// agents that want to scrape this agent directly rather than go through
// awg-admin's JSON-consuming Dashboard (see api/handlers.go's "?fmt=prom").
func WritePrometheus(w io.Writer, snap models.MetricsSnapshot) error {
	ts := snap.System.Timestamp.UnixMilli()

	type metric struct {
		name, help, typ string
		value           float64
	}
	system := []metric{
		{"awg_agent_cpu_percent", "Host CPU utilization percent.", "gauge", snap.System.CPUPercent},
		{"awg_agent_mem_used_bytes", "Host memory used, in bytes.", "gauge", float64(snap.System.MemUsedBytes)},
		{"awg_agent_mem_total_bytes", "Host memory total, in bytes.", "gauge", float64(snap.System.MemTotalBytes)},
		{"awg_agent_load1", "Host load average over 1 minute.", "gauge", snap.System.Load1},
		{"awg_agent_load5", "Host load average over 5 minutes.", "gauge", snap.System.Load5},
		{"awg_agent_load15", "Host load average over 15 minutes.", "gauge", snap.System.Load15},
		{"awg_agent_net_rx_bytes", "Host network bytes received (all interfaces except loopback).", "counter", float64(snap.System.NetRxBytes)},
		{"awg_agent_net_tx_bytes", "Host network bytes transmitted (all interfaces except loopback).", "counter", float64(snap.System.NetTxBytes)},
	}

	for _, m := range system {
		if err := writeMetric(w, m.name, m.help, m.typ, "", m.value, ts); err != nil {
			return err
		}
	}

	peerMetrics := []struct{ name, help, typ, field string }{
		{"awg_agent_peer_rx_bytes", "Bytes received from a WireGuard peer.", "counter", "rx"},
		{"awg_agent_peer_tx_bytes", "Bytes transmitted to a WireGuard peer.", "counter", "tx"},
		{"awg_agent_peer_last_handshake_seconds", "Unix timestamp of the last handshake with a WireGuard peer.", "gauge", "handshake"},
	}
	for _, pm := range peerMetrics {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n", pm.name, pm.help, pm.name, pm.typ); err != nil {
			return err
		}
		for _, iface := range snap.Interfaces {
			for _, peer := range iface.Peers {
				var value float64
				switch pm.field {
				case "rx":
					value = float64(peer.RxBytes)
				case "tx":
					value = float64(peer.TxBytes)
				case "handshake":
					value = float64(peer.LastHandshake.Unix())
				}
				labels := fmt.Sprintf(`{interface=%q,pubkey=%q}`, iface.Interface, peer.PublicKey)
				if _, err := fmt.Fprintf(w, "%s%s %s %d\n", pm.name, labels, formatFloat(value), ts); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func writeMetric(w io.Writer, name, help, typ, labels string, value float64, ts int64) error {
	_, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s%s %s %d\n", name, help, name, typ, name, labels, formatFloat(value), ts)
	return err
}

// formatFloat avoids scientific notation, which most Prometheus exposition
// parsers reject for plain sample values.
func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
