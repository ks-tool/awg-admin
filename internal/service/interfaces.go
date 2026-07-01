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

// validateInterfaceConfig checks the fields the agent needs to bring the
// interface up, so a bad config is rejected up front instead of being stored
// and only surfacing later as a confusing best-effort push warning. The
// address is optional (CreateInterface auto-allocates one when it's blank —
// see defaultInterfaceAddress), but when supplied it must be a valid CIDR:
// otherwise it reaches the agent's netlink.ParseAddr and comes back as
// "agent returned 500: invalid CIDR address:".
func validateInterfaceConfig(cfg agentmodels.InterfaceConfig) error {
	if len(cfg.Interface) == 0 {
		return fmt.Errorf("interface name is required")
	}
	if cfg.Address != "" {
		if _, _, err := net.ParseCIDR(cfg.Address); err != nil {
			return fmt.Errorf("interface address %q must be a CIDR like 10.0.0.1/24", cfg.Address)
		}
	}
	return nil
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
	in.Peers = nil // peers managed via Peers API only
	// Generates a private key when none was supplied (and AWG obfuscation
	// params when not already set) — done here rather than only in
	// internal/api's HTTP handler so the Wails desktop App, which calls
	// this method directly and bypasses internal/api entirely, gets the
	// same behavior instead of persisting an all-zero private key.
	agentmodels.GenerateAmneziaParams(&in)
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
