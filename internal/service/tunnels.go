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
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"time"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

const (
	tunnelMark      = "0x30"
	tunnelTableBase = 52000
	tunnelPrioBase  = 100
	tunnelKeepalive = 10 * time.Second
)

// BuildTunnel wires the given ordered interfaces (entry first, exit last —
// exactly two for now) into one multi-hop tunnel so the entry's clients egress
// the internet via the exit. Both interfaces already exist and are reused: the
// entry keeps its clients + listen port and gains a "gateway peer" to the exit
// plus policy-routing hooks; the exit is reconfigured onto the shared subnet
// with the same obfuscation params, no listen port, a peer back to the entry
// and MASQUERADE hooks. All members get one tunnel id. The shared subnet is
// always the entry interface's own subnet (see below); the subnet argument is
// only accepted when empty or equal to it, never an arbitrary value.
func (s *Service) BuildTunnel(steps []models.TunnelStep, subnet string) (*models.Tunnel, error) {
	debugOp("BuildTunnel").Str("subnet", subnet).Msg("building tunnel")
	if len(steps) != 2 {
		return nil, errors.New("a tunnel needs exactly two interfaces (entry and exit)")
	}
	entryStep, exitStep := steps[0], steps[1]
	if entryStep.ServerID == exitStep.ServerID {
		return nil, errors.New("entry and exit must be on different servers")
	}

	entrySrv, err := s.store.Servers().Get(entryStep.ServerID)
	if err != nil {
		return nil, fmt.Errorf("load entry server: %w", err)
	}
	exitSrv, err := s.store.Servers().Get(exitStep.ServerID)
	if err != nil {
		return nil, fmt.Errorf("load exit server: %w", err)
	}
	entryIface, err := s.store.Servers().Interfaces(entrySrv.ID).Get(entryStep.IfaceID)
	if err != nil {
		return nil, fmt.Errorf("load entry interface: %w", err)
	}
	exitIface, err := s.store.Servers().Interfaces(exitSrv.ID).Get(exitStep.IfaceID)
	if err != nil {
		return nil, fmt.Errorf("load exit interface: %w", err)
	}

	if entryIface.Tunnel != nil || exitIface.Tunnel != nil {
		return nil, errors.New("one of the interfaces is already part of a tunnel")
	}
	if entryIface.ListenPort == 0 {
		return nil, errors.New("entry interface must have a listen port")
	}
	if entrySrv.SSH.Host == "" {
		return nil, errors.New("entry server has no host address for the tunnel endpoint")
	}
	if agentmodels.IsEmpty(entryIface.PrivateKey) || agentmodels.IsEmpty(exitIface.PrivateKey) {
		return nil, errors.New("both interfaces must have a private key")
	}

	// The tunnel subnet is ALWAYS the entry interface's own subnet: the exit is
	// placed on it and routes it back to the relay, so a different subnet
	// wouldn't include the entry's address and the tunnel couldn't carry the
	// relay's clients. It can't be chosen freely — the subnet argument is only
	// honored when it matches (the UI derives it from the entry interface, or
	// leaves it empty and lets this derive it).
	_, subnetNet, err := net.ParseCIDR(entryIface.Address)
	if err != nil {
		return nil, fmt.Errorf("entry interface %q has no valid subnet address: %w", entryIface.Interface, err)
	}
	if subnet != "" {
		if _, reqNet, e := net.ParseCIDR(subnet); e != nil || reqNet.String() != subnetNet.String() {
			return nil, invalidInput("tunnel subnet must match the entry interface subnet %s", subnetNet.String())
		}
	}
	ones, _ := subnetNet.Mask.Size()

	tunnelID := uuid.New()
	psk, err := agentmodels.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate preshared key: %w", err)
	}
	entryPub := entryIface.PrivateKey.PublicKey()
	exitPub := exitIface.PrivateKey.PublicKey()
	endpoint := net.JoinHostPort(entrySrv.SSH.Host, strconv.Itoa(int(entryIface.ListenPort)))
	table, prio := s.nextFreeTable(entrySrv.ID)

	exitIP, err := s.freeHostIP(entryIface, subnetNet)
	if err != nil {
		return nil, err
	}
	exitAddress := fmt.Sprintf("%s/%d", exitIP.String(), ones)

	applyRelayTemplate(entryIface, tunnelID, exitPub, psk, table, prio)
	applyExitTemplate(exitIface, tunnelID, entryIface, entryPub, psk, subnetNet.String(), endpoint, exitAddress)

	// pushInterface both pushes to the agent and persists the interface (via
	// recordSyncResult), so this stores the tunnel id + new config on both ends.
	s.pushInterface(entrySrv.ID, entryIface)
	s.pushInterface(exitSrv.ID, exitIface)

	return &models.Tunnel{
		ID: tunnelID,
		Members: []models.TunnelMember{
			{ServerID: entrySrv.ID, ServerName: entrySrv.Name, IfaceID: entryIface.ID, Interface: entryIface.Interface, Role: "entry"},
			{ServerID: exitSrv.ID, ServerName: exitSrv.Name, IfaceID: exitIface.ID, Interface: exitIface.Interface, Role: "exit"},
		},
	}, nil
}

