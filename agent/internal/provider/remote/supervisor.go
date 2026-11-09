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

package remote

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/ks-tool/vpn-agent/models"
	"github.com/rs/zerolog/log"
)

type Supervisor struct {
	registrations []models.ProviderRegistration
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

func NewSupervisor(registrations []models.ProviderRegistration) *Supervisor {
	return &Supervisor{registrations: registrations}
}

func (s *Supervisor) Start(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	s.cancel = cancel
	for _, registration := range s.registrations {
		if registration.Command == "" {
			continue
		}
		if !filepath.IsAbs(registration.Command) {
			s.Close()
			return fmt.Errorf("provider %s command must be an absolute path", registration.Provider.Name)
		}
		s.wg.Add(1)
		go s.supervise(ctx, registration)
	}
	return nil
}

func (s *Supervisor) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

func (s *Supervisor) supervise(ctx context.Context, registration models.ProviderRegistration) {
	defer s.wg.Done()
	name := registration.Provider.Name
	for ctx.Err() == nil {
		cmd := exec.Command(registration.Command, registration.Args...)
		cmd.Env = os.Environ()
		for key, value := range registration.Env {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		done := make(chan error, 1)
		if err := cmd.Start(); err != nil {
			log.Error().Err(err).Str("provider", name).Str("command", registration.Command).Msg("failed to start provider")
		} else {
			log.Info().Str("provider", name).Int("pid", cmd.Process.Pid).Msg("provider started")
			go func() { done <- cmd.Wait() }()
			select {
			case <-ctx.Done():
				stopProcess(cmd, done)
				return
			case err := <-done:
				if err != nil && !errors.Is(err, context.Canceled) {
					log.Error().Err(err).Str("provider", name).Msg("provider exited; restarting")
				} else {
					log.Warn().Str("provider", name).Msg("provider exited; restarting")
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Second):
		}
	}
}

func stopProcess(cmd *exec.Cmd, done <-chan error) {
	_ = cmd.Process.Signal(syscall.SIGTERM)
	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		<-done
	}
}
