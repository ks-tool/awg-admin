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

// Package deploy installs the awg-agent onto a managed server over SSH. It
// supports two methods, chosen by the models.AgentSource being deployed:
//
//   - systemd (binary): the agent runs as a systemd unit on the host, driving
//     the AmneziaWG kernel module — see systemd.go. Used for a URL/Path source.
//   - docker (image): the agent runs as a Docker container from the source's
//     image (the userspace amneziawg-go build) — see docker.go. Used when the
//     source is an Image.
//
// ToAgent connects and dispatches to the right method; each runs its own
// pre-deploy check (kernel module for systemd, `docker info` for docker) so a
// missing prerequisite fails fast with an actionable message.
package deploy

import (
	"context"
	"fmt"

	"github.com/ks-tool/awg-admin/internal/sshclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

// Paths shared by both deploy methods: the agent's config dir on the server and
// the mTLS material written into it (method-specific paths live in systemd.go /
// docker.go).
const (
	remoteConfigDir      = "/etc/awg-agent"
	remoteCACertPath     = "/etc/awg-agent/ca.pem"
	remoteServerCertPath = "/etc/awg-agent/server.pem"
	remoteServerKeyPath  = "/etc/awg-agent/server-key.pem"
)

// stepFunc reports (and logs) the current deploy step; failFunc logs and
// returns a failed step's error. Both are closures created in ToAgent and
// handed to the per-method deploy functions so progress/errors flow back
// through the same onStep callback and logger.
type (
	stepFunc func(name string) *zerolog.Event
	failFunc func(name string, err error) error
)

// ToAgent deploys the agent to srv over SSH, dispatching to the systemd or
// docker method based on src (an Image source → docker, otherwise systemd).
// udpPorts are the WireGuard ListenPorts of the server's interfaces, published
// by the docker method so remote clients can reach them (ignored by systemd,
// which runs on the host network directly). passphrase decrypts srv.SSH's key
// when it's passphrase-protected (see sshclient.Dial); pass "" when none is
// cached. onStep, if non-nil, is called with a stable step-name key right
// before each step runs, so a caller can surface deploy progress (e.g.
// Service.DeployAgent's in-memory status, polled by the frontend) — the names
// are deliberately untranslated keys, not human-readable text, so the UI can
// localize them itself. Step keys: "connect"; systemd: "check_kernel_module",
// "upload_binary", "upload_unit", "upload_env", "upload_tls", "start_service";
// docker: "check_docker", "upload_tls", "start_container".
func ToAgent(ctx context.Context, srv models.Server, src models.AgentSource, udpPorts []uint16, passphrase string, onStep func(string)) error {
	l := log.With().Str("server_id", srv.ID.String()).Logger()
	l.Info().Str("agent_source", src.Name).Bool("cache_locally", src.CacheLocally).Bool("docker", len(src.Image) > 0).Msg("deploying agent")

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

	if len(src.Image) > 0 {
		return deployDocker(client, srv, src, udpPorts, step, failed)
	}
	return deploySystemd(ctx, client, srv, src, step, failed)
}

// uploadAgentTLS writes the CA + server cert/key into the agent's config dir on
// the server — shared by the systemd and docker deploy paths.
func uploadAgentTLS(client *ssh.Client, tls *models.AgentTLS) error {
	if err := sshclient.UploadFile(client, remoteCACertPath, 0o644, []byte(tls.CA.Certificate)); err != nil {
		return fmt.Errorf("upload CA certificate: %w", err)
	}
	if err := sshclient.UploadFile(client, remoteServerCertPath, 0o644, []byte(tls.Server.Certificate)); err != nil {
		return fmt.Errorf("upload server certificate: %w", err)
	}
	if err := sshclient.UploadFile(client, remoteServerKeyPath, 0o600, []byte(tls.Server.PrivateKey)); err != nil {
		return fmt.Errorf("upload server key: %w", err)
	}
	return nil
}
