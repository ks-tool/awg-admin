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

package wgquick

import (
	"reflect"
	"strings"
	"testing"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
)

func genKey(t *testing.T) agentmodels.Key {
	t.Helper()
	k, err := agentmodels.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

// Key's String/PublicKey have pointer receivers, so a fresh value needs a
// variable first.
func keyStr(k agentmodels.Key) string { return k.String() }
func pubStr(k agentmodels.Key) string { pub := k.PublicKey(); return pub.String() }

// fullConf is a realistic awg-quick server conf exercising every accepted key:
// comments (full-line and trailing), case-insensitive keys, repeated hooks,
// multi-valued lines, a bare AllowedIPs address, a routed LAN and a PSK.
func fullConf(t *testing.T) (string, agentmodels.Key, agentmodels.Key, agentmodels.Key) {
	t.Helper()
	ifKey, p1, p2 := genKey(t), genKey(t), genKey(t)
	psk := genKey(t)
	conf := `# server conf
[Interface]
PrivateKey = ` + ifKey.String() + `
Address = 10.8.0.1/24  # trailing comment
listenport = 51820
MTU = 1420
DNS = 1.1.1.1, 8.8.8.8
FwMark = 0x30
Table = auto
SaveConfig = true
PostUp = iptables -A FORWARD -i %i -j ACCEPT
PostUp = iptables -t nat -C POSTROUTING -o eth0 -j MASQUERADE || iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE
PostDown = iptables -D FORWARD -i %i -j ACCEPT
Jc = 4
Jmin = 50
Jmax = 1000
S1 = 20
S2 = 30
S3 = 40
S4 = 5
H1 = 100000-200000
H2 = 300000-400000
H3 = 500000-600000
H4 = 700000-800000
I1 = <r 20>

[Peer]
# alice laptop
PublicKey = ` + pubStr(p1) + `
PresharedKey = ` + psk.String() + `
AllowedIPs = 10.8.0.2/32, 192.168.50.0/24
PersistentKeepalive = 25

[Peer]
PublicKey = ` + pubStr(p2) + `
AllowedIPs = 10.8.0.3
Endpoint = client.example.org:51820
`
	return conf, ifKey, p1, p2
}

func TestParseConfFull(t *testing.T) {
	text, ifKey, p1, p2 := fullConf(t)
	conf, err := ParseConf(text)
	if err != nil {
		t.Fatalf("ParseConf: %v", err)
	}
	sec := conf.Interface
	if sec.PrivateKey != ifKey {
		t.Fatal("PrivateKey not parsed")
	}
	if !reflect.DeepEqual(sec.Address, []string{"10.8.0.1/24"}) {
		t.Fatalf("Address = %v", sec.Address)
	}
	if sec.ListenPort == nil || *sec.ListenPort != 51820 || sec.MTU == nil || *sec.MTU != 1420 {
		t.Fatalf("ListenPort/MTU = %v/%v", sec.ListenPort, sec.MTU)
	}
	if !reflect.DeepEqual(sec.DNS, []string{"1.1.1.1", "8.8.8.8"}) {
		t.Fatalf("DNS = %v", sec.DNS)
	}
	if sec.FwMark == nil || *sec.FwMark != 0x30 {
		t.Fatalf("FwMark = %v", sec.FwMark)
	}
	if sec.Table != "auto" || sec.SaveConfig == nil || !*sec.SaveConfig {
		t.Fatalf("Table/SaveConfig = %q/%v", sec.Table, sec.SaveConfig)
	}
	if len(sec.PostUp) != 2 || !strings.HasPrefix(sec.PostUp[0], "iptables -A FORWARD") || len(sec.PostDown) != 1 {
		t.Fatalf("hooks = %v / %v", sec.PostUp, sec.PostDown)
	}
	if sec.Jc == nil || *sec.Jc != 4 || sec.S4 == nil || *sec.S4 != 5 || sec.H4 == nil || *sec.H4 != "700000-800000" || sec.I1 == nil || *sec.I1 != "<r 20>" {
		t.Fatal("amnezia params not parsed")
	}

	if len(conf.Peers) != 2 {
		t.Fatalf("peers = %d", len(conf.Peers))
	}
	a, b := conf.Peers[0], conf.Peers[1]
	if a.PublicKey != p1.PublicKey() || a.PresharedKey == nil || !reflect.DeepEqual(a.AllowedIPs, []string{"10.8.0.2/32", "192.168.50.0/24"}) || a.PersistentKeepalive == nil || *a.PersistentKeepalive != 25 {
		t.Fatalf("peer 1 = %+v", a)
	}
	if b.PublicKey != p2.PublicKey() || !reflect.DeepEqual(b.AllowedIPs, []string{"10.8.0.3/32"}) || b.Endpoint != "client.example.org:51820" || b.PersistentKeepalive != nil {
		t.Fatalf("peer 2 = %+v", b)
	}
}

func TestParseConfErrors(t *testing.T) {
	k := keyStr(genKey(t))
	pk := pubStr(genKey(t))
	base := "[Interface]\nPrivateKey = " + k + "\nAddress = 10.0.0.1/24\nListenPort = 51820\n"
	cases := []struct {
		name, text, want string
	}{
		{"no interface", "[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 10.0.0.2/32\n", "no [Interface]"},
		{"unknown interface key", base + "Adress = 10.0.0.1/24\n", "unknown [Interface] key Adress"},
		{"unknown peer key", base + "[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 10.0.0.2/32\nKeepalive = 25\n", "unknown [Peer] key Keepalive"},
		{"missing private key", "[Interface]\nAddress = 10.0.0.1/24\nListenPort = 51820\n", "PrivateKey is required"},
		{"missing address", "[Interface]\nPrivateKey = " + k + "\nListenPort = 51820\n", "Address is required"},
		{"missing listen port", "[Interface]\nPrivateKey = " + k + "\nAddress = 10.0.0.1/24\n", "ListenPort is required"},
		{"bad key", "[Interface]\nPrivateKey = nope\nAddress = 10.0.0.1/24\nListenPort = 51820\n", "line 2: PrivateKey: not a valid"},
		{"bad port", "[Interface]\nPrivateKey = " + k + "\nAddress = 10.0.0.1/24\nListenPort = 70000\n", "ListenPort"},
		{"duplicate scalar", base + "ListenPort = 51821\n", "duplicate key listenport"},
		{"key outside section", "PrivateKey = " + k + "\n", "outside of a section"},
		{"no equals", base + "PostUp iptables\n", "expected `Key = value`"},
		{"unknown section", base + "[Route]\n", "unknown section"},
		{"peer without pubkey", base + "[Peer]\nAllowedIPs = 10.0.0.2/32\n", "PublicKey is required"},
		{"peer without allowedips", base + "[Peer]\nPublicKey = " + pk + "\n", "AllowedIPs is required"},
		{"bad allowedips", base + "[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 10.0.0.300\n", "not an IP address or CIDR"},
		{"bad endpoint", base + "[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 10.0.0.2/32\nEndpoint = example.org\n", "must be host:port"},
		{"duplicate peer", base + "[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 10.0.0.2/32\n[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 10.0.0.3/32\n", "repeats the PublicKey"},
		{"bad save config", base + "SaveConfig = yes\n", "must be true or false"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseConf(tc.text)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestParseConfKeepaliveOffAndFwMarkOff(t *testing.T) {
	k := keyStr(genKey(t))
	pk := pubStr(genKey(t))
	conf, err := ParseConf("[Interface]\nPrivateKey = " + k + "\nAddress = 10.0.0.1/24\nListenPort = 51820\nFwMark = off\n[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 10.0.0.2/32\nPersistentKeepalive = off\n")
	if err != nil {
		t.Fatal(err)
	}
	if conf.Interface.FwMark == nil || *conf.Interface.FwMark != 0 {
		t.Fatalf("FwMark off = %v", conf.Interface.FwMark)
	}
	if conf.Peers[0].PersistentKeepalive == nil || *conf.Peers[0].PersistentKeepalive != 0 {
		t.Fatalf("keepalive off = %v", conf.Peers[0].PersistentKeepalive)
	}
}

func TestTranslateFull(t *testing.T) {
	text, ifKey, p1, _ := fullConf(t)
	conf, err := ParseConf(text)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := Translate("awg0", conf)
	if err != nil {
		t.Fatalf("Translate: %v", err)
	}
	cfg := tr.Config
	if cfg.Interface != "awg0" || cfg.PrivateKey != ifKey || cfg.ListenPort != 51820 || cfg.Address != "10.8.0.1/24" || cfg.MTU != 1420 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.FirewallMark == nil || *cfg.FirewallMark != 0x30 || cfg.Table != 0 {
		t.Fatalf("fwmark/table = %v/%d", cfg.FirewallMark, cfg.Table)
	}
	if !cfg.IsAmnezia() {
		t.Fatal("expected an Amnezia config")
	}
	if len(cfg.Peers) != 2 || cfg.Peers[0].Key != p1.PublicKey() || cfg.Peers[0].PresharedKey == nil || cfg.Peers[0].KeepaliveInterval.Seconds() != 25 || cfg.Peers[1].KeepaliveInterval != 0 {
		t.Fatalf("peers = %+v", cfg.Peers)
	}

	// Only the routed LAN (outside 10.8.0.0/24) needs an explicit route under
	// Table=auto; the /32s are covered by the connected route.
	wantUp := []string{"ip route replace 192.168.50.0/24 dev %i"}
	wantDown := []string{"ip route del 192.168.50.0/24 dev %i 2>/dev/null || true"}
	if !reflect.DeepEqual(tr.GeneratedPostUp, wantUp) || !reflect.DeepEqual(tr.GeneratedPreDown, wantDown) {
		t.Fatalf("generated = %v / %v", tr.GeneratedPostUp, tr.GeneratedPreDown)
	}
	// Generated hooks are appended after the conf's own, so the preview can
	// show them as a separate block and the agent runs them last.
	if len(cfg.PostUp) != 3 || cfg.PostUp[2] != wantUp[0] || len(cfg.PreDown) != 1 || cfg.PreDown[0] != wantDown[0] {
		t.Fatalf("hooks = PostUp %v, PreDown %v", cfg.PostUp, cfg.PreDown)
	}

	joined := strings.Join(tr.Warnings, "\n")
	for _, want := range []string{"DNS", "SaveConfig", "1 peer(s) have no PersistentKeepalive", "PostUp hook \"iptables -A FORWARD -i %i -j ACCEPT\" is not idempotent"} {
		if !strings.Contains(joined, want) {
			t.Errorf("warnings missing %q:\n%s", want, joined)
		}
	}
	// The -C || -A form is idempotent and must not be flagged.
	if strings.Contains(joined, "MASQUERADE") {
		t.Errorf("idempotent iptables hook was flagged:\n%s", joined)
	}
}

func TestTranslateTableNumberRoutesEverything(t *testing.T) {
	k := keyStr(genKey(t))
	pk := pubStr(genKey(t))
	conf, err := ParseConf("[Interface]\nPrivateKey = " + k + "\nAddress = 10.0.0.1/24\nListenPort = 51820\nTable = 52001\n[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 10.0.0.2/32, 0.0.0.0/0\n")
	if err != nil {
		t.Fatal(err)
	}
	tr, err := Translate("wg0", conf)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Config.Table != 52001 {
		t.Fatalf("Table = %d", tr.Config.Table)
	}
	want := []string{
		"ip route replace 10.0.0.2/32 dev %i table 52001",
		"ip route replace 0.0.0.0/0 dev %i table 52001",
	}
	if !reflect.DeepEqual(tr.GeneratedPostUp, want) {
		t.Fatalf("generated = %v", tr.GeneratedPostUp)
	}
}

func TestTranslateTableOffAndDefaultRouteWarning(t *testing.T) {
	k := keyStr(genKey(t))
	pk := pubStr(genKey(t))
	head := "[Interface]\nPrivateKey = " + k + "\nAddress = 10.0.0.1/24\nListenPort = 51820\n"
	peer := "[Peer]\nPublicKey = " + pk + "\nAllowedIPs = 192.168.1.0/24, 0.0.0.0/0\n"

	conf, err := ParseConf(head + "Table = off\n" + peer)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := Translate("wg0", conf)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.GeneratedPostUp) != 0 || tr.Config.Table != 0 {
		t.Fatalf("Table=off generated %v", tr.GeneratedPostUp)
	}

	conf, err = ParseConf(head + peer)
	if err != nil {
		t.Fatal(err)
	}
	tr, err = Translate("wg0", conf)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(tr.GeneratedPostUp, []string{"ip route replace 192.168.1.0/24 dev %i"}) {
		t.Fatalf("generated = %v", tr.GeneratedPostUp)
	}
	if !strings.Contains(strings.Join(tr.Warnings, "\n"), "default-route policy routing") {
		t.Fatalf("expected the default-route warning, got %v", tr.Warnings)
	}
}

func TestTranslateRejectsUnsupportedAddresses(t *testing.T) {
	k := keyStr(genKey(t))
	cases := []struct{ name, addr, want string }{
		{"two addresses", "10.0.0.1/24, 10.1.0.1/24", "exactly one"},
		{"ipv6", "fd00::1/64", "IPv6"},
		{"bare ip", "10.0.0.1", "must be a CIDR"},
		{"bad table", "10.0.0.1/24\nTable = main", "Table"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			conf, err := ParseConf("[Interface]\nPrivateKey = " + k + "\nAddress = " + tc.addr + "\nListenPort = 51820\n")
			if err != nil {
				t.Fatal(err)
			}
			_, err = Translate("wg0", conf)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestLintHooks(t *testing.T) {
	flagged := []string{
		"iptables -A FORWARD -i %i -j ACCEPT",
		"iptables -t nat -I POSTROUTING -o eth0 -j MASQUERADE",
		"ip rule add iif %i table 100",
		"ip route add 10.0.0.0/8 dev %i",
	}
	for _, cmd := range flagged {
		if w := LintHooks("PostUp", []string{cmd}); len(w) != 1 || !strings.HasPrefix(w[0], "PostUp hook") {
			t.Errorf("%q: want one warning, got %v", cmd, w)
		}
	}
	clean := []string{
		"iptables -C FORWARD -i %i -j ACCEPT || iptables -A FORWARD -i %i -j ACCEPT",
		"iptables -D FORWARD -i %i -j ACCEPT",
		"ip rule del iif %i table 100 2>/dev/null; ip rule add iif %i table 100",
		"ip route replace 10.0.0.0/8 dev %i",
		"sysctl -w net.ipv4.ip_forward=1",
	}
	for _, cmd := range clean {
		if w := LintHooks("PostUp", []string{cmd}); len(w) != 0 {
			t.Errorf("%q: want no warning, got %v", cmd, w)
		}
	}
}

func TestParsePeerMap(t *testing.T) {
	k1, k2, k3 := genKey(t), genKey(t), genKey(t)
	text := "# users\nalice:\n  " + k1.String() + ": laptop\n\t" + k2.String() + ": phone   \n\"Bob Smith\":\n  " + k3.String() + ": 'bob pc'\n\nalice:\n"
	pm, err := ParsePeerMap(text)
	if err != nil {
		t.Fatalf("ParsePeerMap: %v", err)
	}
	want := &PeerMap{Users: []PeerMapUser{
		{Name: "alice", Peers: []PeerMapPeer{{PrivateKey: k1, Name: "laptop"}, {PrivateKey: k2, Name: "phone"}}},
		{Name: "Bob Smith", Peers: []PeerMapPeer{{PrivateKey: k3, Name: "bob pc"}}},
	}}
	if !reflect.DeepEqual(pm, want) {
		t.Fatalf("got %+v\nwant %+v", pm, want)
	}

	empty, err := ParsePeerMap("\n# nothing\n")
	if err != nil || len(empty.Users) != 0 {
		t.Fatalf("empty map = %+v, %v", empty, err)
	}
}

func TestParsePeerMapErrors(t *testing.T) {
	k := keyStr(genKey(t))
	cases := []struct{ name, text, want string }{
		{"peer before user", "  " + k + ": laptop\n", "before any user"},
		{"bad key", "alice:\n  notakey: laptop\n", "not a valid base64"},
		{"missing peer name", "alice:\n  " + k + ":\n", "peer name is required"},
		{"header with value", "alice: " + k + "\n", "must be `name:` on its own"},
		{"no colon", "alice\n", "expected a user header"},
		{"empty user", ":\n", "user name is empty"},
		{"duplicate key", "alice:\n  " + k + ": a\nbob:\n  " + k + ": b\n", "listed twice"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePeerMap(tc.text)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}
