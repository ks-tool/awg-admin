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
	"runtime"
	"time"

	"github.com/ks-tool/vpn-agent/models"
)

func GatherHostInfo(version string) models.HostInfo {
	return models.HostInfo{
		Backend:  "external",
		Version:  version,
		Docker:   dockerAvailable(),
		InDocker: inDocker(),
		Arch:     runtime.GOARCH,
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
