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
	"errors"
	"uuid"

	"github.com/ks-tool/vpn-admin/internal/deploy"
	"github.com/ks-tool/vpn-admin/models"
	vpnagentpack "github.com/ks-tool/vpn-admin/pack/vpn-agent"
	"github.com/rs/zerolog/log"
)

func (s *Service) DeployAgentPack(id, agentArtifact string) error {
	debugOp("DeployAgentPack").Str("server_id", id).Msg("deploying agent")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}
	if agentArtifact == "" {
		defaults := &vpnagentpack.Config{}
		defaults.SetDefaults()
		agentArtifact = defaults.Artifact
	}

	s.deployStatus.start(sID)
	go func() {
		err := deploy.ToPack(context.Background(), *srv, agentArtifact, s.tunnels.PassphraseFor(sID), s.tunnels.SudoPasswordFor(sID), func(step string) {
			s.deployStatus.setStep(sID, step)
		})
		s.deployStatus.finish(sID, err)
		if err != nil {
			return
		}
		if err := s.SyncServer(id); err != nil {
			log.Warn().Err(err).Str("server_id", id).Msg("failed to sync server after pack deployment")
		}
	}()
	return nil
}

func (s *Service) DeployProvider(id string, provider models.ProviderDeployment) error {
	debugOp("DeployProvider").Str("server_id", id).Str("provider", provider.Name).Msg("deploying provider")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}
	if provider.Name == "" || provider.Artifact == "" {
		return errors.New("provider name and OCI artifact are required")
	}

	s.deployStatus.start(sID)
	s.deployStatus.setStep(sID, "install_provider_"+provider.Name)
	go func() {
		err := deploy.ProviderToPack(context.Background(), *srv, provider, s.serverInterfaceNames(sID), s.tunnels.PassphraseFor(sID), s.tunnels.SudoPasswordFor(sID), func(step string) {
			s.deployStatus.setStep(sID, step)
		})
		s.deployStatus.finish(sID, err)
		if err != nil {
			return
		}
		s.markFirewallApplied(sID)
		if err := s.SyncServer(id); err != nil {
			log.Warn().Err(err).Str("server_id", id).Str("provider", provider.Name).Msg("failed to sync server after provider deployment")
		}
	}()
	return nil
}
