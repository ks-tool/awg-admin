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

// Package deploy installs the awg-agent binary on a managed server over
// SSH: it gets the binary onto the server (either having the server
// download it itself, or uploading a locally-cached copy — see
// models.AgentSource), uploads the systemd unit, an env file with the
// agent's configuration, and — when mTLS is enabled — the CA/server
// certificates, then enables and starts the service.
package deploy

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/ks-tool/awg-admin/internal/sshclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

//go:embed assets/awg-agent.service
var systemdUnit []byte

const (
	remoteBinPath        = "/usr/local/bin/awg-agent"
	remoteUnitPath       = "/etc/systemd/system/awg-agent.service"
	remoteEnvPath        = "/etc/awg-agent/awg-agent.env"
	remoteDBPath         = "/var/lib/awg-agent"
	remoteCACertPath     = "/etc/awg-agent/ca.pem"
	remoteServerCertPath = "/etc/awg-agent/server.pem"
	remoteServerKeyPath  = "/etc/awg-agent/server-key.pem"
)

// ToAgent gets the agent binary onto srv's host (per src.CacheLocally —
// see models.AgentSource's doc comment), uploads the systemd unit,
// configuration and (if configured) mTLS certificates over SSH, then
// enables and starts the awg-agent service. passphrase decrypts srv.SSH's
// key when it's passphrase-protected (see sshclient.Dial); pass "" when
// none is cached. onStep, if non-nil, is called with a stable step-name
// key ("connect", "upload_binary", "upload_unit", "upload_env",
// "upload_tls", "start_service") right before each step runs, so a caller
// can surface deploy progress (e.g. Service.DeployAgent's in-memory
// status, polled by the frontend) — the names are deliberately untranslated
// keys, not human-readable text, so the UI can localize them itself.
func ToAgent(ctx context.Context, srv models.Server, src models.AgentSource, passphrase string, onStep func(string)) error {
	l := log.With().Str("server_id", srv.ID.String()).Logger()
	l.Info().Str("agent_source", src.Name).Bool("cache_locally", src.CacheLocally).Msg("deploying agent")

	step := func(name string) *zerolog.Event {
		if onStep != nil {
			onStep(name)
		}
		return l.Debug().Str("step", name)
	}
	failed := func(name string, err error) error {
		l.Warn().Err(err).Str("step", name).Msg("deploy step failed")
		return err
	}

	step("connect").Msg("deploy step")
	client, err := sshclient.Dial(srv.SSH, passphrase)
	if err != nil {
		return failed("connect", fmt.Errorf("connect to %s: %w", srv.SSH.Host, err))
	}
	defer func() { _ = client.Close() }()

	step("upload_binary").Msg("deploy step")
	switch {
	case len(src.Path) > 0:
		// The binary is already on awg-admin's own filesystem — read it
		// directly and upload the bytes, no download/caching involved.
		binary, err := os.ReadFile(src.Path)
		if err != nil {
			return failed("upload_binary", fmt.Errorf("read agent binary: %w", err))
		}
		if len(binary) == 0 {
			return failed("upload_binary", fmt.Errorf("agent binary source is empty"))
		}
		if err := sshclient.UploadFile(client, remoteBinPath, 0o755, binary); err != nil {
			return failed("upload_binary", fmt.Errorf("upload agent binary: %w", err))
		}
	case src.CacheLocally:
		// awg-admin fetches (or reuses a previously cached copy of) the
		// binary itself and uploads the bytes over the same SSH
		// connection — for servers without outbound internet access to
		// src.URL.
		binary, err := fetchCached(ctx, src)
		if err != nil {
			return failed("upload_binary", fmt.Errorf("fetch agent binary: %w", err))
		}
		if len(binary) == 0 {
			return failed("upload_binary", fmt.Errorf("agent binary source is empty"))
		}
		if err := sshclient.UploadFile(client, remoteBinPath, 0o755, binary); err != nil {
			return failed("upload_binary", fmt.Errorf("upload agent binary: %w", err))
		}
	default:
		// The server downloads src.URL itself (curl/wget) — awg-admin's
		// own machine never sees the bytes.
		if err := sshclient.DownloadFile(client, src.URL, remoteBinPath, 0o755); err != nil {
			return failed("upload_binary", fmt.Errorf("download agent binary on server: %w", err))
		}
	}

	step("upload_unit").Msg("deploy step")
	if err := sshclient.UploadFile(client, remoteUnitPath, 0o644, systemdUnit); err != nil {
		return failed("upload_unit", fmt.Errorf("upload systemd unit: %w", err))
	}

	step("upload_env").Msg("deploy step")
	if err := sshclient.UploadFile(client, remoteEnvPath, 0o600, []byte(buildEnvFile(srv))); err != nil {
		return failed("upload_env", fmt.Errorf("upload env file: %w", err))
	}

	if tls := srv.Agent.TLS; !tls.IsZero() {
		step("upload_tls").Msg("deploy step")
		if err := sshclient.UploadFile(client, remoteCACertPath, 0o644, []byte(tls.CA.Certificate)); err != nil {
			return failed("upload_tls", fmt.Errorf("upload CA certificate: %w", err))
		}
		if err := sshclient.UploadFile(client, remoteServerCertPath, 0o644, []byte(tls.Server.Certificate)); err != nil {
			return failed("upload_tls", fmt.Errorf("upload server certificate: %w", err))
		}
		if err := sshclient.UploadFile(client, remoteServerKeyPath, 0o600, []byte(tls.Server.PrivateKey)); err != nil {
			return failed("upload_tls", fmt.Errorf("upload server key: %w", err))
		}
	}

	step("start_service").Msg("deploy step")
	// "restart" (not "enable --now") so a redeploy over an already-running
	// agent actually picks up the freshly uploaded binary/config instead of
	// leaving the old process running untouched; restarting a unit that
	// isn't currently active is equivalent to starting it, so this also
	// covers the "agent was stopped" case in the same command.
	if _, err := sshclient.Run(client, "systemctl daemon-reload && systemctl enable awg-agent && systemctl restart awg-agent"); err != nil {
		return failed("start_service", fmt.Errorf("start agent service: %w", err))
	}

	l.Info().Msg("agent deployed")
	return nil
}

func buildEnvFile(srv models.Server) string {
	addr := srv.Agent.Address
	if len(addr) == 0 {
		addr = "127.0.0.1:8080"
	}

	var b strings.Builder
	b.WriteString("AWG_AGENT_ADDR=" + addr + "\n")
	b.WriteString("AWG_AGENT_DB=" + remoteDBPath + "\n")

	if tls := srv.Agent.TLS; !tls.IsZero() {
		b.WriteString("AWG_AGENT_TLS_CERT=" + remoteServerCertPath + "\n")
		b.WriteString("AWG_AGENT_TLS_KEY=" + remoteServerKeyPath + "\n")
		b.WriteString("AWG_AGENT_TLS_CLIENT_CA=" + remoteCACertPath + "\n")
	}
	return b.String()
}
