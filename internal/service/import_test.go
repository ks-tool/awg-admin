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
	"strings"
	"testing"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"
)

func genKey(t *testing.T) agentmodels.Key {
	t.Helper()
	k, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func keyStr(k agentmodels.Key) string { return k.String() }
func pubStr(k agentmodels.Key) string { pub := k.PublicKey(); return pub.String() }

// importFixture seeds the fake agent with a live unmanaged "wg-legacy" and its
// conf: three peers, a routed LAN behind the first, hooks, DNS.
type importFixture struct {
	ifKey, p1, p2, p3 agentmodels.Key
	conf              string
}

func newImportFixture(t *testing.T, fa *fakeAgentTLS) importFixture {
	t.Helper()
	f := importFixture{ifKey: genKey(t), p1: genKey(t), p2: genKey(t), p3: genKey(t)}
	f.conf = `[Interface]
PrivateKey = ` + keyStr(f.ifKey) + `
Address = 10.8.0.1/24
ListenPort = 51900
DNS = 1.1.1.1
PostUp = iptables -A FORWARD -i %i -j ACCEPT
PostDown = iptables -D FORWARD -i %i -j ACCEPT

[Peer]
PublicKey = ` + pubStr(f.p1) + `
AllowedIPs = 10.8.0.2/32, 192.168.77.0/24
PersistentKeepalive = 25

[Peer]
PublicKey = ` + pubStr(f.p2) + `
AllowedIPs = 10.8.0.3/32

[Peer]
PublicKey = ` + pubStr(f.p3) + `
AllowedIPs = 10.8.0.4/32
`
	fa.mu.Lock()
	fa.sources["wg-legacy"] = f.conf
	fa.unmanaged = []models.UnmanagedInterface{{Name: "wg-legacy", Kind: models.InterfaceKindWireGuard}}
	fa.mu.Unlock()
	return f
}

func TestListUnmanagedInterfaces(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)

	list, err := svc.ListUnmanagedInterfaces(srv.ID.String())
	if err != nil {
		t.Fatalf("ListUnmanagedInterfaces: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Fatalf("empty list = %#v, want non-nil empty", list)
	}

	newImportFixture(t, fa)
	list, err = svc.ListUnmanagedInterfaces(srv.ID.String())
	if err != nil {
		t.Fatalf("ListUnmanagedInterfaces: %v", err)
	}
	if len(list) != 1 || list[0].Name != "wg-legacy" || list[0].Kind != models.InterfaceKindWireGuard {
		t.Fatalf("list = %+v", list)
	}
}

// Creating (or renaming to) the name of a live unmanaged interface would make
// the agent adopt — and overwrite — the wg-quick link; only the import may.
func TestCreateInterfaceRejectsUnmanagedName(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)
	newImportFixture(t, fa)

	_, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{Interface: "wg-legacy", Address: "10.1.0.1/24", ListenPort: 51821})
	var verr *ValidationError
	if !errors.As(err, &verr) || !strings.Contains(err.Error(), "import it instead") {
		t.Fatalf("CreateInterface(wg-legacy) err = %v, want a ValidationError pointing at import", err)
	}

	iface, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.1.0.1/24", ListenPort: 51821})
	if err != nil {
		t.Fatalf("CreateInterface(wg0): %v", err)
	}
	cfg := iface.InterfaceConfig
	cfg.Interface = "wg-legacy"
	if _, err := svc.UpdateInterfaceConfig(srv.ID.String(), iface.ID.String(), cfg); !errors.As(err, &verr) {
		t.Fatalf("rename to wg-legacy err = %v, want ValidationError", err)
	}
	// An unrelated edit of an interface keeps working (the name didn't change).
	cfg.Interface = "wg0"
	cfg.MTU = 1400
	if _, err := svc.UpdateInterfaceConfig(srv.ID.String(), iface.ID.String(), cfg); err != nil {
		t.Fatalf("unrelated edit: %v", err)
	}
}