// ListTunnels groups every interface that carries a tunnel id into its Tunnel,
// entry member first. Reconstructed from the interfaces (no separate storage).
func (s *Service) ListTunnels() ([]models.Tunnel, error) {
	debugOp("ListTunnels").Msg("listing tunnels")
	servers, err := s.store.Servers().List()
	if err != nil {
		return nil, err
	}

	byID := map[uuid.UUID]*models.Tunnel{}
	var order []uuid.UUID
	for i := range servers {
		ifaces, err := s.store.Servers().Interfaces(servers[i].ID).List()
		if err != nil {
			return nil, err
		}
		for j := range ifaces {
			if ifaces[j].Tunnel == nil {
				continue
			}
			id := *ifaces[j].Tunnel
			t, ok := byID[id]
			if !ok {
				t = &models.Tunnel{ID: id}
				byID[id] = t
				order = append(order, id)
			}
			role := "exit"
			if ifaces[j].ListenPort != 0 {
				role = "entry"
			}
			t.Members = append(t.Members, models.TunnelMember{
				ServerID: servers[i].ID, ServerName: servers[i].Name,
				IfaceID: ifaces[j].ID, Interface: ifaces[j].Interface, Role: role,
			})
		}
	}

	out := make([]models.Tunnel, 0, len(order))
	for _, id := range order {
		sort.SliceStable(byID[id].Members, func(a, b int) bool {
			return byID[id].Members[a].Role == "entry" && byID[id].Members[b].Role != "entry"
		})
		out = append(out, *byID[id])
	}
	return out, nil
}

// RemoveTunnel tears a tunnel down, leaving its interfaces "empty" — it resets
// every member to a plain interface (clears peers, hooks, routing table and the
// tunnel id, keeping only name/key/address/listen port/obfuscation) and pushes
// it. Exit members are pushed before the entry so the relay's route/rule (which
// the reconcile-on-update path removes via the old PostDown) go last.
func (s *Service) RemoveTunnel(tunnelID string) error {
	debugOp("RemoveTunnel").Str("tunnel_id", tunnelID).Msg("removing tunnel")
	tid, err := uuid.Parse(tunnelID)
	if err != nil {
		return err
	}

	servers, err := s.store.Servers().List()
	if err != nil {
		return err
	}

	type member struct {
		srvID  uuid.UUID
		iface  models.Interface
		isExit bool
	}
	var members []member
	for i := range servers {
		ifaces, err := s.store.Servers().Interfaces(servers[i].ID).List()
		if err != nil {
			return err
		}
		for j := range ifaces {
			if ifaces[j].Tunnel != nil && *ifaces[j].Tunnel == tid {
				members = append(members, member{srvID: servers[i].ID, iface: ifaces[j], isExit: ifaces[j].ListenPort == 0})
			}
		}
	}
	if len(members) == 0 {
		return errors.New("tunnel not found")
	}

	// The tunnel's gateway/relay peers are keyed by another member's interface
	// public key (BuildTunnel adds them directly — they have no user Peer
	// record). Collect those keys so teardown strips ONLY them and keeps the
	// interface's real client peers. Otherwise the client peers vanish from the
	// device while their Peer records linger under the user, so they still show
	// in the list but GetPeerConfig can't find them on the interface.
	tunnelPeerKeys := map[string]bool{}
	for i := range members {
		if agentmodels.IsEmpty(members[i].iface.PrivateKey) {
			continue
		}
		pub := members[i].iface.PrivateKey.PublicKey()
		tunnelPeerKeys[pub.String()] = true
	}

	sort.SliceStable(members, func(a, b int) bool { return members[a].isExit && !members[b].isExit })
	for i := range members {
		resetTunnelInterface(&members[i].iface, tunnelPeerKeys)
		s.pushInterface(members[i].srvID, &members[i].iface)
	}
	return nil
}

