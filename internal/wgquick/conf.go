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

// Package wgquick parses wg-quick / awg-quick configuration files and the
// user→peer map that accompanies an import, and translates them into the
// agent's InterfaceConfig — including the routes wg-quick would have added
// implicitly, which the agent (which doesn't run wg-quick) needs as explicit
// hook commands. Pure: no storage, no network; see internal/service's import
// for the orchestration around it.
package wgquick

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
)

// Conf is a parsed wg-quick/awg-quick configuration file: one [Interface]
// section and any number of [Peer] sections, in file order.
type Conf struct {
	Interface InterfaceSection
	Peers     []PeerSection
}

// InterfaceSection holds the [Interface] keys. Pointers distinguish "not set"
// from a zero value; multi-valued keys (Address, DNS, hooks) accumulate across
// repeated lines the way wg-quick treats them.
type InterfaceSection struct {
	PrivateKey agentmodels.Key
	Address    []string
	ListenPort *int
	MTU        *int
	DNS        []string
	FwMark     *int
	// Table is the raw value ("", "auto", "off" or a table number); Translate
	// interprets it.
	Table      string
	SaveConfig *bool

	PreUp, PostUp, PreDown, PostDown []string

	// AmneziaWG obfuscation parameters, as awg-quick accepts them.
	Jc, Jmin, Jmax, S1, S2, S3, S4     *int
	H1, H2, H3, H4, I1, I2, I3, I4, I5 *string
}

// PeerSection holds one [Peer] section's keys.
type PeerSection struct {
	PublicKey    agentmodels.Key
	PresharedKey *agentmodels.Key
	// AllowedIPs are normalized CIDRs (a bare address becomes /32 or /128).
	AllowedIPs []string
	Endpoint   string
	// PersistentKeepalive in seconds; nil when the key is absent, 0 for "off".
	PersistentKeepalive *int
}

// ParseConf parses text as a wg-quick/awg-quick configuration. It's a strict
// reader of the INI subset both tools accept: `#` starts a comment anywhere on
// a line (as wg-quick strips it), keys are case-insensitive, and unknown keys
// are an error rather than ignored — a misspelt key would otherwise silently
// drop a setting from the imported interface. The required keys (PrivateKey,
// Address, ListenPort; per peer PublicKey and AllowedIPs) are checked here
// too, so a Conf that comes back non-nil is complete.
func ParseConf(text string) (*Conf, error) {
	conf := &Conf{}
	const (
		none = iota
		sectInterface
		sectPeer
	)
	section := none
	seenInterface := false
	seenScalar := map[string]bool{} // per section: duplicate-scalar detection
	var peer *PeerSection

	for i, raw := range strings.Split(text, "\n") {
		lineNo := i + 1
		line := raw
		if idx := strings.IndexByte(line, '#'); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "[") {
			if !strings.HasSuffix(line, "]") {
				return nil, fmt.Errorf("line %d: malformed section header %q", lineNo, line)
			}
			seenScalar = map[string]bool{}
			switch strings.ToLower(strings.TrimSpace(line[1 : len(line)-1])) {
			case "interface":
				if seenInterface {
					return nil, fmt.Errorf("line %d: duplicate [Interface] section", lineNo)
				}
				seenInterface = true
				section = sectInterface
			case "peer":
				conf.Peers = append(conf.Peers, PeerSection{})
				peer = &conf.Peers[len(conf.Peers)-1]
				section = sectPeer
			default:
				return nil, fmt.Errorf("line %d: unknown section %s", lineNo, line)
			}
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: expected `Key = value`, got %q", lineNo, line)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNo)
		}
		lkey := strings.ToLower(key)

		var err error
		switch section {
		case none:
			return nil, fmt.Errorf("line %d: %s outside of a section", lineNo, key)
		case sectInterface:
			err = conf.Interface.set(lkey, key, value, seenScalar)
		case sectPeer:
			err = peer.set(lkey, key, value, seenScalar)
		}
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", lineNo, err)
		}
	}

	if !seenInterface {
		return nil, fmt.Errorf("no [Interface] section")
	}
	return conf, conf.validate()
}

// scalar records a single-valued key, rejecting a repeat: wg-quick would let
// the last one win, but a duplicated key is far more likely a paste mistake.
func scalar(seen map[string]bool, key string) error {
	if seen[key] {
		return fmt.Errorf("duplicate key %s", key)
	}
	seen[key] = true
	return nil
}

func (s *InterfaceSection) set(lkey, key, value string, seen map[string]bool) error {
	switch lkey {
	case "address":
		s.Address = append(s.Address, splitList(value)...)
		return nil
	case "dns":
		s.DNS = append(s.DNS, splitList(value)...)
		return nil
	case "preup":
		s.PreUp = append(s.PreUp, value)
		return nil
	case "postup":
		s.PostUp = append(s.PostUp, value)
		return nil
	case "predown":
		s.PreDown = append(s.PreDown, value)
		return nil
	case "postdown":
		s.PostDown = append(s.PostDown, value)
		return nil
	}

	if err := scalar(seen, lkey); err != nil {
		return err
	}
	var err error
	switch lkey {
	case "privatekey":
		s.PrivateKey, err = parseKey(key, value)
	case "listenport":
		s.ListenPort, err = parseIntRange(key, value, 1, 65535)
	case "mtu":
		s.MTU, err = parseIntRange(key, value, 1, 65535)
	case "fwmark":
		s.FwMark, err = parseFwMark(value)
	case "table":
		s.Table = value
	case "saveconfig":
		s.SaveConfig, err = parseBool(key, value)
	case "jc":
		s.Jc, err = parseIntRange(key, value, 0, 1<<16)
	case "jmin":
		s.Jmin, err = parseIntRange(key, value, 0, 1<<16)
	case "jmax":
		s.Jmax, err = parseIntRange(key, value, 0, 1<<16)
	case "s1":
		s.S1, err = parseIntRange(key, value, 0, 1<<16)
	case "s2":
		s.S2, err = parseIntRange(key, value, 0, 1<<16)
	case "s3":
		s.S3, err = parseIntRange(key, value, 0, 1<<16)
	case "s4":
		s.S4, err = parseIntRange(key, value, 0, 1<<16)
	case "h1":
		s.H1 = new(value)
	case "h2":
		s.H2 = new(value)
	case "h3":
		s.H3 = new(value)
	case "h4":
		s.H4 = new(value)
	case "i1":
		s.I1 = new(value)
	case "i2":
		s.I2 = new(value)
	case "i3":
		s.I3 = new(value)
	case "i4":
		s.I4 = new(value)
	case "i5":
		s.I5 = new(value)
	default:
		return fmt.Errorf("unknown [Interface] key %s", key)
	}
	return err
}

