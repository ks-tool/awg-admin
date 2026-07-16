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
	"testing"

	"github.com/ks-tool/awg-admin/models"
)

// TestUpdateServerSwitchesSSHAuthMethod locks the credential-merge rule in
// UpdateServer: the one credential the user actually entered decides the auth
// method and clears the others, while an edit that carries no credential
// preserves the stored ones. The bug this guards against: switching key→password
// left the old key set, and Dial prefers a key (sshclient.authMethod), so the
// tunnel kept failing with "attempted methods [publickey]".
func TestUpdateServerSwitchesSSHAuthMethod(t *testing.T) {
	base := func() models.SSHConfig {
		return models.SSHConfig{Host: "example.invalid", Port: 22, User: "deploy"}
	}

	t.Run("key to password clears the key", func(t *testing.T) {
		svc := newTestService(t)
		srv, err := svc.CreateServer(ServerInput{
			Name: "s", SSH: withKeyData(base(), "KEYBODY"),
		})
		if err != nil {
			t.Fatalf("CreateServer: %v", err)
		}

		updated, err := svc.UpdateServer(srv.ID.String(), ServerInput{
			Name: "s", SSH: withPassword(base(), "secret"),
		})
		if err != nil {
			t.Fatalf("UpdateServer: %v", err)
		}
		if updated.SSH.KeyData != "" || updated.SSH.Key != "" {
			t.Errorf("key not cleared on switch to password: key=%q keyData=%q", updated.SSH.Key, updated.SSH.KeyData)
		}
		if updated.SSH.Password != "secret" {
			t.Errorf("password = %q, want %q", updated.SSH.Password, "secret")
		}
	})

	t.Run("password to key clears the password", func(t *testing.T) {
		svc := newTestService(t)
		srv, err := svc.CreateServer(ServerInput{
			Name: "s", SSH: withPassword(base(), "secret"),
		})
		if err != nil {
			t.Fatalf("CreateServer: %v", err)
		}

		updated, err := svc.UpdateServer(srv.ID.String(), ServerInput{
			Name: "s", SSH: withKeyData(base(), "KEYBODY"),
		})
		if err != nil {
			t.Fatalf("UpdateServer: %v", err)
		}
		if updated.SSH.Password != "" {
			t.Errorf("password not cleared on switch to key: %q", updated.SSH.Password)
		}
		if updated.SSH.KeyData != "KEYBODY" {
			t.Errorf("keyData = %q, want %q", updated.SSH.KeyData, "KEYBODY")
		}
	})

	t.Run("unrelated edit preserves stored credentials", func(t *testing.T) {
		svc := newTestService(t)
		srv, err := svc.CreateServer(ServerInput{
			Name: "s", SSH: withPassword(base(), "secret"),
		})
		if err != nil {
			t.Fatalf("CreateServer: %v", err)
		}

		// The form never re-sends the password on an unrelated change (here a
		// port bump), so it arrives empty — must not wipe the stored one.
		noCreds := base()
		noCreds.Port = 2222
		updated, err := svc.UpdateServer(srv.ID.String(), ServerInput{Name: "s", SSH: noCreds})
		if err != nil {
			t.Fatalf("UpdateServer: %v", err)
		}
		if updated.SSH.Password != "secret" {
			t.Errorf("password wiped by unrelated edit: %q", updated.SSH.Password)
		}
		if updated.SSH.Port != 2222 {
			t.Errorf("port = %d, want 2222", updated.SSH.Port)
		}
	})
}

func withKeyData(c models.SSHConfig, data string) models.SSHConfig {
	c.KeyData = data
	return c
}

func withPassword(c models.SSHConfig, pw string) models.SSHConfig {
	c.Password = pw
	return c
}
