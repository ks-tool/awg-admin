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

package awg

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeFirewallSystem struct {
	backend     string
	queryExists bool
	commands    []string
	content     string
}

func (f *fakeFirewallSystem) Run(name string, args ...string) error {
	f.commands = append(f.commands, name+" "+strings.Join(args, " "))
	return nil
}

func (f *fakeFirewallSystem) Output(name string, args ...string) (string, error) {
	command := name + " " + strings.Join(args, " ")
	if command == "systemctl is-active --quiet firewalld" || command == "pgrep -x firewalld" {
		if f.backend == "firewalld" {
			return "active", nil
		}
		return "", errors.New("not running")
	}
	if command == "ufw status" {
		if f.backend == "ufw" {
			return "Status: active", nil
		}
		return "Status: inactive", nil
	}
	f.commands = append(f.commands, command)
	if f.queryExists {
		return "yes", nil
	}
	return "", errors.New("rule not found")
}

func (f *fakeFirewallSystem) ReadFile(string) ([]byte, error) {
	return []byte(f.content), nil
}

func (f *fakeFirewallSystem) WriteFile(_ string, data []byte, _ os.FileMode) error {
	f.content = string(data)
	return nil
}

func TestFirewalldRulesAreRuntimeAndPermanent(t *testing.T) {
	system := &fakeFirewallSystem{backend: "firewalld"}
	requests := []firewallRequest{
		{kind: "interface", action: "add", iface: "awg0", listenPort: 51820},
		{kind: "transit", action: "add", iface: "awg0"},
		{kind: "exit", action: "add", iface: "awg0", mark: "0x1234"},
	}
	for _, request := range requests {
		if err := applyFirewallRequest(context.Background(), request, system); err != nil {
			t.Fatal(err)
		}
	}
	commands := strings.Join(system.commands, "\n")
	for _, expected := range []string{
		"firewall-cmd --permanent --new-policy=vpn-admin-forward",
		"firewall-cmd --permanent --policy=vpn-admin-forward --add-ingress-zone=trusted",
		"firewall-cmd --permanent --policy=vpn-admin-forward --add-egress-zone=ANY",
		"firewall-cmd --zone=trusted --change-interface=awg0",
		"firewall-cmd --permanent --zone=trusted --add-forward",
		"firewall-cmd --permanent --add-port=51820/udp",
		"firewall-cmd --permanent --direct --add-rule ipv4 nat POSTROUTING 0 -o awg0 -j MASQUERADE",
		"firewall-cmd --direct --add-rule ipv4 mangle PREROUTING 0 -i awg0 -j MARK --set-mark 0x1234",
		"firewall-cmd --permanent --direct --add-rule ipv4 nat POSTROUTING 0 ! -o awg0 -m mark --mark 0x1234 -j MASQUERADE",
	} {
		if !strings.Contains(commands, expected) {
			t.Errorf("missing command %q in:\n%s", expected, commands)
		}
	}
}

func TestUFWFiltersAndPersistentNetfilterRules(t *testing.T) {
	system := &fakeFirewallSystem{backend: "ufw", content: "*filter\nCOMMIT\n"}
	for _, request := range []firewallRequest{
		{kind: "interface", action: "add", iface: "awg0", listenPort: 51820},
		{kind: "transit", action: "add", iface: "awg0"},
		{kind: "exit", action: "add", iface: "awg0", mark: "0x1234"},
	} {
		if err := applyFirewallRequest(context.Background(), request, system); err != nil {
			t.Fatal(err)
		}
	}
	commands := strings.Join(system.commands, "\n")
	for _, expected := range []string{"ufw allow 51820/udp", "ufw allow in on awg0", "ufw route allow in on awg0"} {
		if !strings.Contains(commands, expected) {
			t.Errorf("missing command %q in:\n%s", expected, commands)
		}
	}
	for _, expected := range []string{"*nat", "*mangle", "# vpn-admin:transit-awg0", "# vpn-admin:exit-awg0", "-A POSTROUTING -o awg0 -j MASQUERADE", "-A PREROUTING -i awg0 -j MARK --set-mark 0x1234"} {
		if !strings.Contains(system.content, expected) {
			t.Errorf("missing persistent rule %q in:\n%s", expected, system.content)
		}
	}

	for _, request := range []firewallRequest{{kind: "transit", action: "remove", iface: "awg0"}, {kind: "exit", action: "remove", iface: "awg0", mark: "0x1234"}} {
		if err := applyFirewallRequest(context.Background(), request, system); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Contains(system.content, "# vpn-admin:") || strings.Contains(system.content, "MASQUERADE") || strings.Contains(system.content, "--set-mark") {
		t.Fatalf("owned rules survived removal:\n%s", system.content)
	}
	if !strings.HasPrefix(system.content, "*filter\nCOMMIT\n") {
		t.Fatalf("existing UFW filter rules were damaged:\n%s", system.content)
	}
}

func TestUFWRuleUpdateIsIdempotentAndPreservesForeignRule(t *testing.T) {
	rule := netfilterRule{id: "transit-awg0", table: "nat", chain: "POSTROUTING", args: []string{"-o", "awg0", "-j", "MASQUERADE"}}
	content := "*filter\nCOMMIT\n"
	updated, owned, changed := setUFWNetfilterRule(content, rule, true)
	if !owned || !changed {
		t.Fatal("new owned rule was not added")
	}
	repeated, owned, changed := setUFWNetfilterRule(updated, rule, true)
	if !owned || changed || repeated != updated {
		t.Fatal("repeated add changed persistent rules")
	}

	foreign := "*nat\n-A POSTROUTING -o awg0 -j MASQUERADE\nCOMMIT\n"
	updated, owned, changed = setUFWNetfilterRule(foreign, rule, true)
	if owned || changed || updated != foreign {
		t.Fatal("foreign rule was claimed")
	}
	updated, owned, changed = setUFWNetfilterRule(foreign, rule, false)
	if owned || changed || updated != foreign {
		t.Fatal("foreign rule was removed")
	}
}

func TestInterfaceRulesAllowZeroListenPort(t *testing.T) {
	rules := firewallNetfilterRules(firewallRequest{kind: "interface", iface: "awg0"})
	if len(rules) != 2 {
		t.Fatalf("zero listen port generated UDP rule: %+v", rules)
	}
	for _, rule := range rules {
		if strings.Contains(strings.Join(rule.args, " "), "--dport") {
			t.Fatalf("unexpected UDP rule: %+v", rule)
		}
	}
}