func (p *PeerSection) set(lkey, key, value string, seen map[string]bool) error {
	if lkey == "allowedips" {
		for _, raw := range splitList(value) {
			cidr, err := normalizeCIDR(raw)
			if err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			p.AllowedIPs = append(p.AllowedIPs, cidr)
		}
		return nil
	}

	if err := scalar(seen, lkey); err != nil {
		return err
	}
	var err error
	switch lkey {
	case "publickey":
		p.PublicKey, err = parseKey(key, value)
	case "presharedkey":
		var k agentmodels.Key
		if k, err = parseKey(key, value); err == nil {
			p.PresharedKey = &k
		}
	case "endpoint":
		if _, _, err = net.SplitHostPort(value); err != nil {
			return fmt.Errorf("%s %q must be host:port", key, value)
		}
		p.Endpoint = value
	case "persistentkeepalive":
		if strings.EqualFold(value, "off") {
			p.PersistentKeepalive = new(0)
		} else {
			p.PersistentKeepalive, err = parseIntRange(key, value, 0, 65535)
		}
	default:
		return fmt.Errorf("unknown [Peer] key %s", key)
	}
	return err
}

// validate enforces the keys the import can't do without.
func (c *Conf) validate() error {
	if agentmodels.IsEmpty(c.Interface.PrivateKey) {
		return fmt.Errorf("[Interface] PrivateKey is required")
	}
	if len(c.Interface.Address) == 0 {
		return fmt.Errorf("[Interface] Address is required")
	}
	if c.Interface.ListenPort == nil {
		return fmt.Errorf("[Interface] ListenPort is required (without it no client Endpoint can be rendered)")
	}
	seen := make(map[agentmodels.Key]int, len(c.Peers))
	for i, p := range c.Peers {
		n := i + 1
		if agentmodels.IsEmpty(p.PublicKey) {
			return fmt.Errorf("[Peer] #%d: PublicKey is required", n)
		}
		if len(p.AllowedIPs) == 0 {
			return fmt.Errorf("[Peer] #%d (%s): AllowedIPs is required", n, shortKey(p.PublicKey))
		}
		if prev, dup := seen[p.PublicKey]; dup {
			return fmt.Errorf("[Peer] #%d repeats the PublicKey of [Peer] #%d (%s)", n, prev, shortKey(p.PublicKey))
		}
		seen[p.PublicKey] = n
	}
	return nil
}

// ─────────────────────────────── value parsers ─────────────────────────────

func splitList(value string) []string {
	var out []string
	for _, part := range strings.Split(value, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}

func parseKey(key, value string) (agentmodels.Key, error) {
	k, err := agentmodels.ParseKey(value)
	if err != nil {
		return agentmodels.Key{}, fmt.Errorf("%s: not a valid base64 WireGuard key", key)
	}
	return k, nil
}

func parseIntRange(key, value string, min, max int) (*int, error) {
	n, err := strconv.Atoi(value)
	if err != nil || n < min || n > max {
		return nil, fmt.Errorf("%s %q must be an integer between %d and %d", key, value, min, max)
	}
	return &n, nil
}

// parseFwMark accepts wg's forms: decimal, 0x-hex, or "off" (= 0).
func parseFwMark(value string) (*int, error) {
	if strings.EqualFold(value, "off") {
		return new(0), nil
	}
	n, err := strconv.ParseInt(value, 0, 32)
	if err != nil || n < 0 {
		return nil, fmt.Errorf("FwMark %q must be a non-negative integer (decimal or 0x hex) or off", value)
	}
	return new(int(n)), nil
}

func parseBool(key, value string) (*bool, error) {
	switch strings.ToLower(value) {
	case "true":
		return new(true), nil
	case "false":
		return new(false), nil
	}
	return nil, fmt.Errorf("%s %q must be true or false", key, value)
}

// normalizeCIDR parses an AllowedIPs entry — a CIDR or a bare address (which
// wg treats as a /32 or /128) — into canonical CIDR text with the host bits
// masked off, as wg itself stores it.
func normalizeCIDR(raw string) (string, error) {
	if _, ipnet, err := net.ParseCIDR(raw); err == nil {
		return ipnet.String(), nil
	}
	ip := net.ParseIP(raw)
	if ip == nil {
		return "", fmt.Errorf("%q is not an IP address or CIDR", raw)
	}
	if v4 := ip.To4(); v4 != nil {
		return v4.String() + "/32", nil
	}
	return ip.String() + "/128", nil
}

// shortKey abbreviates a key for messages the way the UI does.
func shortKey(k agentmodels.Key) string {
	s := k.String()
	if len(s) > 8 {
		return s[:8] + "…"
	}
	return s
}
