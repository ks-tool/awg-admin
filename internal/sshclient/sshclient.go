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

// Package sshclient connects to a managed server over SSH, the transport
// used both to deploy the agent and, by default, to reach its HTTP API
// without exposing it on a public interface (see Tunnel in tunnel.go).
package sshclient

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"github.com/ks-tool/awg-admin/models"

	"golang.org/x/crypto/ssh"
)

const dialTimeout = 15 * time.Second

// PassphraseRequiredError indicates the SSH private key (uploaded KeyData or
// a Key file) is passphrase-protected and either no passphrase was supplied
// or the one supplied was wrong. Its Error() message carries the
// sshPassphraseMarker sentinel so callers across process boundaries (HTTP
// JSON error bodies, Wails-bound method errors serialized to plain strings)
// can still detect it with a substring check and prompt the user instead of
// showing a generic connection-failure message.
type PassphraseRequiredError struct {
	Err error
}

// sshPassphraseMarker is matched verbatim by the frontend (both HTTP and
// Wails bindings transports collapse Go errors to plain strings, so a
// stable substring is the only reliable way to recognize this case).
const sshPassphraseMarker = "SSH_PASSPHRASE_REQUIRED"

func (e *PassphraseRequiredError) Error() string {
	return fmt.Sprintf("%s: ssh key requires a passphrase: %v", sshPassphraseMarker, e.Err)
}

func (e *PassphraseRequiredError) Unwrap() error { return e.Err }

// Dial opens an SSH connection to the server using its stored SSH config.
// cfg.Key is a path to a private key file on the machine running awg-admin
// (see models.SSHConfig), not the key body itself; cfg.KeyData, if set, is
// the key's raw PEM content uploaded directly into storage and takes
// precedence over Key. passphrase decrypts the key when it's
// passphrase-protected — pass "" when none is cached yet; a
// *PassphraseRequiredError is returned if one turns out to be needed.
//
// Host key verification is intentionally not performed (InsecureIgnoreHostKey):
// these are servers the operator just typed an address for, with no
// pre-shared known_hosts entry, mirroring how most lightweight server
// management tools behave on first connect.
func Dial(cfg models.SSHConfig, passphrase string) (*ssh.Client, error) {
	auth, err := authMethod(cfg, passphrase)
	if err != nil {
		return nil, err
	}

	port := cfg.Port
	if port == 0 {
		port = 22
	}
	user := cfg.User
	if len(user) == 0 {
		user = "root"
	}

	clientCfg := &ssh.ClientConfig{
		User:            user,
		Auth:            []ssh.AuthMethod{auth},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec // see doc comment
		Timeout:         dialTimeout,
	}

	addr := net.JoinHostPort(cfg.Host, fmt.Sprintf("%d", port))
	client, err := ssh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", addr, err)
	}
	return client, nil
}

func authMethod(cfg models.SSHConfig, passphrase string) (ssh.AuthMethod, error) {
	keyData, keySource, err := loadKey(cfg)
	if err != nil {
		return nil, err
	}
	if keyData != nil {
		signer, err := parseSigner(keyData, passphrase)
		if err != nil {
			var passErr *PassphraseRequiredError
			if errors.As(err, &passErr) {
				return nil, passErr
			}
			return nil, fmt.Errorf("parse private key (%s): %w", keySource, err)
		}
		return ssh.PublicKeys(signer), nil
	}
	if len(cfg.Password) > 0 {
		return ssh.Password(cfg.Password), nil
	}
	return nil, fmt.Errorf("no SSH credentials configured (key or password required)")
}

// loadKey returns the raw private key bytes for cfg, preferring an uploaded
// KeyData over a Key file path, and keySource (for error messages). Returns
// a nil slice (no error) if cfg has no key configured at all, signaling the
// caller to fall back to password auth.
func loadKey(cfg models.SSHConfig) (keyData []byte, keySource string, err error) {
	if len(cfg.KeyData) > 0 {
		return []byte(cfg.KeyData), "uploaded key", nil
	}
	if len(cfg.Key) > 0 {
		data, err := os.ReadFile(cfg.Key)
		if err != nil {
			return nil, cfg.Key, fmt.Errorf("read private key %q: %w", cfg.Key, err)
		}
		return data, cfg.Key, nil
	}
	return nil, "", nil
}

