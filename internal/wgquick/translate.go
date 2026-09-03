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
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
)

// Translation is the agent-side config derived from a wg-quick conf, with the
// pieces the admin shows separately in the import preview.
type Translation struct {
	// Config is the complete InterfaceConfig to store and push. Its PostUp /
	// PreDown already include GeneratedPostUp / GeneratedPreDown, appended after
	// the conf's own hooks.
	Config agentmodels.InterfaceConfig
	// GeneratedPostUp / GeneratedPreDown reproduce what wg-quick did implicitly
	// (see Translate) — kept apart so the preview can show them as a block.
	GeneratedPostUp  []string
	GeneratedPreDown []string
	// Warnings are non-blocking findings for the operator.
	Warnings []string
}

// Translate maps a parsed conf onto the agent's InterfaceConfig for the named
// interface, encoding as explicit hook commands what wg-quick used to do
// behind the scenes (the agent doesn't run wg-quick):
//
//   - routes: wg-quick adds a route for every peer AllowedIPs prefix. With the
//     default Table (empty/"auto") a prefix already covered by the interface's
//     own connected route (its subnet) needs nothing, everything else gets
//     `ip route replace <cidr> dev %i`. With `Table = N` every prefix is routed
//     in table N (there's no connected route there), and Table itself is
//     carried over. `Table = off` adds no routes. A default route (/0) under
//     Table=auto — wg-quick's fwmark policy routing — isn't translated and is
//     reported as a warning.
//   - the generated routes are idempotent (`replace`, and `del … || true`) so
//     the agent's teardown-then-setup on every config update is a no-op for
//     unchanged rules.
//
// Hard errors are the things the admin's model can't represent: more than one
// Address, an IPv6 Address, a non-numeric Table. Everything else that changes
// meaning (DNS becoming the peers' default client DNS, SaveConfig, hooks that
// aren't idempotent, peers without keepalive) is a warning.
func Translate(name string, conf *Conf) (*Translation, error) {
	sec := conf.Interface
	if len(sec.Address) != 1 {
		return nil, fmt.Errorf("[Interface] Address must be exactly one IPv4 CIDR, got %d (%s); multi-address interfaces aren't supported", len(sec.Address), strings.Join(sec.Address, ", "))
	}
	ip, subnet, err := net.ParseCIDR(sec.Address[0])
	if err != nil {
		return nil, fmt.Errorf("[Interface] Address %q must be a CIDR like 10.0.0.1/24", sec.Address[0])
	}
	if ip.To4() == nil {
		return nil, fmt.Errorf("[Interface] Address %q is IPv6; only IPv4 interfaces are supported", sec.Address[0])
	}

	table, tableMode, err := parseTable(sec.Table)
	if err != nil {
		return nil, err
	}

	tr := &Translation{}
	cfg := agentmodels.InterfaceConfig{
		Interface:  name,
		PrivateKey: sec.PrivateKey,
		ListenPort: uint16(*sec.ListenPort),
		Address:    sec.Address[0],
		DNS:        sec.DNS,
		Table:      table,
		PreUp:      sec.PreUp,
		PostUp:     sec.PostUp,
		PreDown:    sec.PreDown,
		PostDown:   sec.PostDown,
		Jc:         sec.Jc, Jmin: sec.Jmin, Jmax: sec.Jmax,
		S1: sec.S1, S2: sec.S2, S3: sec.S3, S4: sec.S4,
		H1: sec.H1, H2: sec.H2, H3: sec.H3, H4: sec.H4,
		I1: sec.I1, I2: sec.I2, I3: sec.I3, I4: sec.I4, I5: sec.I5,
	}
	if sec.MTU != nil {
		cfg.MTU = *sec.MTU
	}
	if sec.FwMark != nil {
		cfg.FirewallMark = sec.FwMark
	}

	noKeepalive := 0
	for _, p := range conf.Peers {
		peer := agentmodels.InterfacePeer{
			Key:          p.PublicKey,
			PresharedKey: p.PresharedKey,
			AllowedIPs:   p.AllowedIPs,
			Endpoint:     p.Endpoint,
		}
		if p.PersistentKeepalive != nil && *p.PersistentKeepalive > 0 {
			peer.KeepaliveInterval = time.Duration(*p.PersistentKeepalive) * time.Second
		} else {
			noKeepalive++
		}
		cfg.Peers = append(cfg.Peers, peer)
	}

	// Implicit routes → explicit hooks.
	subnetOnes, _ := subnet.Mask.Size()
	seenRoute := map[string]bool{}
	for _, p := range conf.Peers {
		for _, cidr := range p.AllowedIPs {
			if seenRoute[cidr] {
				continue
			}
			_, ipnet, _ := net.ParseCIDR(cidr)
			ones, bits := ipnet.Mask.Size()
			switch tableMode {
			case tableOff:
				continue
			case tableNumber:
				// no connected route in table N — route everything there
			default: // auto
				if ones == 0 {
					tr.Warnings = append(tr.Warnings, fmt.Sprintf("peer %s routes %s: wg-quick's default-route policy routing (fwmark + `ip rule`) isn't translated; add the equivalent hook commands by hand if this interface is meant to carry a default route", shortKey(p.PublicKey), cidr))
					continue
				}
				if bits == 32 && subnet.Contains(ipnet.IP) && ones >= subnetOnes {
					continue // covered by the interface's connected route, as wg-quick skips it
				}
			}
			seenRoute[cidr] = true
			suffix := ""
			if tableMode == tableNumber {
				suffix = " table " + strconv.Itoa(table)
			}
			tr.GeneratedPostUp = append(tr.GeneratedPostUp, fmt.Sprintf("ip route replace %s dev %%i%s", cidr, suffix))
			tr.GeneratedPreDown = append(tr.GeneratedPreDown, fmt.Sprintf("ip route del %s dev %%i%s 2>/dev/null || true", cidr, suffix))
		}
	}
	cfg.PostUp = append(cfg.PostUp, tr.GeneratedPostUp...)
	cfg.PreDown = append(cfg.PreDown, tr.GeneratedPreDown...)
	tr.Config = cfg

	// Non-blocking findings.
	if len(sec.DNS) > 0 {
		tr.Warnings = append(tr.Warnings, fmt.Sprintf("[Interface] DNS (%s) changes meaning: wg-quick set it as the host's resolver, awg-admin uses it as the default client DNS rendered into this interface's peer configs", strings.Join(sec.DNS, ", ")))
	}
	if sec.SaveConfig != nil {
		tr.Warnings = append(tr.Warnings, "SaveConfig is ignored: awg-admin's database is the source of truth after import; make sure the wg-quick unit is disabled so it can't rewrite the conf on stop")
	}
	if noKeepalive > 0 {
		tr.Warnings = append(tr.Warnings, fmt.Sprintf("%d peer(s) have no PersistentKeepalive (kept as 0, not the default 10s awg-admin gives new peers); peers behind NAT may become unreachable", noKeepalive))
	}
	tr.Warnings = append(tr.Warnings, LintHooks("PreUp", sec.PreUp)...)
	tr.Warnings = append(tr.Warnings, LintHooks("PostUp", sec.PostUp)...)
	tr.Warnings = append(tr.Warnings, LintHooks("PreDown", sec.PreDown)...)
	tr.Warnings = append(tr.Warnings, LintHooks("PostDown", sec.PostDown)...)
	return tr, nil
}

