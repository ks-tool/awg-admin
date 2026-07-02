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

// SetPeerDisabled activates or deactivates a peer, mirroring the interface-level
// Disabled toggle. A deactivated peer keeps its stored config (keys, address,
// PSK, keepalive) but is dropped from the InterfaceConfig pushed to the agent
// (ToAmneziaConfig omits it), so it's removed from the live WireGuard device and
// can't connect until reactivated. Both the user's models.Peer and the owning
// interface's InterfacePeer carry the flag; both are updated in one storage
// write, then the interface is re-pushed. Returns the updated (sanitized) user.
func (s *Service) SetPeerDisabled(userID, publicKey string, disabled bool) (*models.User, error) {
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, invalidInput("invalid user id %q", userID)
	}
	pk, err := agentmodels.ParseKey(publicKey)
	if err != nil {
		return nil, invalidInput("invalid peer public key")
	}

	peer, err := s.store.Users().Peers(uID).Get(pk)
	if err != nil {
		return nil, err
	}
	ifaceID := peer.InterfaceId

	// The InterfacePeer (AllowedIPs/PSK/keepalive/endpoint) lives on the owning
	// interface's config — read it so only Disabled changes and the rest carries
	// over unchanged through the upsert.
	iface, _, err := s.getInterfaceByID(ifaceID)
	if err != nil {
		return nil, fmt.Errorf("load interface: %w", err)
	}
	var ifacePeer *agentmodels.InterfacePeer
	for i := range iface.Peers {
		if iface.Peers[i].Key == pk {
			cp := iface.Peers[i]
			ifacePeer = &cp
			break
		}
	}
	if ifacePeer == nil {
		return nil, invalidInput("peer not found on its interface")
	}

	peer.Disabled = disabled
	ifacePeer.Disabled = disabled

	ps := s.store.Users().Peers(uID)
	if ext, ok := ps.(interface {
		SetWithIfacePeer(*models.Peer, agentmodels.InterfacePeer) error
	}); ok {
		if err = ext.SetWithIfacePeer(peer, *ifacePeer); err != nil {
			return nil, err
		}
	} else if err = ps.Set(peer); err != nil {
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

// MigratePeer moves a peer to a different interface, preserving its keys, name,
// DNS and preshared key. Its address is kept when it's still inside the target
// interface's subnet and free there (e.g. moving between a tunnel's members,
// which share an address space); otherwise a free host address on the target is
// auto-assigned. Both the source and target interfaces are re-pushed. Returns
// the updated (sanitized) user.
func (s *Service) MigratePeer(userID, publicKey, targetIfaceID string) (*models.User, error) {
	uID, err := uuid.Parse(userID)
	if err != nil {
		return nil, invalidInput("invalid user id %q", userID)
	}
	pk, err := agentmodels.ParseKey(publicKey)
	if err != nil {
		return nil, invalidInput("invalid peer public key")
	}
	targetID, err := uuid.Parse(targetIfaceID)
	if err != nil {
		return nil, invalidInput("invalid target interface id %q", targetIfaceID)
	}

	peer, err := s.store.Users().Peers(uID).Get(pk)
	if err != nil {
		return nil, err
	}
	sourceID := peer.InterfaceId
	if sourceID == targetID {
		return nil, invalidInput("peer is already on that interface")
	}

	// The InterfacePeer (PSK/endpoint/keepalive/AllowedIPs) lives on the source
	// interface's config — read it so those carry over.
	sourceIface, _, err := s.getInterfaceByID(sourceID)
	if err != nil {
		return nil, fmt.Errorf("load source interface: %w", err)
	}
	var srcIfacePeer *agentmodels.InterfacePeer
	for i := range sourceIface.Peers {
		if sourceIface.Peers[i].Key == pk {
			cp := sourceIface.Peers[i]
			srcIfacePeer = &cp
			break
		}
	}
	if srcIfacePeer == nil {
		return nil, invalidInput("peer not found on its current interface")
	}

	targetIface, _, err := s.getInterfaceByID(targetID)
	if err != nil {
		return nil, fmt.Errorf("load target interface: %w", err)
	}

	// Resolve the peer's address on the target (kept if valid+free, else
	// reassigned).
	network, used, err := s.interfaceUsage(targetID)
	if err != nil {
		return nil, fmt.Errorf("target interface: %w", err)
	}
	// Only un-protect the migrating peer's own address when source and target
	// share one address pool (the same tunnel): then interfaceUsage(target)
	// counted that address via the source member, so keeping it is safe. For
	// unrelated interfaces the same IP in `used` belongs to a DIFFERENT peer
	// already on the target — leave it, so migratedAllowedIPs reassigns rather
	// than producing two peers on one /32.
	if sourceIface.Tunnel != nil && targetIface.Tunnel != nil && *sourceIface.Tunnel == *targetIface.Tunnel {
		for _, cidr := range srcIfacePeer.AllowedIPs {
			if ip, isHost := parseAllowedIP(cidr); ip != nil && isHost {
				delete(used, ip.String())
			}
		}
	}
	newAllowedIPs, err := s.migratedAllowedIPs(srcIfacePeer.AllowedIPs, network, used)
	if err != nil {
		return nil, err
	}

	// Move it: remove from the source (user record + source interface), then
	// re-add under the target with the resolved address, keeping its identity.
	if err = s.store.Users().Peers(uID).Delete(pk); err != nil {
		return nil, err
	}
	newPeer := &models.Peer{
		Name:        peer.Name,
		PrivateKey:  peer.PrivateKey,
		InterfaceId: targetID,
		Disabled:    peer.Disabled,
		DNS:         peer.DNS,
	}
	newIfacePeer := agentmodels.InterfacePeer{
		Key:               srcIfacePeer.Key,
		PresharedKey:      srcIfacePeer.PresharedKey,
		AllowedIPs:        newAllowedIPs,
		Endpoint:          srcIfacePeer.Endpoint,
		Disabled:          srcIfacePeer.Disabled,
		KeepaliveInterval: srcIfacePeer.KeepaliveInterval,
	}
	ps := s.store.Users().Peers(uID)
	if ext, ok := ps.(interface {
		SetWithIfacePeer(*models.Peer, agentmodels.InterfacePeer) error
	}); ok {
		if err = ext.SetWithIfacePeer(newPeer, newIfacePeer); err != nil {
			return nil, err
		}
	} else if err = ps.Set(newPeer); err != nil {
		return nil, err
	}

	s.pushInterfaceByID(sourceID)
	s.pushInterfaceByID(targetID)

	u, err := s.store.Users().Get(uID)
	if err != nil {
		return nil, err
	}
	sanitized := sanitizeUser(*u)
	return &sanitized, nil
}

// getInterfaceByID loads an interface (and its owning server ID) by interface ID
// across all servers.
func (s *Service) getInterfaceByID(ifaceID uuid.UUID) (*models.Interface, uuid.UUID, error) {
	srv, err := s.findServerByInterface(ifaceID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	iface, err := s.store.Servers().Interfaces(srv.ID).Get(ifaceID)
	if err != nil {
		return nil, uuid.Nil, err
	}
	return iface, srv.ID, nil
}

// migratedAllowedIPs decides a peer's AllowedIPs on the target interface: keep
// them when every host entry is inside the target subnet and free there,
// otherwise swap the host part for a freshly-assigned free address on the target
// (routed CIDRs carry over unchanged). used must already exclude the migrating
// peer's own addresses.
func (s *Service) migratedAllowedIPs(current []string, network *net.IPNet, used map[string]bool) ([]string, error) {
	keep := true
	hostCount := 0
	var routes []string
	for _, cidr := range current {
		ip, isHost := parseAllowedIP(cidr)
		if isHost && ip != nil {
			hostCount++
			if !network.Contains(ip) || used[ip.String()] {
				keep = false
			}
		} else {
			routes = append(routes, cidr)
		}
	}
	if keep {
		return current, nil
	}
	// Reassign a distinct free address for EACH host entry (not just one, which
	// would drop a peer's secondary addresses), keeping routed CIDRs unchanged.
	out := make([]string, 0, hostCount+len(routes))
	for i := 0; i < hostCount; i++ {
		ip, err := pickFreeHostIP(network, used)
		if err != nil {
			return nil, err
		}
		used[ip.String()] = true // don't hand the same address to the next entry
		out = append(out, fmt.Sprintf("%s/32", ip.String()))
	}
	return append(out, routes...), nil
}

// interfaceUsage loads the interface owning ifaceID and returns its subnet along
// with the set of host IPs already taken on it — the interface's own address and
// every existing peer's AllowedIPs. Shared by nextFreeIP (auto-assign a free
// address) and validatePeerAllowedIPs (validate an explicit one). The interface's
// own host address is included in the used set, so a peer is never handed (or
// allowed to claim) the IP the interface itself uses.
//
// A tunnel's members share ONE address space: the exit sits on the entry's
// subnet, and either end can carry client peers — so a peer's address must be
// unique across every member, not just this interface. When ifaceID is a tunnel
// member, the used set therefore unions all members' addresses and peers;
// otherwise nextFreeIP could hand out the other member's address (the reported
// bug: a peer on the entry got the exit's 172.23.0.2) or a peer already on it.
func (s *Service) interfaceUsage(ifaceID uuid.UUID) (*net.IPNet, map[string]bool, error) {
	srv, err := s.findServerByInterface(ifaceID)
	if err != nil {
		return nil, nil, err
	}
	iface, err := s.store.Servers().Interfaces(srv.ID).Get(ifaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("interface: %w", err)
	}
	_, network, err := net.ParseCIDR(iface.Address)
	if err != nil {
		return nil, nil, fmt.Errorf("interface has no valid address: %w", err)
	}

	used := map[string]bool{}
	addInterfaceIPs(used, iface)

	if iface.Tunnel != nil {
		if all, e := s.allInterfaces(); e == nil {
			for i := range all {
				if all[i].ID != iface.ID && all[i].Tunnel != nil && *all[i].Tunnel == *iface.Tunnel {
					addInterfaceIPs(used, &all[i])
				}
			}
		}
	}
	return network, used, nil
}

// addInterfaceIPs records an interface's own address and every host IP its peers
// use into used, for free-IP allocation and collision checks.
func addInterfaceIPs(used map[string]bool, iface *models.Interface) {
	if ip, _, e := net.ParseCIDR(iface.Address); e == nil {
		used[ip.String()] = true
	}
	for _, peer := range iface.Peers {
		for _, cidr := range peer.AllowedIPs {
			if ip, _ := parseAllowedIP(cidr); ip != nil {
				used[ip.String()] = true
			}
		}
	}
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
	return pickFreeHostIP(network, used)
}

// pickFreeHostIP returns the first host address in network not present in used.
// On a /31 (RFC 3021 point-to-point) or /32 every address is usable; larger
// subnets skip the network and broadcast addresses.
func pickFreeHostIP(network *net.IPNet, used map[string]bool) (net.IP, error) {
	networkIP := network.IP.Mask(network.Mask)
	broadcast := broadcastIP(network)
	ones, bits := network.Mask.Size()
	skipEnds := bits-ones >= 2
	for ip := cloneIP(networkIP); network.Contains(ip); incIP(ip) {
		if skipEnds && (ip.Equal(networkIP) || ip.Equal(broadcast)) {
			continue
		}
		if !used[ip.String()] {
			return cloneIP(ip), nil
		}
	}
	return nil, fmt.Errorf("no free host address in subnet %s", network.String())
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