// parseSigner parses keyData, decrypting it with passphrase when non-empty.
// Any failure once a passphrase was supplied (wrong passphrase, or any other
// parse error) is reported as *PassphraseRequiredError too, so the caller
// re-prompts instead of surfacing a raw parse error — from the user's
// perspective "wrong passphrase" and "passphrase needed" call for the same
// retry UI.
func parseSigner(keyData []byte, passphrase string) (ssh.Signer, error) {
	if len(passphrase) == 0 {
		signer, err := ssh.ParsePrivateKey(keyData)
		if err != nil {
			var missing *ssh.PassphraseMissingError
			if errors.As(err, &missing) {
				return nil, &PassphraseRequiredError{Err: err}
			}
			return nil, err
		}
		return signer, nil
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(keyData, []byte(passphrase))
	if err != nil {
		return nil, &PassphraseRequiredError{Err: err}
	}
	return signer, nil
}

// Run executes a single command on the remote host and returns its
// combined stdout+stderr output.
func Run(client *ssh.Client, cmd string) (string, error) {
	session, err := client.NewSession()
	if err != nil {
		return "", fmt.Errorf("open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	out, err := session.CombinedOutput(cmd)
	if err != nil {
		return string(out), fmt.Errorf("run %q: %w (output: %s)", cmd, err, out)
	}
	return string(out), nil
}

// UploadFile writes data to remotePath on the remote host with the given
// permissions, creating parent directories as needed. It streams the file
// over the session's stdin rather than relying on SFTP/SCP being installed,
// since `install` ships with coreutils on essentially every Linux distro.
func UploadFile(client *ssh.Client, remotePath string, mode os.FileMode, data []byte) error {
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("open session: %w", err)
	}
	defer func() { _ = session.Close() }()

	stdin, err := session.StdinPipe()
	if err != nil {
		return fmt.Errorf("open stdin pipe: %w", err)
	}

	cmd := fmt.Sprintf("install -D -m %o /dev/stdin %s", mode.Perm(), shellQuote(remotePath))
	if err := session.Start(cmd); err != nil {
		return fmt.Errorf("start %q: %w", cmd, err)
	}

	if _, err := stdin.Write(data); err != nil {
		return fmt.Errorf("write file data: %w", err)
	}
	if err := stdin.Close(); err != nil {
		return fmt.Errorf("close stdin: %w", err)
	}

	if err := session.Wait(); err != nil {
		return fmt.Errorf("upload %s: %w", remotePath, err)
	}
	return nil
}

// DownloadFile runs a remote command that fetches url directly into
// remotePath on the host client is connected to, with the given
// permissions — used to deploy an AgentSource without local caching
// (models.AgentSource.CacheLocally == false), so the binary's bytes never
// pass through the machine running awg-admin. Tries curl first, falls
// back to wget if curl isn't on the remote PATH; downloads to a temporary
// path first and installs it atomically via `install -D -m` (mirroring
// UploadFile), so a failed/partial download never leaves a broken file at
// remotePath.
func DownloadFile(client *ssh.Client, url, remotePath string, mode os.FileMode) error {
	tmp := remotePath + ".download"
	cmd := fmt.Sprintf(
		"if command -v curl >/dev/null 2>&1; then curl -fsSL -o %[1]s %[3]s; "+
			"elif command -v wget >/dev/null 2>&1; then wget -qO %[1]s %[3]s; "+
			"else echo 'neither curl nor wget found on PATH' >&2; exit 127; fi && "+
			"install -D -m %[2]o %[1]s %[4]s && rm -f %[1]s",
		shellQuote(tmp), mode.Perm(), shellQuote(url), shellQuote(remotePath),
	)
	if _, err := Run(client, cmd); err != nil {
		return fmt.Errorf("download %s on remote host: %w", url, err)
	}
	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
