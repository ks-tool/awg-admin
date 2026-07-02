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
	"net"
	"testing"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// hostPeerIPs returns the /32 (host) AllowedIPs of an interface's peers — the
// client addresses, excluding tunnel gateway peers (0.0.0.0/0 or a subnet route).
func hostPeerIPs(iface *models.Interface) []string {
	var out []string
	for _, p := range iface.Peers {
		for _, a := range p.AllowedIPs {
			if _, n, err := net.ParseCIDR(a); err == nil {
				if o, b := n.Mask.Size(); o == b {
					out = append(out, a)
				}
			}
		}
	}
	return out
}

func TestTunnelUnifiedPeerPoolAndMigrate(t *testing.T) {
	svc := newTestService(t)
	srv1 := tunnelTestServer(t, svc, "relay")
	srv2 := tunnelTestServer(t, svc, "exit")

	entryCfg := agentmodels.InterfaceConfig{Interface: "awg0", Address: "172.23.30.1/24", ListenPort: 53000}
	agentmodels.GenerateAmneziaParams(&entryCfg)
	entry, err := svc.CreateInterface(srv1.ID.String(), entryCfg)
	if err != nil {
		t.Fatalf("CreateInterface entry: %v", err)
	}
	exit, err := svc.CreateInterface(srv2.ID.String(), agentmodels.InterfaceConfig{Interface: "awg0", Address: "10.8.8.1/24", ListenPort: 51820})
	if err != nil {
		t.Fatalf("CreateInterface exit: %v", err)
	}
	if _, err = svc.BuildTunnel([]models.TunnelStep{
		{ServerID: srv1.ID, IfaceID: entry.ID},
		{ServerID: srv2.ID, IfaceID: exit.ID},
	}, ""); err != nil {
		t.Fatalf("BuildTunnel: %v", err)
	}

	// The exit is moved onto the entry's subnet at 172.23.30.2 (the shared pool).
	exitStored, _ := svc.store.Servers().Interfaces(srv2.ID).Get(exit.ID)
	if exitStored.Address != "172.23.30.2/24" {
		t.Fatalf("exit address = %q, want 172.23.30.2/24", exitStored.Address)
	}

	user, err := svc.CreateUser(UserInput{Name: "u1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Req 1: a peer auto-assigned on the ENTRY must skip the exit's .2 (the bug)
	// and land on .3 — the tunnel members share one address pool.
	entryUser, err := svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: entry.ID})
	if err != nil {
		t.Fatalf("AddPeer entry: %v", err)
	}
	entryStored, _ := svc.store.Servers().Interfaces(srv1.ID).Get(entry.ID)
	if got := hostPeerIPs(entryStored); len(got) != 1 || got[0] != "172.23.30.3/32" {
		t.Fatalf("entry client peer IPs = %v, want [172.23.30.3/32] (must skip the exit's .2)", got)
	}

	// And a peer on the EXIT must avoid .1/.2/.3 across the shared pool -> .4.
	if _, err = svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: exit.ID}); err != nil {
		t.Fatalf("AddPeer exit: %v", err)
	}
	exitStored, _ = svc.store.Servers().Interfaces(srv2.ID).Get(exit.ID)
	if got := hostPeerIPs(exitStored); len(got) != 1 || got[0] != "172.23.30.4/32" {
		t.Fatalf("exit client peer IPs = %v, want [172.23.30.4/32]", got)
	}

	// Req 2: migrate the entry's peer (.3) to the exit. Same subnet -> keep its IP.
	var entryPeerKey string
	for _, p := range entryUser.Peers {
		if p.InterfaceId == entry.ID {
			entryPeerKey = p.PrivateKey.String() // sanitized: holds the public key
		}
	}
	if entryPeerKey == "" {
		t.Fatal("could not find the entry peer's public key")
	}
	u, err := svc.MigratePeer(user.ID.String(), entryPeerKey, exit.ID.String())
	if err != nil {
		t.Fatalf("MigratePeer: %v", err)
	}

	// The peer's user record now points at the exit interface.
	var migratedTo uuid.UUID
	for _, p := range u.Peers {
		if p.PrivateKey.String() == entryPeerKey {
			migratedTo = p.InterfaceId
		}
	}
	if migratedTo != exit.ID {
		t.Fatalf("migrated peer InterfaceId = %s, want exit %s", migratedTo, exit.ID)
	}

	// The entry lost its client peer; the exit now carries both .3 (migrated,
	// address kept) and .4.
	entryStored, _ = svc.store.Servers().Interfaces(srv1.ID).Get(entry.ID)
	if got := hostPeerIPs(entryStored); len(got) != 0 {
		t.Fatalf("after migrate, entry client peers = %v, want none", got)
	}
	exitStored, _ = svc.store.Servers().Interfaces(srv2.ID).Get(exit.ID)
	set := map[string]bool{}
	for _, ip := range hostPeerIPs(exitStored) {
		set[ip] = true
	}
	if !set["172.23.30.3/32"] || !set["172.23.30.4/32"] || len(set) != 2 {
		t.Fatalf("after migrate, exit client peers = %v, want {172.23.30.3/32, 172.23.30.4/32}", set)
	}
}

// TestMigratePeerReassignsOnCollision covers the case where the target interface
// (not a tunnel sibling) already has a peer at the migrating peer's address:
// the migrated peer must be reassigned rather than duplicating that /32.
func TestMigratePeerReassignsOnCollision(t *testing.T) {
	svc := newTestService(t)
	srvA := tunnelTestServer(t, svc, "A")
	srvB := tunnelTestServer(t, svc, "B")

	// Same subnet on two different servers — allowed (uniqueness is per-server).
	ifaceA, err := svc.CreateInterface(srvA.ID.String(), agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820})
	if err != nil {
		t.Fatalf("CreateInterface A: %v", err)
	}
	ifaceB, err := svc.CreateInterface(srvB.ID.String(), agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820})
	if err != nil {
		t.Fatalf("CreateInterface B: %v", err)
	}

	user, err := svc.CreateUser(UserInput{Name: "u1"})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Each interface independently auto-allocates 10.0.0.2 for its peer.
	userA, err := svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: ifaceA.ID})
	if err != nil {
		t.Fatalf("AddPeer A: %v", err)
	}
	if _, err = svc.AddPeer(user.ID.String(), AddPeerInput{InterfaceID: ifaceB.ID}); err != nil {
		t.Fatalf("AddPeer B: %v", err)
	}

	var keyA string
	for _, p := range userA.Peers {
		if p.InterfaceId == ifaceA.ID {
			keyA = p.PrivateKey.String()
		}
	}
	if _, err = svc.MigratePeer(user.ID.String(), keyA, ifaceB.ID.String()); err != nil {
		t.Fatalf("MigratePeer: %v", err)
	}

	bStored, _ := svc.store.Servers().Interfaces(srvB.ID).Get(ifaceB.ID)
	count := map[string]int{}
	for _, ip := range hostPeerIPs(bStored) {
		count[ip]++
	}
	if count["10.0.0.2/32"] != 1 {
		t.Fatalf("target must keep exactly one .2 (no duplicate), got %v", count)
	}
	if count["10.0.0.3/32"] != 1 {
		t.Fatalf("migrated peer must be reassigned to .3, got %v", count)
	}
	if len(hostPeerIPs(bStored)) != 2 {
		t.Fatalf("target should have 2 distinct client peers, got %v", hostPeerIPs(bStored))
	}
}
