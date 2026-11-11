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

package proxy

import (
	"errors"
	"strings"
	"testing"
)

type fakeFirewallSystem struct {
	backend string
	queries bool
	calls   []string
}

func (f *fakeFirewallSystem) Output(name string, args ...string) (string, error) {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if name == "systemctl" && f.backend == "firewalld" {
		return "", nil
	}
	if name == "ufw" && f.backend == "ufw" {
		return "Status: active", nil
	}
	if name == "firewall-cmd" && strings.Contains(call, "--query-port") {
		if f.queries {
			return "", nil
		}
		return "", errors.New("missing")
	}
	return "", errors.New("missing")
}

func (f *fakeFirewallSystem) Run(name string, args ...string) error {
	call := name + " " + strings.Join(args, " ")
	f.calls = append(f.calls, call)
	if name == "iptables" && strings.HasPrefix(call, "iptables -C ") && !f.queries {
		return errors.New("missing")
	}
	return nil
}

func TestListenerFirewallBackends(t *testing.T) {
	for _, backend := range []string{"firewalld", "ufw", "iptables"} {
		t.Run(backend, func(t *testing.T) {
			system := &fakeFirewallSystem{backend: backend}
			firewall := &listenerFirewall{system: system}
			if err := firewall.apply(443, "tcp", true); err != nil {
				t.Fatal(err)
			}
			joined := strings.Join(system.calls, "\n")
			switch backend {
			case "firewalld":
				if !strings.Contains(joined, "--add-port=443/tcp") || !strings.Contains(joined, "--permanent") {
					t.Fatalf("missing firewalld rules: %s", joined)
				}
			case "ufw":
				if !strings.Contains(joined, "ufw allow 443/tcp") {
					t.Fatalf("missing ufw rule: %s", joined)
				}
			case "iptables":
				if !strings.Contains(joined, "iptables -A INPUT -p tcp --dport 443 -j ACCEPT") {
					t.Fatalf("missing iptables rule: %s", joined)
				}
			}
		})
	}
}

func TestListenerFirewallRemoval(t *testing.T) {
	system := &fakeFirewallSystem{backend: "firewalld", queries: true}
	firewall := &listenerFirewall{system: system}
	if err := firewall.apply(8443, "udp", false); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(system.calls, "\n")
	if !strings.Contains(joined, "--remove-port=8443/udp") {
		t.Fatalf("missing removal: %s", joined)
	}
}
