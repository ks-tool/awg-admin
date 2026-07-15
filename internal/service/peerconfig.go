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
	"encoding/base64"
	"fmt"
	"net"
	"strconv"
	"strings"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"

	"github.com/google/uuid"
	"github.com/skip2/go-qrcode"
)

// GetPeerConfig renders a wg-quick style client config for the peer
// identified by key, for download or QR-code provisioning on the frontend.
// The agent only knows InterfaceConfig/InterfacePeer (server-side shapes);
// this assembles the client-side equivalent from the peer's own private
// key plus its owning interface's server public key, listen port and
// AmneziaWG obfuscation params (if any).
func (s *Service) GetPeerConfig(userID string, key string) (string, error) {
	debugOp("GetPeerConfig").Str("user_id", userID).Str("public_key", key).Msg("rendering peer config")
	uID, err := uuid.Parse(userID)
	if err != nil {
		return "", err
	}
	pk, err := agentmodels.ParseKey(key)
	if err != nil {
		return "", err
	}

	peer, err := s.store.Users().Peers(uID).Get(pk)
	if err != nil {
		return "", err
	}

	srv, err := s.findServerByInterface(peer.InterfaceId)
	if err != nil {
		return "", err
	}
	iface, err := s.store.Servers().Interfaces(srv.ID).Get(peer.InterfaceId)
	if err != nil {
		return "", err
	}

	pubKey := peer.PrivateKey.PublicKey()
	var ifacePeer *agentmodels.InterfacePeer
	for i := range iface.Peers {
		if iface.Peers[i].Key == pubKey {
			ifacePeer = &iface.Peers[i]
			break
		}
	}
	if ifacePeer == nil {
		return "", fmt.Errorf("peer not found on interface %s", iface.Interface)
	}

	var b strings.Builder
	b.WriteString("[Interface]\n")
	_, _ = fmt.Fprintf(&b, "PrivateKey = %s\n", peer.PrivateKey.String())
	if len(ifacePeer.AllowedIPs) > 0 {
		_, _ = fmt.Fprintf(&b, "Address = %s\n", strings.Join(ifacePeer.AllowedIPs, ", "))
	}
	// A per-peer DNS overrides the interface's; fall back to the interface's
	// when the peer doesn't set one.
	dns := peer.DNS
	if len(dns) == 0 {
		dns = iface.DNS
	}
	if len(dns) > 0 {
		_, _ = fmt.Fprintf(&b, "DNS = %s\n", strings.Join(dns, ", "))
	}
	writeAmneziaParams(&b, iface.InterfaceConfig)

	b.WriteString("\n[Peer]\n")
	ifacePub := iface.PrivateKey.PublicKey()
	_, _ = fmt.Fprintf(&b, "PublicKey = %s\n", ifacePub.String())
	if ifacePeer.PresharedKey != nil {
		_, _ = fmt.Fprintf(&b, "PresharedKey = %s\n", ifacePeer.PresharedKey.String())
	}
	// Full-tunnel by default — the server-side AllowedIPs on InterfacePeer is
	// what the server routes *to* this peer (its own IP), not what the
	// client should route *through* the tunnel. IPv4-only: this network is
	// IPv4-only, so routing ::/0 through it would just break the client's
	// IPv6 connectivity for no benefit.
	b.WriteString("AllowedIPs = 0.0.0.0/0\n")
	// Only emit an Endpoint when the interface actually has a listen port. A
	// port-0 interface (it binds a random ephemeral port and only dials out —
	// e.g. was a tunnel exit) has no stable inbound port, so "Endpoint = host:0"
	// would be invalid; omit the line rather than hand the client an unusable
	// endpoint to fill in by hand.
	if host := srv.SSH.Host; len(host) > 0 && iface.ListenPort != 0 {
		_, _ = fmt.Fprintf(&b, "Endpoint = %s\n", net.JoinHostPort(host, strconv.Itoa(int(iface.ListenPort))))
	}
	if ifacePeer.KeepaliveInterval > 0 {
		_, _ = fmt.Fprintf(&b, "PersistentKeepalive = %d\n", int(ifacePeer.KeepaliveInterval.Seconds()))
	}

	return b.String(), nil
}

// qrCodeSize is the side length, in pixels, of the PNG QR code rendered by
// GetPeerQRCode — large enough to stay scannable by a phone camera held at
// a normal viewing distance from a laptop/monitor screen.
const qrCodeSize = 384

// GetPeerQRCode renders key's client config (see GetPeerConfig) as a PNG QR
// code, base64-encoded so it can travel as plain JSON/Wails-bound string
// data and be dropped straight into an <img src="data:image/png;base64,...">
// on the frontend. Generating the image here — rather than handing the raw
// config text to the frontend and rendering the QR code client-side with a
// JS library — means the wg-quick text (which embeds the peer's private
// key) only ever needs to leave the process as pixels, not as a string the
// browser has to hold onto and could leak via devtools/extensions/clipboard
// history.
func (s *Service) GetPeerQRCode(userID string, key string) (string, error) {
	debugOp("GetPeerQRCode").Str("user_id", userID).Str("public_key", key).Msg("rendering peer QR code")
	cfg, err := s.GetPeerConfig(userID, key)
	if err != nil {
		return "", err
	}

	png, err := qrcode.Encode(cfg, qrcode.Medium, qrCodeSize)
	if err != nil {
		return "", fmt.Errorf("encode QR code: %w", err)
	}
	return base64.StdEncoding.EncodeToString(png), nil
}

// writeAmneziaParams appends cfg's AmneziaWG obfuscation parameters (only
// the ones that were actually set) so generated client configs keep working
// against an AmneziaWG server instead of falling back to plain WireGuard.
func writeAmneziaParams(b *strings.Builder, cfg agentmodels.InterfaceConfig) {
	writeInt := func(name string, v *int) {
		if v != nil {
			_, _ = fmt.Fprintf(b, "%s = %d\n", name, *v)
		}
	}
	writeStr := func(name string, v *string) {
		if v != nil {
			_, _ = fmt.Fprintf(b, "%s = %s\n", name, *v)
		}
	}
	writeInt("Jc", cfg.Jc)
	writeInt("Jmin", cfg.Jmin)
	writeInt("Jmax", cfg.Jmax)
	writeInt("S1", cfg.S1)
	writeInt("S2", cfg.S2)
	writeInt("S3", cfg.S3)
	writeInt("S4", cfg.S4)
	writeStr("H1", cfg.H1)
	writeStr("H2", cfg.H2)
	writeStr("H3", cfg.H3)
	writeStr("H4", cfg.H4)
	writeStr("I1", cfg.I1)
	writeStr("I2", cfg.I2)
	writeStr("I3", cfg.I3)
	writeStr("I4", cfg.I4)
	writeStr("I5", cfg.I5)
}
