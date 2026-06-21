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
	"github.com/ks-tool/awg-admin/internal/sshclient"
	"github.com/ks-tool/awg-admin/storage"

	"github.com/rs/zerolog/log"
)

type Option func(*Service)

func GeneratePresharedKey() Option {
	return func(h *Service) {
		h.generatePresharedKey = true
	}
}

type Service struct {
	store   storage.Storage
	tunnels *sshclient.Manager

	deployStatus *deployStatusStore

	generatePresharedKey bool
}

func New(s storage.Storage, opts ...Option) *Service {
	svc := &Service{store: s, tunnels: sshclient.NewManager(), deployStatus: newDeployStatusStore()}
	for _, opt := range opts {
		opt(svc)
	}
	return svc
}

// StartTunnels opens an SSH tunnel to the agent of every stored server that
// doesn't have mTLS configured, and keeps each connection open for the
// lifetime of the process (see sshclient.Manager). Servers with mTLS are
// reached directly instead. Per-server dial failures are logged but don't
// prevent the rest of the servers from getting a tunnel, since a server
// being temporarily unreachable at startup shouldn't block the whole app.
func (s *Service) StartTunnels() {
	servers, err := s.store.Servers().List()
	if err != nil {
		log.Error().Err(err).Msg("failed to list servers for tunnel setup")
		return
	}
	errs := s.tunnels.OpenAll(servers)
	for id, err := range errs {
		log.Warn().Err(err).Str("server_id", id.String()).Msg("failed to open agent tunnel")
	}
	s.syncTunnelledServers(servers, errs)
}

// StopTunnels closes every tunnel opened by StartTunnels.
func (s *Service) StopTunnels() {
	if err := s.tunnels.Close(); err != nil {
		log.Error().Err(err).Msg("failed to close agent tunnels")
	}
}
