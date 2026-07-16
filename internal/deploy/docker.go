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
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/ks-tool/awg-admin/internal/sshclient"
	"github.com/ks-tool/awg-admin/models"

	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"
)

const (
	// The container name is stable so a redeploy replaces it; the named volume
	// holds the agent's data dir. The agent listens inside the container on the
	// port from the server's Agent.Address (published straight through to the
	// host at that same address); defaultAgentPort is only the fallback when the
	// address carries no port.
	dockerContainerName = "awg-agent"
	dockerDataVolume    = "awg-agent-data"
	dockerDataPath      = "/data"
	defaultAgentPort    = "8080"
)

// deployDocker runs the agent as a Docker container from src.Image (the
// userspace amneziawg-go build). It pre-checks Docker is up (`docker info` —
// output reusable later for server info), uploads mTLS material if configured,
// then (re)creates the container with a single `docker run -d` (no separate
// pull — run fetches the image if it's missing).
func deployDocker(client *ssh.Client, sudo sshclient.Sudo, srv models.Server, src models.AgentSource, udpPorts []uint16, step stepFunc, failed failFunc) error {
	step("check_docker").Msg("deploy step")
	if _, err := sshclient.RunAs(client, sudo, "docker info"); err != nil {
		return failed("check_docker", fmt.Errorf("the Docker not available on %s (docker info failed) — install/start Docker or use a binary agent source: %w", srv.SSH.Host, err))
	}

	if tls := srv.Agent.TLS; !tls.IsZero() {
		step("upload_tls").Msg("deploy step")
		if err := uploadAgentTLS(client, sudo, tls); err != nil {
			return failed("upload_tls", err)
		}
	}

	step("start_container").Msg("deploy step")
	if _, err := sshclient.RunAs(client, sudo, dockerRunCmd(srv, src, udpPorts)); err != nil {
		return failed("start_container", fmt.Errorf("start agent container: %w", err))
	}

	log.Info().Str("server_id", srv.ID.String()).Msg("agent deployed (docker)")
	return nil
}

// dockerRunCmd builds the shell command that (re)creates the agent container
// from src.Image. Any previous container is removed first so a redeploy picks
// up the new image/config. The agent listens inside the container on the port
// from the server's Agent.Address and is published on the host at that same
// address (the loopback the SSH tunnel forwards to, or a public IP for direct
// mTLS); each interface's WireGuard ListenPort is published as UDP on all host
// interfaces so remote clients can reach it.
func dockerRunCmd(srv models.Server, src models.AgentSource, udpPorts []uint16) string {
	addr := srv.Agent.Address
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		port = defaultAgentPort
	}

	var b strings.Builder
	b.WriteString("docker rm -f " + dockerContainerName + " >/dev/null 2>&1; ")
	b.WriteString("docker run -d --name " + dockerContainerName + " --restart unless-stopped")
	// Privileges the userspace agent needs, kept minimal (no --privileged):
	//   - /dev/net/tun: create the userspace WireGuard TUN device.
	//   - CAP_NET_ADMIN: configure the interface (addr/link/routes) and run the
	//     policy-routing / iptables lifecycle hooks.
	//   - CAP_NET_RAW: the iptables hooks (NAT/mangle) need it.
	// It forwards client traffic like any VPN gateway, but a container netns
	// defaults net.ipv4.ip_forward to 0 — enable it (namespaced, per-container).
	b.WriteString(" --cap-add NET_ADMIN --cap-add NET_RAW --device /dev/net/tun")
	b.WriteString(" --sysctl net.ipv4.ip_forward=1")

	// HTTP API (TCP): published on the host at the address the admin dials.
	b.WriteString(" -p " + addr + ":" + port)

	// WireGuard ListenPort(s) (UDP): published on all host interfaces so remote
	// clients can reach them. New interfaces added after this deploy need a
	// redeploy to publish their port (a container's port set is fixed at run).
	for _, p := range udpPorts {
		if p == 0 {
			continue
		}
		ps := strconv.Itoa(int(p))
		b.WriteString(" -p " + ps + ":" + ps + "/udp")
	}

	b.WriteString(" -v " + dockerDataVolume + ":" + dockerDataPath)
	b.WriteString(" -e AWG_AGENT_ADDR=0.0.0.0:" + port)
	b.WriteString(" -e AWG_AGENT_DB=" + dockerDataPath)
	if tls := srv.Agent.TLS; !tls.IsZero() {
		b.WriteString(" -v " + remoteConfigDir + ":" + remoteConfigDir + ":ro")
		b.WriteString(" -e AWG_AGENT_TLS_CERT=" + remoteServerCertPath)
		b.WriteString(" -e AWG_AGENT_TLS_KEY=" + remoteServerKeyPath)
		b.WriteString(" -e AWG_AGENT_TLS_CLIENT_CA=" + remoteCACertPath)
	}
	b.WriteString(" " + shellQuote(src.Image))
	return b.String()
}

// shellQuote wraps s in single quotes for safe interpolation into a remote
// `sh -c` command (src.Image is user-provided).
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