// With no reachable agent the unmanaged-name check must be skipped, not block.
func TestCreateInterfaceUnmanagedCheckIsBestEffort(t *testing.T) {
	svc, srv := newValidationTestServer(t)
	if _, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.0.0.1/24", ListenPort: 51820}); err != nil {
		t.Fatalf("CreateInterface with unreachable agent: %v", err)
	}
}

func TestPreviewImportReportsPlanWithoutWriting(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)
	f := newImportFixture(t, fa)
	alice, err := svc.CreateUser(UserInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	peerMap := "alice:\n  " + keyStr(f.p1) + ": laptop\nbob:\n  " + keyStr(f.p2) + ": pc\n"
	pv, err := svc.PreviewImport(srv.ID.String(), "wg-legacy", peerMap)
	if err != nil {
		t.Fatalf("PreviewImport: %v", err)
	}
	if pv.Interface != "wg-legacy" || pv.Address != "10.8.0.1/24" || pv.ListenPort != 51900 || pv.Amnezia || !pv.Live || pv.LiveKind != models.InterfaceKindWireGuard || pv.Source != "/etc/wireguard/wg-legacy.conf" {
		t.Fatalf("preview header = %+v", pv)
	}
	if len(pv.Users) != 2 {
		t.Fatalf("users = %+v", pv.Users)
	}
	if pv.Users[0].Name != "alice" || !pv.Users[0].Exists || len(pv.Users[0].Peers) != 1 || pv.Users[0].Peers[0].Name != "laptop" || pv.Users[0].Peers[0].PublicKey != pubStr(f.p1) || pv.Users[0].Peers[0].Keepalive != 25 {
		t.Fatalf("alice = %+v", pv.Users[0])
	}
	if pv.Users[1].Name != "bob" || pv.Users[1].Exists || len(pv.Users[1].Peers) != 1 || pv.Users[1].Peers[0].Keepalive != 0 {
		t.Fatalf("bob = %+v", pv.Users[1])
	}
	if len(pv.EmbeddedPeers) != 1 || pv.EmbeddedPeers[0].PublicKey != pubStr(f.p3) || pv.EmbeddedPeers[0].Name != "" {
		t.Fatalf("embedded = %+v", pv.EmbeddedPeers)
	}
	if len(pv.Hooks.PostUp) != 1 || len(pv.GeneratedHooks.PostUp) != 1 || pv.GeneratedHooks.PostUp[0] != "ip route replace 192.168.77.0/24 dev %i" {
		t.Fatalf("hooks = %+v / generated %+v", pv.Hooks, pv.GeneratedHooks)
	}
	joined := strings.Join(pv.Warnings, "\n")
	for _, want := range []string{"DNS", "not idempotent", "without a user", "PersistentKeepalive"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q:\n%s", want, joined)
		}
	}

	// Dry run: no interface, no bob, alice untouched, nothing pushed.
	ifaces, _ := svc.ListInterfaces(srv.ID.String())
	if len(ifaces) != 0 {
		t.Fatalf("preview created interfaces: %+v", ifaces)
	}
	users, _ := svc.ListUsers()
	if len(users) != 1 || users[0].ID != alice.ID || len(users[0].Peers) != 0 {
		t.Fatalf("preview touched users: %+v", users)
	}
	if _, ok := fa.get("wg-legacy"); ok {
		t.Fatal("preview pushed to the agent")
	}
}

