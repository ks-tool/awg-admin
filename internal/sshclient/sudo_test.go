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

package sshclient

import "testing"

func TestSudoShell(t *testing.T) {
	cmd := "systemctl daemon-reload && systemctl restart awg-agent"
	tests := []struct {
		name string
		sudo Sudo
		want string
	}{
		{"disabled leaves cmd untouched", Sudo{}, cmd},
		{"passwordless wraps in sudo -n sh -c", Sudo{Enabled: true}, "sudo -n sh -c '" + cmd + "'"},
		{"password uses -k -S with empty prompt", Sudo{Enabled: true, Password: "pw"}, "sudo -k -S -p '' sh -c '" + cmd + "'"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sudo.shell(cmd); got != tt.want {
				t.Errorf("shell() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSudoSimple(t *testing.T) {
	cmd := "install -D -m 755 /dev/stdin '/usr/local/bin/awg-agent'"
	tests := []struct {
		name string
		sudo Sudo
		want string
	}{
		{"disabled", Sudo{}, cmd},
		{"passwordless", Sudo{Enabled: true}, "sudo -n " + cmd},
		{"password", Sudo{Enabled: true, Password: "pw"}, "sudo -k -S -p '' " + cmd},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.sudo.simple(cmd); got != tt.want {
				t.Errorf("simple() = %q, want %q", got, tt.want)
			}
		})
	}
}

// A command that itself contains single-quoted args (the download pipeline,
// docker run) must survive the outer sudo `sh -c` wrapping: shellQuote escapes
// the inner quotes as '\” so the remote shell reconstructs the original cmd.
func TestSudoShellNestedQuotes(t *testing.T) {
	cmd := "docker run 'my-image:latest'"
	want := `sudo -n sh -c 'docker run '\''my-image:latest'\'''`
	if got := (Sudo{Enabled: true}).shell(cmd); got != want {
		t.Errorf("shell() = %q, want %q", got, want)
	}
}

func TestSudoStdinPrefix(t *testing.T) {
	if p := (Sudo{}).stdinPrefix(); p != nil {
		t.Errorf("disabled stdinPrefix = %q, want nil", p)
	}
	if p := (Sudo{Enabled: true}).stdinPrefix(); p != nil {
		t.Errorf("passwordless stdinPrefix = %q, want nil", p)
	}
	if p := string((Sudo{Enabled: true, Password: "pw"}).stdinPrefix()); p != "pw\n" {
		t.Errorf("password stdinPrefix = %q, want %q", p, "pw\n")
	}
}
