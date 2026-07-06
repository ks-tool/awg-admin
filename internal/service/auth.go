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
	"errors"

	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidCredentials is returned by Login and ChangeCredentials when the
// supplied username/password doesn't match the stored admin account.
var ErrInvalidCredentials = errors.New("invalid username or password")

// Login validates username/password against the single stored admin
// account. Only used by the standalone web-server's session-cookie login
// flow (internal/api) — the Wails desktop app never calls this, since it
// already runs as a local single-user process with no network exposure.
func (s *Service) Login(username, password string) error {
	debugOp("Login").Str("username", username).Msg("login attempt")
	creds, err := s.store.Auth().Get()
	if err != nil {
		return err
	}
	if creds.Username != username {
		return ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

// CurrentUsername returns the stored admin account's username, so the
// frontend can confirm a session is still valid and show who's logged in.
func (s *Service) CurrentUsername() (string, error) {
	debugOp("CurrentUsername").Msg("getting current username")
	creds, err := s.store.Auth().Get()
	if err != nil {
		return "", err
	}
	return creds.Username, nil
}

// BasicAuthEnabled reports whether the standalone server's HTTP Basic Auth
// gate (internal/api.BasicAuthMiddleware) is currently turned on. Off by
// default; toggled via SetBasicAuthEnabled from the Settings page.
func (s *Service) BasicAuthEnabled() (bool, error) {
	debugOp("BasicAuthEnabled").Msg("checking basic auth status")
	creds, err := s.store.Auth().Get()
	if err != nil {
		return false, err
	}
	return creds.BasicAuthEnabled, nil
}

// SetBasicAuthEnabled persists whether the standalone server requires HTTP
// Basic Auth (checked against the same admin account as session login) in
// front of every request, including the login page and static assets.
func (s *Service) SetBasicAuthEnabled(enabled bool) error {
	debugOp("SetBasicAuthEnabled").Bool("enabled", enabled).Msg("setting basic auth enabled")
	creds, err := s.store.Auth().Get()
	if err != nil {
		return err
	}
	creds.BasicAuthEnabled = enabled
	return s.store.Auth().Set(creds)
}

// VerifyBasicAuth reports whether a request carrying username/password (as
// decoded from an HTTP Basic Auth header by http.Request.BasicAuth) should
// be let through. When BasicAuthEnabled is off, every request passes
// (true) regardless of the supplied credentials — there's nothing to
// check. When on, it's a straight comparison against the same stored
// admin account session login uses.
func (s *Service) VerifyBasicAuth(username, password string) (bool, error) {
	debugOp("VerifyBasicAuth").Str("username", username).Msg("verifying basic auth")
	creds, err := s.store.Auth().Get()
	if err != nil {
		return false, err
	}
	if !creds.BasicAuthEnabled {
		return true, nil
	}
	if creds.Username != username {
		return false, nil
	}
	return bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(password)) == nil, nil
}

// ChangeCredentials updates the admin account's username and/or password,
// requiring the current password to confirm the change. Pass "" for
// newUsername/newPassword to leave that field as-is.
func (s *Service) ChangeCredentials(currentPassword, newUsername, newPassword string) error {
	debugOp("ChangeCredentials").Str("username", newUsername).Msg("changing credentials")
	creds, err := s.store.Auth().Get()
	if err != nil {
		return err
	}
	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(currentPassword)); err != nil {
		return ErrInvalidCredentials
	}

	if len(newUsername) > 0 {
		creds.Username = newUsername
	}
	if len(newPassword) > 0 {
		hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		creds.PasswordHash = string(hash)
	}
	return s.store.Auth().Set(creds)
}