func TestImportInterfaceFromServerCreatesEverythingAndPushes(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)
	f := newImportFixture(t, fa)
	alice, err := svc.CreateUser(UserInput{Name: "alice"})
	if err != nil {
		t.Fatal(err)
	}

	peerMap := "alice:\n  " + keyStr(f.p1) + ": laptop\nbob:\n  " + keyStr(f.p2) + ": pc\n"
	iface, err := svc.ImportInterfaceFromServer(srv.ID.String(), "wg-legacy", peerMap)
	if err != nil {
		t.Fatalf("ImportInterfaceFromServer: %v", err)
	}
	if iface.Interface != "wg-legacy" || iface.PrivateKey != f.ifKey || len(iface.Peers) != 3 || !iface.InSync {
		t.Fatalf("imported = %+v", iface)
	}
	// Full config (all three peers, hooks incl. the generated route) reached the agent.
	pushed, ok := fa.get("wg-legacy")
	if !ok || len(pushed.Peers) != 3 || len(pushed.PostUp) != 2 || pushed.ListenPort != 51900 {
		t.Fatalf("pushed = %+v (ok=%v)", pushed, ok)
	}

	// alice reused, bob created, both peers attached to the new interface and
	// resolvable by public key; the third peer stays embedded (no user).
	users, _ := svc.ListUsers()
	if len(users) != 2 {
		t.Fatalf("users = %+v", users)
	}
	for _, u := range users {
		switch u.Name {
		case "alice":
			if u.ID != alice.ID || len(u.Peers) != 1 || u.Peers[0].Name != "laptop" || u.Peers[0].InterfaceId != iface.ID {
				t.Fatalf("alice = %+v", u)
			}
		case "bob":
			if len(u.Peers) != 1 || u.Peers[0].Name != "pc" || u.Peers[0].InterfaceId != iface.ID {
				t.Fatalf("bob = %+v", u)
			}
		}
		if _, err := svc.GetPeerConfig(u.ID.String(), u.Peers[0].PrivateKey.String()); err != nil {
			t.Fatalf("GetPeerConfig for %s: %v", u.Name, err)
		}
	}
	// The peer config renders with the imported interface's endpoint port.
	cfgText, _ := svc.GetPeerConfig(alice.ID.String(), pubStr(f.p1))
	if !strings.Contains(cfgText, "Endpoint = example.invalid:51900") || !strings.Contains(cfgText, "PublicKey = "+pubStr(f.ifKey)) {
		t.Fatalf("rendered client config:\n%s", cfgText)
	}

	// Now it's a normal managed interface: not unmanaged anymore on a second
	// import attempt, and the name is taken.
	if _, err := svc.ImportInterfaceFromServer(srv.ID.String(), "wg-legacy", ""); err == nil || !strings.Contains(err.Error(), "already used") {
		t.Fatalf("second import err = %v, want a name conflict", err)
	}
}

func TestImportWithEmptyMapEmbedsAllPeers(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)
	newImportFixture(t, fa)

	iface, err := svc.ImportInterfaceFromServer(srv.ID.String(), "wg-legacy", "# no users\n")
	if err != nil {
		t.Fatalf("ImportInterfaceFromServer: %v", err)
	}
	if len(iface.Peers) != 3 {
		t.Fatalf("peers = %d", len(iface.Peers))
	}
	users, _ := svc.ListUsers()
	if len(users) != 0 {
		t.Fatalf("users created: %+v", users)
	}
}

func TestPreviewImportValidationErrors(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)
	f := newImportFixture(t, fa)
	stranger := genKey(t)

	// A peer key already registered to a user elsewhere must not be imported twice.
	other, _ := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.1.0.1/24", ListenPort: 51821})
	carol, _ := svc.CreateUser(UserInput{Name: "carol"})
	if _, err := svc.AddPeer(carol.ID.String(), AddPeerInput{Name: "old", InterfaceID: other.ID, PrivateKey: keyStr(f.p3)}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if _, err := svc.CreateUser(UserInput{Name: "dup"}); err != nil {
			t.Fatal(err)
		}
	}

	cases := []struct{ name, iface, peerMap, want string }{
		{"no conf on server", "wg9", "", "no wg-quick config for \"wg9\""},
		{"bad map", "wg-legacy", "  " + keyStr(f.p1) + ": x\n", "peer map: line 1"},
		{"key matches no peer", "wg-legacy", "alice:\n  " + keyStr(stranger) + ": ghost\n", "matches no [Peer]"},
		{"key already registered", "wg-legacy", "alice:\n  " + keyStr(f.p3) + ": again\n", "already exists in the database (user \"carol\")"},
		{"ambiguous user", "wg-legacy", "dup:\n  " + keyStr(f.p1) + ": laptop\n", "matches 2 users"},
		{"empty name", "", "", "interface name is required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.PreviewImport(srv.ID.String(), tc.iface, tc.peerMap)
			var verr *ValidationError
			if !errors.As(err, &verr) || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want ValidationError containing %q", err, tc.want)
			}
		})
	}
}

