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
	"fmt"
	"net"
	"strings"
	"time"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// defaultKeepaliveInterval is used when AddPeerInput.KeepaliveInterval is
// left unset (0) — AmneziaWG/WireGuard peers behind NAT need persistent
// keepalive to stay reachable, so leaving it disabled by default isn't a
// sensible default for newly created peers.
const defaultKeepaliveInterval = 10 // seconds

type AddPeerInput struct {
	Name        string
	InterfaceID uuid.UUID
	AllowedIPs  []string
	Endpoint    string
	// DNS sets the peer's client-side DNS (the wg-quick `[Interface] DNS`
	// line). Empty falls back to the interface's DNS when rendering the config.
	DNS []string
	// PrivateKey, when non-empty, is the peer's private key to use as-is
	// (base64 WireGuard key). Empty means generate a fresh one.
	PrivateKey string
	// PresharedKey, when non-empty and WithPresharedKey is false, is the PSK
	// to use as-is (base64 WireGuard key). Ignored when WithPresharedKey is
	// set (a fresh PSK is generated instead).
	PresharedKey      string
	WithPresharedKey  bool
	KeepaliveInterval int // seconds; 0 = use defaultKeepaliveInterval
}

func (s *Service) ListPeers(userID string) ([]models.Peer, error) {
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	peers, err := s.store.Users().Peers(uID).List()
	if err != nil {
		return nil, err
	}
	return sanitizePeers(peers), nil
}

func (s *Service) GetPeer(userID string, publicKey string) (*models.Peer, error) {
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	pk, err := agentmodels.ParseKey(publicKey)
	if err != nil {
		return nil, err
	}
	peer, err := s.store.Users().Peers(uID).Get(pk)
	if err != nil {
		return nil, err
	}
	sanitized := sanitizePeer(*peer)
	return &sanitized, nil
}

func (s *Service) AddPeer(userID string, in AddPeerInput) (*models.User, error) {
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	if len(in.AllowedIPs) == 0 {
		ip, err := s.nextFreeIP(in.InterfaceID)
		if err != nil {
			return nil, err
		}
		in.AllowedIPs = []string{fmt.Sprintf("%s/32", ip.String())}
	} else if err := s.validatePeerAllowedIPs(in.InterfaceID, in.AllowedIPs); err != nil {
		return nil, err
	}

	var privKey agentmodels.Key
	if k := strings.TrimSpace(in.PrivateKey); k != "" {
		privKey, err = agentmodels.ParseKey(k)
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
	} else {
		privKey, err = agentmodels.GeneratePrivateKey()
		if err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
	}

	peer := &models.Peer{Name: in.Name, PrivateKey: privKey, InterfaceId: in.InterfaceID, DNS: in.DNS}

	ifacePeer := agentmodels.InterfacePeer{
		Key:        privKey.PublicKey(),
		AllowedIPs: in.AllowedIPs,
		Endpoint:   in.Endpoint,
	}
	if in.WithPresharedKey {
		psk, err := agentmodels.GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("generate PSK: %w", err)
		}
		ifacePeer.PresharedKey = &psk
	} else if k := strings.TrimSpace(in.PresharedKey); k != "" {
		psk, err := agentmodels.ParseKey(k)
		if err != nil {
			return nil, fmt.Errorf("parse preshared key: %w", err)
		}
		ifacePeer.PresharedKey = &psk
	}
	if in.KeepaliveInterval <= 0 {
		in.KeepaliveInterval = defaultKeepaliveInterval
	}
	ifacePeer.KeepaliveInterval = time.Duration(in.KeepaliveInterval) * time.Second

	ps := s.store.Users().Peers(uID)
	if ext, ok := ps.(interface {
		SetWithIfacePeer(*models.Peer, agentmodels.InterfacePeer) error
	}); ok {
		// bbolt path: stores peer + interface peer in one transaction
		if err = ext.SetWithIfacePeer(peer, ifacePeer); err != nil {
			return nil, err
		}
	} else {
		// fallback: basic Set (no AllowedIPs on interface peer)
		if err = ps.Set(peer); err != nil {
			return nil, err
		}
	}

	s.pushInterfaceByID(in.InterfaceID)
	u, err := s.store.Users().Get(uID)
	if err != nil {
		return nil, err
	}
	sanitized := sanitizeUser(*u)
	return &sanitized, nil
}

// DeletePeer removes the peer by public key from the user and the interface.
func (s *Service) DeletePeer(userID string, key string) (*models.User, error) {
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, err
	}
	pk, err := agentmodels.ParseKey(key)
	if err != nil {
		return nil, err
	}

	peer, err := s.store.Users().Peers(uID).Get(pk)
	if err != nil {
		return nil, err
	}
	ifaceID := peer.InterfaceId

	if err = s.store.Users().Peers(uID).Delete(pk); err != nil {
		return nil, err
	}
	s.pushInterfaceByID(ifaceID)
	u, err := s.store.Users().Get(uID)
	if err != nil {
		return nil, err
	}
	sanitized := sanitizeUser(*u)
	return &sanitized, nil
}

