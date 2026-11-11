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

// TunnelMetric is the source-to-destination p95 round-trip latency for one
// topology connection. A nil LatencyP95Ms means that no successful ICMP sample
// is available yet; Error describes a per-source collection failure.
type TunnelMetric struct {
	ConnectionID string   `json:"connectionId"`
	LatencyP95Ms *float64 `json:"latencyP95Ms,omitempty"`
	Error        string   `json:"error,omitempty"`
}