func TestPreviewImportRejectsConflictsWithExistingInterfaces(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)
	newImportFixture(t, fa)

	// Same listen port as the conf → rejected by the shared uniqueness check.
	if _, err := svc.CreateInterface(srv.ID.String(), agentmodels.InterfaceConfig{Interface: "wg0", Address: "10.1.0.1/24", ListenPort: 51900}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.PreviewImport(srv.ID.String(), "wg-legacy", "")
	if err == nil || !strings.Contains(err.Error(), "listen port 51900") {
		t.Fatalf("err = %v, want a port conflict", err)
	}
}

func TestPreviewImportRejectsBadConfAndPeerIPs(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)
	k, p := genKey(t), genKey(t)

	set := func(conf string) {
		fa.mu.Lock()
		fa.sources["wg-x"] = conf
		fa.mu.Unlock()
	}
	set("[Interface]\nPrivateKey = " + keyStr(k) + "\nAddress = 10.0.0.1/24\nListenPort = 51820\nFoo = bar\n")
	if _, err := svc.PreviewImport(srv.ID.String(), "wg-x", ""); err == nil || !strings.Contains(err.Error(), "unknown [Interface] key Foo") {
		t.Fatalf("unknown key err = %v", err)
	}
	set("[Interface]\nPrivateKey = " + keyStr(k) + "\nAddress = 10.0.0.1/24\nListenPort = 51820\n[Peer]\nPublicKey = " + pubStr(p) + "\nAllowedIPs = 10.9.9.9/32\n")
	if _, err := svc.PreviewImport(srv.ID.String(), "wg-x", ""); err == nil || !strings.Contains(err.Error(), "not in the interface subnet") {
		t.Fatalf("peer outside subnet err = %v", err)
	}
	set("[Interface]\nPrivateKey = " + keyStr(k) + "\nAddress = 10.0.0.1/24\nListenPort = 51820\n[Peer]\nPublicKey = " + pubStr(p) + "\nAllowedIPs = 10.0.0.1/32\n")
	if _, err := svc.PreviewImport(srv.ID.String(), "wg-x", ""); err == nil || !strings.Contains(err.Error(), "already used by the interface itself") {
		t.Fatalf("peer on interface IP err = %v", err)
	}
}

func TestPreviewImportWarnsOnDownAndUserspace(t *testing.T) {
	svc, srv, fa := newTestServerWithAgent(t)
	newImportFixture(t, fa)

	// Interface down (not in the unmanaged list): the agent will create it.
	fa.mu.Lock()
	fa.unmanaged = nil
	fa.mu.Unlock()
	pv, err := svc.PreviewImport(srv.ID.String(), "wg-legacy", "")
	if err != nil {
		t.Fatal(err)
	}
	if pv.Live || !strings.Contains(strings.Join(pv.Warnings, "\n"), "no live interface") {
		t.Fatalf("down: live=%v warnings=%v", pv.Live, pv.Warnings)
	}

	// Live under a userspace agent: adoption impossible, say so.
	fa.mu.Lock()
	fa.unmanaged = []models.UnmanagedInterface{{Name: "wg-legacy", Kind: models.InterfaceKindWireGuard}}
	fa.info.Backend = "userspace"
	fa.mu.Unlock()
	pv, err = svc.PreviewImport(srv.ID.String(), "wg-legacy", "")
	if err != nil {
		t.Fatal(err)
	}
	if !pv.Live || !strings.Contains(strings.Join(pv.Warnings, "\n"), "userspace build") {
		t.Fatalf("userspace: live=%v warnings=%v", pv.Live, pv.Warnings)
	}
}