const (
	tableAuto = iota
	tableOff
	tableNumber
)

// parseTable interprets wg-quick's Table: empty/"auto" (default behaviour),
// "off" (no routes), or a routing table number.
func parseTable(raw string) (int, int, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return 0, tableAuto, nil
	case "off":
		return 0, tableOff, nil
	}
	n, err := strconv.ParseInt(strings.TrimSpace(raw), 0, 32)
	if err != nil || n <= 0 {
		return 0, 0, fmt.Errorf("[Interface] Table %q must be auto, off or a positive routing table number", raw)
	}
	return int(n), tableNumber, nil
}

var (
	iptablesAppend = regexp.MustCompile(`\b(ip6?tables(?:-legacy|-nft)?)\b[^;&|]*\s-(?:A|I|--append|--insert)\s`)
	iptablesCheck  = regexp.MustCompile(`\b(ip6?tables(?:-legacy|-nft)?)\b[^;&|]*\s(?:-C|--check)\s`)
	ipRuleAdd      = regexp.MustCompile(`\bip\s+(?:-\d\s+)?rule\s+add\b`)
	ipRuleDel      = regexp.MustCompile(`\bip\s+(?:-\d\s+)?rule\s+del(?:ete)?\b`)
	ipRouteAdd     = regexp.MustCompile(`\bip\s+(?:-\d\s+)?route\s+add\b`)
)

// LintHooks flags hook commands that aren't idempotent. It matters twice for
// an imported interface: on adoption the hooks run on top of rules wg-quick
// already applied, and the agent re-runs the old config's down-hooks plus the
// new config's up-hooks on every later update — so an `iptables -A` without a
// `-C` check duplicates its rule each time, and an `ip rule add` without a
// preceding `del` stacks rules. Heuristic (regex on the command text); returns
// one warning per offending command, phase-prefixed.
func LintHooks(phase string, cmds []string) []string {
	var out []string
	for _, cmd := range cmds {
		switch {
		case iptablesAppend.MatchString(cmd) && !iptablesCheck.MatchString(cmd):
			out = append(out, fmt.Sprintf("%s hook %q is not idempotent: an iptables -A/-I without a -C check duplicates the rule on every re-apply — use `iptables -C … || iptables -A …`", phase, cmd))
		case ipRuleAdd.MatchString(cmd) && !ipRuleDel.MatchString(cmd):
			out = append(out, fmt.Sprintf("%s hook %q is not idempotent: `ip rule add` stacks a duplicate rule on every re-apply — prefix it with `ip rule del … 2>/dev/null;`", phase, cmd))
		case ipRouteAdd.MatchString(cmd):
			out = append(out, fmt.Sprintf("%s hook %q is not idempotent: `ip route add` fails when the route already exists — use `ip route replace`", phase, cmd))
		}
	}
	return out
}
