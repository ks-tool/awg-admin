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

package models

// AuthCredentials is the single admin account used to log in to the
// standalone web-server mode (cmd/awg-admin.go). The Wails desktop app
// never reads this — it already runs as a local single-user process with
// no network exposure, so it has nothing to authenticate against.
type AuthCredentials struct {
	Username string `json:"username"`
	// PasswordHash is a bcrypt hash, never the plaintext password.
	PasswordHash string `json:"passwordHash"`
	// BasicAuthEnabled gates the standalone web server behind HTTP Basic
	// Auth (checked against this same Username/PasswordHash) in addition
	// to the existing session-cookie login — off by default. Useful as an
	// extra layer in front of the whole app (including the login page and
	// static assets) when exposing it directly without a reverse proxy.
	BasicAuthEnabled bool `json:"basicAuthEnabled,omitempty"`
}