// resetTunnelInterface reverts a tunnel member to a plain interface: it drops
// the tunnel's gateway/relay peers (those keyed by another member's interface
// public key, in tunnelPeerKeys) and all tunnel infra (lifecycle hooks, routing
// table, tunnel id), while KEEPING the interface's client peers so they survive
// the teardown. Identity (name, key, address, listen port, obfuscation) is kept.
func resetTunnelInterface(iface *models.Interface, tunnelPeerKeys map[string]bool) {
	kept := make([]agentmodels.InterfacePeer, 0, len(iface.Peers))
	for i := range iface.Peers {
		if tunnelPeerKeys[iface.Peers[i].Key.String()] {
			continue // the tunnel's own gateway/relay peer — drop it
		}
		kept = append(kept, iface.Peers[i])
	}
	iface.Peers = kept
	iface.PreUp = nil
	iface.PostUp = nil
	iface.PreDown = nil
	iface.PostDown = nil
	iface.Table = 0
	iface.Tunnel = nil
}

// applyRelayTemplate turns the entry interface into the tunnel's relay: it keeps
// the interface's clients and listen port, and adds a gateway peer to the exit
// (AllowedIPs 0.0.0.0/0) plus policy routing so traffic arriving on the
// interface is sent out to the exit (Table + ip rule/route hooks). Hooks are
// idempotent and use wg-quick's %i placeholder (the agent substitutes the
// interface name).
func applyRelayTemplate(iface *models.Interface, tunnelID uuid.UUID, exitPub, psk agentmodels.Key, table, prio int) {
	iface.Table = table
	iface.PreUp = append(iface.PreUp,
		"sysctl -w net.ipv4.ip_forward=1",
		fmt.Sprintf("ip rule del iif %%i table %d priority %d 2>/dev/null; ip rule add iif %%i table %d priority %d", table, prio, table, prio),
		fmt.Sprintf("ip route replace default dev %%i table %d", table),
	)
	iface.PostDown = append(iface.PostDown,
		fmt.Sprintf("ip route del default dev %%i table %d 2>/dev/null || true", table),
		fmt.Sprintf("ip rule del iif %%i table %d priority %d 2>/dev/null || true", table, prio),
	)
	pskCopy := psk
	iface.Peers = append(iface.Peers, agentmodels.InterfacePeer{
		Key:          exitPub,
		PresharedKey: &pskCopy,
		AllowedIPs:   []string{"0.0.0.0/0"},
	})
	id := tunnelID
	iface.Tunnel = &id
}

// applyExitTemplate turns the exit interface into the tunnel's dedicated
// egress: it moves onto the shared subnet with the entry's obfuscation params,
// drops its listen port (it dials the entry), keeps a single peer back to the
// entry (routing the whole subnet), and MASQUERADEs the relayed traffic out.
func applyExitTemplate(iface *models.Interface, tunnelID uuid.UUID, entry *models.Interface, entryPub, psk agentmodels.Key, entrySubnet, endpoint, exitAddress string) {
	iface.Address = exitAddress
	iface.ListenPort = 0
	iface.Table = 0
	copyAmneziaParams(&iface.InterfaceConfig, &entry.InterfaceConfig)

	iface.PreUp = []string{
		"sysctl -w net.ipv4.ip_forward=1",
		"iptables -t mangle -C PREROUTING -i %i -j MARK --set-mark " + tunnelMark + " 2>/dev/null || iptables -t mangle -A PREROUTING -i %i -j MARK --set-mark " + tunnelMark,
		"iptables -t nat -C POSTROUTING ! -o %i -m mark --mark " + tunnelMark + " -j MASQUERADE 2>/dev/null || iptables -t nat -A POSTROUTING ! -o %i -m mark --mark " + tunnelMark + " -j MASQUERADE",
	}
	iface.PostUp = nil
	iface.PreDown = nil
	iface.PostDown = []string{
		"iptables -t mangle -D PREROUTING -i %i -j MARK --set-mark " + tunnelMark + " 2>/dev/null || true",
		"iptables -t nat -D POSTROUTING ! -o %i -m mark --mark " + tunnelMark + " -j MASQUERADE 2>/dev/null || true",
	}

	pskCopy := psk
	iface.Peers = []agentmodels.InterfacePeer{{
		Key:               entryPub,
		PresharedKey:      &pskCopy,
		AllowedIPs:        []string{entrySubnet},
		Endpoint:          endpoint,
		KeepaliveInterval: tunnelKeepalive,
	}}
	id := tunnelID
	iface.Tunnel = &id
}

