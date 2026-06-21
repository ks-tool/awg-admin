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
	"sync"

	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
)

// deployStatusStore is an in-memory map of the most recent
// Service.DeployAgent run per server — like sessionStore
// (internal/api/session.go) and the SSH passphrase cache
// (internal/sshclient.Manager), this is process-lifetime state, never
// persisted: a deploy in flight when awg-admin restarts simply loses its
// progress, which is fine since the underlying SSH/systemd operations
// aren't resumable anyway.
type deployStatusStore struct {
	mu    sync.Mutex
	state map[uuid.UUID]models.DeployStatus
}

func newDeployStatusStore() *deployStatusStore {
	return &deployStatusStore{state: make(map[uuid.UUID]models.DeployStatus)}
}

func (s *deployStatusStore) start(id uuid.UUID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state[id] = models.DeployStatus{Step: "connect"}
}

func (s *deployStatusStore) setStep(id uuid.UUID, step string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state[id]
	st.Step = step
	s.state[id] = st
}

func (s *deployStatusStore) finish(id uuid.UUID, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.state[id]
	st.Done = true
	if err != nil {
		st.Error = err.Error()
	}
	s.state[id] = st
}

func (s *deployStatusStore) get(id uuid.UUID) (models.DeployStatus, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.state[id]
	return st, ok
}
