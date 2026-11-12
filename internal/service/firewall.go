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
	"context"
	"strings"
	"uuid"

	"github.com/rs/zerolog/log"
)

func firewallSignature(names []string) string {
	return strings.Join(names, "\x00")
}

func (s *Service) markFirewallApplied(serverID uuid.UUID) {
	id := serverID.String()
	signature := firewallSignature(s.serverInterfaceNames(serverID))
	s.firewallMu.Lock()
	s.firewallApplied[id] = signature
	if s.firewallDesired[id] == signature {
		delete(s.firewallDesired, id)
	}
	s.firewallMu.Unlock()
}

func (s *Service) scheduleFirewallSync(serverID uuid.UUID) {
	id := serverID.String()
	signature := firewallSignature(s.serverInterfaceNames(serverID))
	s.firewallMu.Lock()
	s.firewallDesired[id] = signature
	if s.firewallRunning[id] {
		s.firewallMu.Unlock()
		return
	}
	if s.firewallApplied[id] == signature {
		delete(s.firewallDesired, id)
		s.firewallMu.Unlock()
		return
	}
	s.firewallRunning[id] = true
	s.firewallMu.Unlock()
	go s.runFirewallSync(serverID)
}

func (s *Service) runFirewallSync(serverID uuid.UUID) {
	id := serverID.String()
	for {
		s.firewallMu.Lock()
		signature := s.firewallDesired[id]
		unchanged := s.firewallApplied[id] == signature
		s.firewallMu.Unlock()
		if !unchanged {
			srv, err := s.store.Servers().Get(serverID)
			if err == nil {
				err = s.applyFirewall(context.Background(), *srv, s.serverInterfaceNames(serverID), s.tunnels.PassphraseFor(serverID), s.tunnels.SudoPasswordFor(serverID))
			}
			if err != nil {
				log.Warn().Err(err).Str("server_id", id).Msg("failed to update VPN firewall")
			} else {
				s.firewallMu.Lock()
				s.firewallApplied[id] = signature
				s.firewallMu.Unlock()
			}
		}

		s.firewallMu.Lock()
		if s.firewallDesired[id] != signature {
			s.firewallMu.Unlock()
			continue
		}
		delete(s.firewallRunning, id)
		delete(s.firewallDesired, id)
		s.firewallMu.Unlock()
		return
	}
}