func copyAmneziaParams(dst, src *agentmodels.InterfaceConfig) {
	dst.Jc, dst.Jmin, dst.Jmax = copyIntPtr(src.Jc), copyIntPtr(src.Jmin), copyIntPtr(src.Jmax)
	dst.S1, dst.S2, dst.S3, dst.S4 = copyIntPtr(src.S1), copyIntPtr(src.S2), copyIntPtr(src.S3), copyIntPtr(src.S4)
	dst.H1, dst.H2, dst.H3, dst.H4 = copyStrPtr(src.H1), copyStrPtr(src.H2), copyStrPtr(src.H3), copyStrPtr(src.H4)
	dst.I1, dst.I2, dst.I3, dst.I4, dst.I5 = copyStrPtr(src.I1), copyStrPtr(src.I2), copyStrPtr(src.I3), copyStrPtr(src.I4), copyStrPtr(src.I5)
}

func copyIntPtr(p *int) *int {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

func copyStrPtr(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// nextFreeSubnet returns the first /24 in 172.23.0.0/16 that doesn't overlap
// any existing interface's address.
func (s *Service) nextFreeSubnet() (string, error) {
	ifaces, err := s.allInterfaces()
	if err != nil {
		return "", err
	}
	var used []*net.IPNet
	for i := range ifaces {
		if _, n, e := net.ParseCIDR(ifaces[i].Address); e == nil {
			used = append(used, n)
		}
	}
	for octet := 0; octet <= 255; octet++ {
		_, cand, _ := net.ParseCIDR(fmt.Sprintf("172.23.%d.0/24", octet))
		overlaps := false
		for _, u := range used {
			if cand.Contains(u.IP) || u.Contains(cand.IP) {
				overlaps = true
				break
			}
		}
		if !overlaps {
			return cand.String(), nil
		}
	}
	return "", errors.New("no free /24 subnet available in 172.23.0.0/16")
}

// nextFreeTable returns a routing table number and ip-rule priority not yet
// used by another interface on entryServerID (so multiple tunnels on one relay
// don't collide).
func (s *Service) nextFreeTable(entryServerID uuid.UUID) (int, int) {
	used := map[int]bool{}
	if ifaces, err := s.store.Servers().Interfaces(entryServerID).List(); err == nil {
		for i := range ifaces {
			if ifaces[i].Table != 0 {
				used[ifaces[i].Table] = true
			}
		}
	}
	table := tunnelTableBase
	for used[table] {
		table++
	}
	return table, tunnelPrioBase + (table - tunnelTableBase)
}

// freeHostIP returns the first usable host address in subnetNet not already
// taken by the entry interface's address, its peers' AllowedIPs, or any other
// interface's address on that subnet.
func (s *Service) freeHostIP(entry *models.Interface, subnetNet *net.IPNet) (net.IP, error) {
	used := map[string]bool{}
	if ip, _, e := net.ParseCIDR(entry.Address); e == nil {
		used[ip.String()] = true
	}
	for _, p := range entry.Peers {
		for _, a := range p.AllowedIPs {
			if ip, _, e := net.ParseCIDR(a); e == nil {
				used[ip.String()] = true
			}
		}
	}
	if ifaces, err := s.allInterfaces(); err == nil {
		for i := range ifaces {
			if ip, _, e := net.ParseCIDR(ifaces[i].Address); e == nil && subnetNet.Contains(ip) {
				used[ip.String()] = true
			}
		}
	}

	broadcast := broadcastIP(subnetNet)
	ip := cloneIP(subnetNet.IP.Mask(subnetNet.Mask)) // network address
	for incIP(ip); subnetNet.Contains(ip); incIP(ip) {
		if ip.Equal(broadcast) || used[ip.String()] {
			continue
		}
		return cloneIP(ip), nil
	}
	return nil, errors.New("no free host address in the tunnel subnet")
}

// allInterfaces returns every stored interface across all servers.
func (s *Service) allInterfaces() ([]models.Interface, error) {
	servers, err := s.store.Servers().List()
	if err != nil {
		return nil, err
	}
	var out []models.Interface
	for i := range servers {
		ifaces, err := s.store.Servers().Interfaces(servers[i].ID).List()
		if err != nil {
			return nil, err
		}
		out = append(out, ifaces...)
	}
	return out, nil
}
