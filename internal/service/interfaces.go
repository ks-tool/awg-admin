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

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// ValidationError marks an error as caused by bad user input (a malformed field
// or a uniqueness conflict) rather than an internal fault, so the HTTP layer can
// map it to a 4xx instead of a 500 (see internal/api's handleErr). Its message
// is the user-facing explanation.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// invalidInput builds a ValidationError with a formatted message.
func invalidInput(format string, args ...any) error {
	return &ValidationError{Msg: fmt.Sprintf(format, args...)}
}

// validateInterfaceConfig checks the fields the agent needs to bring the
// interface up, so a bad config is rejected up front instead of being stored
// and only surfacing later as a confusing best-effort push warning. The
// address is optional (CreateInterface auto-allocates one when it's blank —
// see defaultInterfaceAddress), but when supplied it must be a valid CIDR:
// otherwise it reaches the agent's netlink.ParseAddr and comes back as
// "agent returned 500: invalid CIDR address:".
func validateInterfaceConfig(cfg agentmodels.InterfaceConfig) error {
	if len(cfg.Interface) == 0 {
		return invalidInput("interface name is required")
	}
	if cfg.Address != "" {
		if _, _, err := net.ParseCIDR(cfg.Address); err != nil {
			return invalidInput("interface address %q must be a CIDR like 10.0.0.1/24", cfg.Address)
		}
	}
	return nil
}

// validateInterfaceUnique rejects a create/update whose name, listen port, or
// subnet collides with another interface ON THE SAME SERVER — the conflicts that
// would otherwise fail (or silently misbehave) on the agent's host: two links
// can't share a name or bind the same UDP port, and overlapping subnets break
// routing. Listen port 0 is exempt (it means "no fixed port" — an exit-node
// interface dials out, so several can legitimately have it). excludeID is the
// interface being updated, skipped in the scan; pass uuid.Nil on create.
func (s *Service) validateInterfaceUnique(sID uuid.UUID, cfg agentmodels.InterfaceConfig, excludeID uuid.UUID, old *models.Interface) error {
	existing, err := s.store.Servers().Interfaces(sID).List()
	if err != nil {
		return err
	}

	// On update, only re-validate a field that actually changed: an unchanged
	// name/port/subnet was already accepted, so re-checking it would wrongly
	// block an unrelated edit if a conflict slipped in via a bypass path
	// (BuildTunnel/ImportInterface persist directly) or predates this validation.
	checkName := old == nil || old.Interface != cfg.Interface
	checkPort := old == nil || old.ListenPort != cfg.ListenPort
	checkSubnet := old == nil || old.Address != cfg.Address

	var newNet *net.IPNet
	if checkSubnet && cfg.Address != "" {
		if _, newNet, err = net.ParseCIDR(cfg.Address); err != nil {
			return invalidInput("interface address %q must be a CIDR like 10.0.0.1/24", cfg.Address)
		}
	}

	for i := range existing {
		other := existing[i]
		if other.ID == excludeID {
			continue
		}
		if checkName && other.Interface == cfg.Interface {
			return invalidInput("interface name %q is already used on this server", cfg.Interface)
		}
		if checkPort && cfg.ListenPort != 0 && other.ListenPort == cfg.ListenPort {
			return invalidInput("listen port %d is already used by interface %q on this server", cfg.ListenPort, other.Interface)
		}
		if newNet != nil && other.Address != "" {
			if _, otherNet, perr := net.ParseCIDR(other.Address); perr == nil && netsOverlap(newNet, otherNet) {
				return invalidInput("subnet %s overlaps interface %q (%s) on this server", cfg.Address, other.Interface, other.Address)
			}
		}
	}
	return nil
}

// netsOverlap reports whether two IP networks intersect — true when either one
// contains the other's base address.
func netsOverlap(a, b *net.IPNet) bool {
	return a.Contains(b.IP) || b.Contains(a.IP)
}

// defaultInterfaceAddress allocates the first host address (.1) of the next
// free /24 in the auto pool (172.23.0.0/16, shared with the tunnel wizard —
// see nextFreeSubnet), used when an interface is created without an explicit
// address. A WireGuard interface can't come up without one, and picking a free
// subnet automatically is friendlier than forcing the admin to hand-pick a
// non-overlapping range.
func (s *Service) defaultInterfaceAddress() (string, error) {
	subnet, err := s.nextFreeSubnet()
	if err != nil {
		return "", err
	}
	ip, ipnet, err := net.ParseCIDR(subnet)
	if err != nil {
		return "", err
	}
	ones, _ := ipnet.Mask.Size()
	incIP(ip) // network .0 -> first usable host .1
	return fmt.Sprintf("%s/%d", ip.String(), ones), nil
}

func (s *Service) ListInterfaces(serverID string) ([]models.Interface, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	return s.store.Servers().Interfaces(sID).List()
}

