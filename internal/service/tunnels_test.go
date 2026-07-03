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
	"strings"
	"testing"
	"time"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
)

// tunnelTestServer creates a server whose agent is unreachable (127.0.0.1:1
// refuses fast), so the best-effort interface pushes fail quickly without a
// slow SSH dial — the stored config is what we assert on.
func tunnelTestServer(t *testing.T, svc *Service, name string) *models.Server {
	t.Helper()
	srv, err := svc.CreateServer(ServerInput{
		Name:  name,
		SSH:   models.SSHConfig{Host: "127.0.0.1", Port: 1, User: "root", Password: "x"},
		Agent: models.Agent{Address: "127.0.0.1:1"},
	})
	if err != nil {
		t.Fatalf("CreateServer %s: %v", name, err)
	}
	return srv
}

func TestBuildAndRemoveTunnel(t *testing.T) {
	svc := newTestService(t)
	srv1 := tunnelTestServer(t, svc, "relay")
	srv2 := tunnelTestServer(t, svc, "exit")

	// Build the entry as an AmneziaWG interface (obfuscation params are opt-in
	// now — CreateInterface no longer auto-generates them), matching what the UI
	// sends via GenerateInterfaceDefaults, so the tunnel has params to copy to
	// the exit.
	entryCfg := agentmodels.InterfaceConfig{
		Interface: "awg0", Address: "172.23.24.2/24", ListenPort: 53053,
	}
	agentmodels.GenerateAmneziaParams(&entryCfg)
	entry, err := svc.CreateInterface(srv1.ID.String(), entryCfg)
	if err != nil {
		t.Fatalf("CreateInterface entry: %v", err)
	}
	exit, err := svc.CreateInterface(srv2.ID.String(), agentmodels.InterfaceConfig{
		Interface: "awg0", Address: "10.9.9.1/24", ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateInterface exit: %v", err)
	}

	// A client peer on the entry — must survive the tunnel build.
	clientKey, _ := agentmodels.GeneratePrivateKey()
	entryStored, _ := svc.store.Servers().Interfaces(srv1.ID).Get(entry.ID)
	entryStored.Peers = append(entryStored.Peers, agentmodels.InterfacePeer{
		Key: clientKey.PublicKey(), AllowedIPs: []string{"172.23.24.10/32"},
	})
	if err := svc.store.Servers().Interfaces(srv1.ID).Set(entryStored); err != nil {
		t.Fatalf("seed client peer: %v", err)
	}

	tun, err := svc.BuildTunnel([]models.TunnelStep{
		{ServerID: srv1.ID, IfaceID: entry.ID},
		{ServerID: srv2.ID, IfaceID: exit.ID},
	}, "")
	if err != nil {
		t.Fatalf("BuildTunnel: %v", err)
	}

	// --- Entry (relay) ---
	ei, _ := svc.store.Servers().Interfaces(srv1.ID).Get(entry.ID)
	if ei.Tunnel == nil || *ei.Tunnel != tun.ID {
		t.Errorf("entry Tunnel id not set to %v: %v", tun.ID, ei.Tunnel)
	}
	if ei.ListenPort != 53053 {
		t.Errorf("entry ListenPort changed to %d, want 53053 (kept)", ei.ListenPort)
	}
	if ei.Table == 0 {
		t.Errorf("entry Table not set")
	}
	var hasClient, hasGateway bool
	for _, p := range ei.Peers {
		if len(p.AllowedIPs) == 1 && p.AllowedIPs[0] == "172.23.24.10/32" {
			hasClient = true
		}
		if len(p.AllowedIPs) == 1 && p.AllowedIPs[0] == "0.0.0.0/0" && p.PresharedKey != nil && p.Endpoint == "" {
			hasGateway = true
		}
	}
	if !hasClient {
		t.Errorf("entry lost its client peer")
	}
	if !hasGateway {
		t.Errorf("entry missing gateway peer (0.0.0.0/0, PSK, no endpoint)")
	}
	if !hooksContain(ei.PreUp, "ip rule") || !hooksContain(ei.PreUp, "ip route") {
		t.Errorf("entry PreUp missing ip rule/route: %v", ei.PreUp)
	}
	if !hooksContain(ei.PostDown, "ip route del") {
		t.Errorf("entry PostDown missing route cleanup: %v", ei.PostDown)
	}

	// --- Exit ---
	xi, _ := svc.store.Servers().Interfaces(srv2.ID).Get(exit.ID)
	if xi.Tunnel == nil || *xi.Tunnel != tun.ID {
		t.Errorf("exit Tunnel id not set")
	}
	if xi.ListenPort != 0 {
		t.Errorf("exit ListenPort = %d, want 0", xi.ListenPort)
	}
	if !strings.HasPrefix(xi.Address, "172.23.24.") {
		t.Errorf("exit Address %q not on the entry subnet 172.23.24.0/24", xi.Address)
	}
	if xi.Jc == nil || ei.Jc == nil || *xi.Jc != *ei.Jc {
		t.Errorf("exit Amnezia Jc not copied from entry")
	}
	if len(xi.Peers) != 1 {
		t.Fatalf("exit peers = %d, want 1", len(xi.Peers))
	}
	p := xi.Peers[0]
	if p.Endpoint != "127.0.0.1:53053" {
		t.Errorf("exit peer Endpoint = %q, want 127.0.0.1:53053", p.Endpoint)
	}
	if len(p.AllowedIPs) != 1 || p.AllowedIPs[0] != "172.23.24.0/24" {
		t.Errorf("exit peer AllowedIPs = %v, want [172.23.24.0/24]", p.AllowedIPs)
	}
	if p.KeepaliveInterval != 10*time.Second {
		t.Errorf("exit peer keepalive = %v, want 10s", p.KeepaliveInterval)
	}
	if p.Key != entryStored.PrivateKey.PublicKey() {
		t.Errorf("exit peer key is not the entry's public key")
	}
	if !hooksContain(xi.PreUp, "MASQUERADE") {
		t.Errorf("exit PreUp missing MASQUERADE: %v", xi.PreUp)
	}

	// --- ListTunnels ---
	tuns, err := svc.ListTunnels()
	if err != nil {
		t.Fatalf("ListTunnels: %v", err)
	}
	if len(tuns) != 1 || len(tuns[0].Members) != 2 {
		t.Fatalf("ListTunnels = %+v, want 1 tunnel with 2 members", tuns)
	}
	if tuns[0].Members[0].Role != "entry" || tuns[0].Members[1].Role != "exit" {
		t.Errorf("member roles/order = %q,%q; want entry,exit", tuns[0].Members[0].Role, tuns[0].Members[1].Role)
	}

	// --- Delete guard ---
	if err := svc.DeleteInterface(srv1.ID.String(), entry.ID.String()); err == nil {
		t.Errorf("DeleteInterface of a tunnel member should be refused")
	}

	// --- RemoveTunnel drops the tunnel config but KEEPS client peers ---
	if err := svc.RemoveTunnel(tun.ID.String()); err != nil {
		t.Fatalf("RemoveTunnel: %v", err)
	}
	ei2, _ := svc.store.Servers().Interfaces(srv1.ID).Get(entry.ID)
	if ei2.Tunnel != nil {
		t.Errorf("entry Tunnel not cleared after RemoveTunnel")
	}
	if ei2.Table != 0 || len(ei2.PreUp) != 0 || len(ei2.PostDown) != 0 {
		t.Errorf("entry tunnel infra not cleared: table=%d preUp=%d postDown=%d",
			ei2.Table, len(ei2.PreUp), len(ei2.PostDown))
	}
	// The client peer must survive; only the tunnel's gateway peer is removed.
	if len(ei2.Peers) != 1 {
		t.Fatalf("entry peers after RemoveTunnel = %d, want 1 (the client peer kept)", len(ei2.Peers))
	}
	if len(ei2.Peers[0].AllowedIPs) != 1 || ei2.Peers[0].AllowedIPs[0] != "172.23.24.10/32" {
		t.Errorf("entry kept the wrong peer: %v (want the client 172.23.24.10/32)", ei2.Peers[0].AllowedIPs)
	}
	// After removal the interface can be deleted again.
	if err := svc.DeleteInterface(srv1.ID.String(), entry.ID.String()); err != nil {
		t.Errorf("DeleteInterface after RemoveTunnel: %v", err)
	}
}

func hooksContain(hooks []string, sub string) bool {
	for _, h := range hooks {
		if strings.Contains(h, sub) {
			return true
		}
	}
	return false
}
