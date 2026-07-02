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

package agent

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/ks-tool/awg-admin/agent/internal/service"
	"github.com/ks-tool/awg-admin/agent/models"
)

// gatherHostInfo discovers, once at startup, what the host the agent runs on
// supports: the active backend's creatable interface kinds and kernel-module
// availability (service.ActiveBackendInfo — so this is backend-agnostic here),
// whether Docker is usable, and whether the agent itself runs inside a
// container. The result is served unchanged over GET /info for the process's
// lifetime, since none of it changes under a running agent. version is the
// build-stamped version string. SetBackend must have been called first.
func gatherHostInfo(version string) models.HostInfo {
	be := service.ActiveBackendInfo()
	return models.HostInfo{
		Backend:        be.Kind,
		Version:        version,
		Docker:         dockerAvailable(),
		InDocker:       inDocker(),
		KernelModule:   be.KernelModule,
		InterfaceKinds: be.InterfaceKinds,
	}
}

// inDocker reports whether the agent process runs inside a Docker container,
// detected by the /.dockerenv file Docker creates in every container's root
// filesystem — the check the userspace image relies on to know it's containerized.
func inDocker() bool {
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// dockerAvailable reports whether Docker is usable on the host: the docker CLI
// is on PATH and `docker info` succeeds (daemon installed and reachable),
// mirroring the docker deploy's pre-flight check. Bounded by a short timeout so
// a wedged daemon can't stall agent startup; the command's output is discarded.
func dockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}
