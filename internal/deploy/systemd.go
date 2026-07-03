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

package deploy

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/ks-tool/awg-admin/internal/sshclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

//go:embed assets/awg-agent.service
var systemdUnit []byte

const (
	remoteBinPath  = "/usr/local/bin/awg-agent"
	remoteUnitPath = "/etc/systemd/system/awg-agent.service"
	remoteEnvPath  = "/etc/awg-agent/awg-agent.env"
	remoteDBPath   = "/var/lib/awg-agent"
)

// deploySystemd installs the kernel agent as a systemd unit: it gets the binary
// onto the host (per src.CacheLocally — see models.AgentSource), uploads the
// unit, the env file and (if configured) the mTLS certificates, then enables
// and restarts the service. The kernel agent drives the AmneziaWG kernel module
// (amneziawg-dkms), so a pre-check verifies the module is present first.
func deploySystemd(ctx context.Context, client *ssh.Client, srv models.Server, src models.AgentSource, step stepFunc, failed failFunc) error {
	// The kernel agent needs the AmneziaWG kernel module (amneziawg-dkms).
	// Pre-check it's present so a missing module fails fast with an actionable
	// message instead of the agent later crash-looping. Skipped for a userspace
	// source (src.Userspace): the userspace agent (awg-agent-userspace) runs over
	// systemd too and needs no kernel module — its whole point.
	if !src.Userspace {
		step("check_kernel_module").Msg("deploy step")
		if _, err := sshclient.Run(client, "modinfo amneziawg"); err != nil {
			return failed("check_kernel_module", fmt.Errorf("AmneziaWG kernel module (amneziawg-dkms) not found on %s — install it, mark the source as the userspace agent, or deploy the userspace agent via a Docker image source: %w", srv.SSH.Host, err))
		}
	}

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
		// binary itself and uploads the bytes over the same SSH connection —
		// for servers without outbound internet access to src.URL.
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
		// The server downloads src.URL itself (curl/wget) — awg-admin's own
		// machine never sees the bytes.
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
		if err := uploadAgentTLS(client, tls); err != nil {
			return failed("upload_tls", err)
		}
	}

	step("start_service").Msg("deploy step")
	// "restart" (not "enable --now") so a redeploy over an already-running
	// agent actually picks up the freshly uploaded binary/config instead of
	// leaving the old process running untouched; restarting a unit that isn't
	// currently active is equivalent to starting it, so this also covers the
	// "agent was stopped" case in the same command.
	if _, err := sshclient.Run(client, "systemctl daemon-reload && systemctl enable awg-agent && systemctl restart awg-agent"); err != nil {
		return failed("start_service", fmt.Errorf("start agent service: %w", err))
	}

	log.Info().Str("server_id", srv.ID.String()).Msg("agent deployed (systemd)")
	return nil
}

func buildEnvFile(srv models.Server) string {
	addr := srv.Agent.Address
	if len(addr) == 0 {
		addr = "127.0.0.1:" + defaultAgentPort
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