func (s *Service) GetInterface(serverID, ifaceID string) (*models.Interface, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	iID, err := uuid.Parse(ifaceID)
	if err != nil {
		return nil, err
	}
	return s.store.Servers().Interfaces(sID).Get(iID)
}

func (s *Service) CreateInterface(serverID string, in agentmodels.InterfaceConfig) (*models.Interface, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}

	if err := validateInterfaceConfig(in); err != nil {
		return nil, err
	}
	if in.Address == "" {
		if in.Address, err = s.defaultInterfaceAddress(); err != nil {
			return nil, err
		}
	}
	if err := s.validateInterfaceUnique(sID, in, uuid.Nil, nil); err != nil {
		return nil, err
	}
	in.Peers = nil // peers managed via Peers API only
	// Every interface needs a private key; generate one when the caller didn't
	// supply it. Done here rather than only in internal/api's HTTP handler so
	// the Wails desktop App, which calls this method directly and bypasses
	// internal/api entirely, gets the same behavior instead of persisting an
	// all-zero private key. The AmneziaWG obfuscation params
	// (jc/jmin/jmax/s1..s4/h1..h4/i1..i5) are NOT auto-generated here: the
	// caller decides. The UI's "Amnezia Interface" toggle sends a generated set
	// (see GenerateInterfaceDefaults) for an Amnezia interface, or omits them
	// entirely for a plain WireGuard one.
	if agentmodels.IsEmpty(in.PrivateKey) {
		if in.PrivateKey, err = agentmodels.GeneratePrivateKey(); err != nil {
			return nil, fmt.Errorf("generate key: %w", err)
		}
	}
	iface := &models.Interface{
		ID:              uuid.New(),
		InterfaceConfig: in,
	}
	if err = s.store.Servers().Interfaces(sID).Set(iface); err != nil {
		return nil, err
	}
	s.pushInterface(sID, iface)
	return iface, nil
}

// GenerateInterfaceDefaults returns a fresh InterfaceConfig populated with a
// generated private key and a full set of AmneziaWG obfuscation parameters. The
// add-interface form calls it to pre-fill the "Amnezia" tab; it touches no
// storage and reaches no agent, so it's safe to call on every modal open.
func (s *Service) GenerateInterfaceDefaults() agentmodels.InterfaceConfig {
	var cfg agentmodels.InterfaceConfig
	agentmodels.GenerateAmneziaParams(&cfg)
	return cfg
}

// UpdateInterfaceConfig replaces all config fields but always preserves peers.
func (s *Service) UpdateInterfaceConfig(serverID, ifaceID string, cfg agentmodels.InterfaceConfig) (*models.Interface, error) {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return nil, err
	}
	iID, err := uuid.Parse(ifaceID)
	if err != nil {
		return nil, err
	}
	if err := validateInterfaceConfig(cfg); err != nil {
		return nil, err
	}

	ifaces := s.store.Servers().Interfaces(sID)
	iface, err := ifaces.Get(iID)
	if err != nil {
		return nil, err
	}
	// A blank address on edit keeps the interface's current one rather than
	// moving it to a freshly-allocated subnet (which would orphan every peer's
	// AllowedIPs); only auto-allocate if there was nothing to keep.
	if cfg.Address == "" {
		cfg.Address = iface.Address
	}
	if cfg.Address == "" {
		if cfg.Address, err = s.defaultInterfaceAddress(); err != nil {
			return nil, err
		}
	}
	if err := s.validateInterfaceUnique(sID, cfg, iID, iface); err != nil {
		return nil, err
	}
	cfg.Peers = iface.Peers // immutable via this endpoint
	iface.InterfaceConfig = cfg
	if err = ifaces.Set(iface); err != nil {
		return nil, err
	}
	s.pushInterface(sID, iface)
	return iface, nil
}

func (s *Service) DeleteInterface(serverID, ifaceID string) error {
	sID, err := uuid.Parse(serverID)
	if err != nil {
		return err
	}
	iID, err := uuid.Parse(ifaceID)
	if err != nil {
		return err
	}

	ifaces := s.store.Servers().Interfaces(sID)
	iface, err := ifaces.Get(iID)
	if err != nil {
		return err
	}
	// An interface that's part of a tunnel can't be deleted on its own —
	// remove the tunnel first (see Service.RemoveTunnel). The tunnel id isn't
	// surfaced in the UI, so this is what stops the user from breaking a tunnel
	// by deleting one of its interfaces.
	if iface.Tunnel != nil {
		return fmt.Errorf("interface %q is part of a tunnel; remove the tunnel first", iface.Interface)
	}
	if err = ifaces.Delete(iID); err != nil {
		return err
	}
	s.pushInterfaceDelete(sID, iface.Interface)
	return nil
}
