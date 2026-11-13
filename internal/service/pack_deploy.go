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
	"fmt"
	"os"
	"path/filepath"
	"time"
	"uuid"

	"github.com/ks-tool/vpn-admin/internal/deploy"
	"github.com/ks-tool/vpn-admin/models"
	vpnagentpack "github.com/ks-tool/vpn-admin/pack/vpn-agent"
	"github.com/rs/zerolog/log"
)

func (s *Service) DeployAgentPack(id string, deployment models.AgentDeployment) error {
	debugOp("DeployAgentPack").Str("server_id", id).Msg("deploying agent")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}
	if len(deployment.Artifact) == 0 && len(deployment.BinaryPath) == 0 {
		defaults := &vpnagentpack.Config{}
		defaults.SetDefaults()
		deployment.Artifact = defaults.Artifact
	}
	if (len(deployment.Artifact) == 0) == (len(deployment.BinaryPath) == 0) {
		return errors.New("select exactly one agent OCI artifact or local binary")
	}
	if len(deployment.BinaryPath) > 0 {
		info, err := os.Stat(deployment.BinaryPath)
		if err != nil {
			return fmt.Errorf("read local agent binary: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return errors.New("local agent binary must be a non-empty regular file")
		}
	}
	deployment.Registry, err = s.resolveRegistry(deployment.Registry)
	if err != nil {
		return err
	}
	if err := s.configureAgentDeployment(srv, deployment.Settings); err != nil {
		return err
	}

	s.deployStatus.start(sID)
	go func() {
		err := deploy.ToPack(context.Background(), *srv, deployment, s.tunnels.PassphraseFor(sID), s.tunnels.SudoPasswordFor(sID), func(step string) {
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

func (s *Service) DeployProvider(id string, provider models.ProviderDeployment, registry models.Registry) error {
	debugOp("DeployProvider").Str("server_id", id).Str("provider", provider.Name).Msg("deploying provider")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}
	if len(provider.BinaryPath) > 0 {
		const prefix = "vpn-provider-"
		base := filepath.Base(provider.BinaryPath)
		if !filepath.IsAbs(provider.BinaryPath) || len(base) <= len(prefix) || base[:len(prefix)] != prefix {
			return errors.New("local provider binary must be an absolute path to vpn-provider-<name>")
		}
		provider.Name = base[len(prefix):]
		info, err := os.Stat(provider.BinaryPath)
		if err != nil {
			return fmt.Errorf("read local provider binary: %w", err)
		}
		if !info.Mode().IsRegular() || info.Size() == 0 {
			return errors.New("local provider binary must be a non-empty regular file")
		}
	}
	if len(provider.Name) == 0 || (len(provider.Artifact) == 0) == (len(provider.BinaryPath) == 0) {
		return errors.New("provider name and exactly one OCI artifact or local binary are required")
	}
	registry, err = s.resolveRegistry(registry)
	if err != nil {
		return err
	}

	s.deployStatus.start(sID)
	s.deployStatus.setStep(sID, "install_provider_"+provider.Name)
	go func() {
		err := deploy.ProviderToPack(context.Background(), *srv, provider, registry, s.tunnels.PassphraseFor(sID), s.tunnels.SudoPasswordFor(sID), func(step string) {
			s.deployStatus.setStep(sID, step)
		})
		if err == nil {
			err = s.waitForProvider(id, provider.Name, 20*time.Second)
		}
		s.deployStatus.finish(sID, err)
		if err != nil {
			return
		}
		if err := s.SyncServer(id); err != nil {
			log.Warn().Err(err).Str("server_id", id).Str("provider", provider.Name).Msg("failed to sync server after provider deployment")
		}
	}()
	return nil
}

func (s *Service) RemoveProvider(id, name string) error {
	debugOp("RemoveProvider").Str("server_id", id).Str("provider", name).Msg("removing provider")
	sID, err := uuid.Parse(id)
	if err != nil {
		return err
	}
	srv, err := s.store.Servers().Get(sID)
	if err != nil {
		return err
	}
	s.deployStatus.start(sID)
	s.deployStatus.setStep(sID, "remove_provider_"+name)
	go func() {
		err := deploy.RemoveProviderFromPack(context.Background(), *srv, name, s.tunnels.PassphraseFor(sID), s.tunnels.SudoPasswordFor(sID))
		if err == nil {
			err = s.waitForProviderRemoval(id, name, 20*time.Second)
		}
		if err == nil {
			err = s.removeCachedProvider(sID, name)
		}
		s.deployStatus.finish(sID, err)
		if err == nil {
			if err := s.SyncServer(id); err != nil {
				log.Warn().Err(err).Str("server_id", id).Str("provider", name).Msg("failed to sync server after provider removal")
			}
		}
	}()
	return nil
}