// interfaceUsage loads the interface owning ifaceID and returns its subnet along
// with the set of host IPs already taken on it — the interface's own address and
// every existing peer's AllowedIPs. Shared by nextFreeIP (auto-assign a free
// address) and validatePeerAllowedIPs (validate an explicit one). The interface's
// own host address is included in the used set, so a peer is never handed (or
// allowed to claim) the IP the interface itself uses.
func (s *Service) interfaceUsage(ifaceID uuid.UUID) (*net.IPNet, map[string]bool, error) {
	srv, err := s.findServerByInterface(ifaceID)
	if err != nil {
		return nil, nil, err
	}
	iface, err := s.store.Servers().Interfaces(srv.ID).Get(ifaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("interface: %w", err)
	}
	hostIP, network, err := net.ParseCIDR(iface.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("interface has no valid address: %w", err)
	}

	used := map[string]bool{hostIP.String(): true}
	for _, peer := range iface.Peers {
		for _, cidr := range peer.AllowedIPs {
			if ip, _ := parseAllowedIP(cidr); ip != nil {
				used[ip.String()] = true
			}
		}
	}
	return network, used, nil
}

// parseAllowedIP parses an AllowedIPs entry, returning its address and whether
// it's a single host — a /32 (IPv4) or /128 (IPv6) CIDR, or a bare IP — rather
// than a broader route CIDR. nil address if it parses as neither.
func parseAllowedIP(s string) (net.IP, bool) {
	if ip, ipnet, err := net.ParseCIDR(s); err == nil {
		ones, bits := ipnet.Mask.Size()
		return ip, ones == bits
	}
	if ip := net.ParseIP(s); ip != nil {
		return ip, true
	}
	return nil, false
}

// validatePeerAllowedIPs rejects the two mistakes admins hit when supplying a
// peer's addresses by hand: a host address outside the interface's subnet, and a
// host address already taken by the interface itself or another peer (two peers
// sharing one IP). Only the peer's own tunnel address (a host entry) is checked;
// a broader route CIDR — a LAN behind a site-to-site peer (see the Endpoint
// field) — is legitimately outside the subnet and passes through untouched. When
// AllowedIPs is left empty, nextFreeIP picks an address that can't hit either, so
// this only runs for explicitly-supplied ones.
func (s *Service) validatePeerAllowedIPs(ifaceID uuid.UUID, allowedIPs []string) error {
	network, used, err := s.interfaceUsage(ifaceID)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, raw := range allowedIPs {
		ip, isHost := parseAllowedIP(raw)
		if ip == nil {
			return invalidInput("allowed IP %q is not a valid IP address or CIDR", raw)
		}
		if !isHost {
			continue // routed CIDR — allowed outside the subnet
		}
		if !network.Contains(ip) {
			return invalidInput("allowed IP %q is not in the interface subnet %s", ip, network.String())
		}
		key := ip.String()
		if used[key] || seen[key] {
			return invalidInput("allowed IP %q is already in use on this interface", ip)
		}
		seen[key] = true
	}
	return nil
}

// nextFreeIP picks the first host address in the interface's subnet that isn't
// already used by the interface itself or one of its peers, for auto-assigning
// AllowedIPs when the caller of AddPeer left it empty.
func (s *Service) nextFreeIP(ifaceID uuid.UUID) (net.IP, error) {
	network, used, err := s.interfaceUsage(ifaceID)
	if err != nil {
		return nil, err
	}

	networkIP := network.IP.Mask(network.Mask)
	broadcast := broadcastIP(network)
	// On a /31 (RFC 3021 point-to-point) or /32 there's no network/broadcast
	// address to reserve — every address is usable — so only skip the ends on
	// larger subnets.
	ones, bits := network.Mask.Size()
	skipEnds := bits-ones >= 2
	for ip := cloneIP(networkIP); network.Contains(ip); incIP(ip) {
		if skipEnds && (ip.Equal(networkIP) || ip.Equal(broadcast)) {
			continue
		}
		if !used[ip.String()] {
			return ip, nil
		}
	}
	return nil, fmt.Errorf("no free IP addresses available on interface subnet %s", network.String())
}

func cloneIP(ip net.IP) net.IP {
	out := make(net.IP, len(ip))
	copy(out, ip)
	return out
}

func incIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

func broadcastIP(network *net.IPNet) net.IP {
	broadcast := cloneIP(network.IP.Mask(network.Mask))
	for i := range broadcast {
		broadcast[i] |= ^network.Mask[i]
	}
	return broadcast
}

// pushInterfaceByID re-pushes the full config of the interface owning a
// just-added/removed peer — the agent has no separate peers endpoint, so
// any peer change requires resending the whole InterfaceConfig (see
// agent/models.InterfaceConfig.Peers). Best-effort: the peer change in
// storage already succeeded by the time this runs.
func (s *Service) pushInterfaceByID(ifaceID uuid.UUID) {
	srv, err := s.findServerByInterface(ifaceID)
	if err != nil {
		log.Warn().Err(err).Str("interface_id", ifaceID.String()).Msg("push interface: failed to find owning server")
		return
	}
	iface, err := s.store.Servers().Interfaces(srv.ID).Get(ifaceID)
	if err != nil {
		log.Warn().Err(err).Str("interface_id", ifaceID.String()).Msg("push interface: failed to load interface")
		return
	}
	s.pushInterface(srv.ID, iface)
}
