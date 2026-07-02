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

// The admin exposes a peer's public key base64-encoded (WireGuard's string
// form — the `pk` field after sanitizePeer swaps the private key for the
// public one), while the agent keys its per-peer metrics by the SAME public
// key hex-encoded (hex.EncodeToString). Convert to hex so the two match.
export function b64ToHex(b64: string): string {
  try {
    const bin = atob(b64);
    let hex = '';
    for (let i = 0; i < bin.length; i++) hex += bin.charCodeAt(i).toString(16).padStart(2, '0');
    return hex;
  } catch {
    return '';
  }
}

// A WireGuard peer is treated as "connected right now" if it completed a
// handshake within this window. A live peer re-handshakes roughly every ~2 min
// (on traffic, or via persistent keepalive), so 3 min is the standard heuristic
// with a small grace margin — matching how `wg`/most dashboards judge "online".
export const PEER_CONNECTED_WINDOW_MS = 3 * 60 * 1000;

// isPeerConnected reports whether a peer's last handshake is recent enough to
// count as a live connection. lastHandshake is the agent's PeerSnapshot value —
// a Go time.Time that marshals as an RFC3339 string on the wire (tygo types it
// as number, hence the cast). A peer that never handshaked carries the zero time
// ("0001-01-01…"), which parses far in the past and is correctly not connected.
export function isPeerConnected(
  lastHandshake: string | number | null | undefined,
  nowMs: number = Date.now(),
): boolean {
  if (lastHandshake == null) return false;
  const t = new Date(lastHandshake as unknown as string).getTime();
  if (!Number.isFinite(t) || t <= 0) return false;
  return nowMs - t <= PEER_CONNECTED_WINDOW_MS;
}
