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

package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const (
	sessionCookieName = "awg_admin_session"
	sessionTTL        = 7 * 24 * time.Hour
)

// sessionStore is an in-memory set of valid session tokens for the
// standalone web server's single admin account. Tokens are opaque random
// values rather than signed/stateless JWTs — there's exactly one account
// and one process holding the store, so there's nothing distributed to
// support (see project conventions: single-user app). Sessions don't
// survive a server restart, matching how the SSH passphrase cache
// (internal/sshclient.Manager) already treats in-memory state as scoped to
// the process's lifetime ("for the duration of the session").
type sessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time
}

func newSessionStore() *sessionStore {
	return &sessionStore{sessions: make(map[string]time.Time)}
}

func (s *sessionStore) create() string {
	token := randomToken()
	s.mu.Lock()
	s.sessions[token] = time.Now().Add(sessionTTL)
	s.mu.Unlock()
	return token
}

func (s *sessionStore) valid(token string) bool {
	if len(token) == 0 {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(exp) {
		delete(s.sessions, token)
		return false
	}
	return true
}

// validRequest reports whether r already carries a valid session cookie —
// used by BasicAuthMiddleware so an admin who's already logged in via the
// normal session flow is never *additionally* challenged for HTTP Basic
// Auth credentials (see that middleware's doc comment).
func (s *sessionStore) validRequest(r *http.Request) bool {
	c, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}
	return s.valid(c.Value)
}

func (s *sessionStore) revoke(token string) {
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failing means the system RNG is broken — nothing
		// sensible to do but fail loudly rather than issue a weak token.
		panic(err)
	}
	return hex.EncodeToString(b)
}
