//go:build linux

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

// Command awg-agent-userspace is the agent built to drive interfaces through a
// userspace amneziawg-go process instead of the kernel module — for hosts
// without a (matching) AmneziaWG kernel module. It's identical to awg-agent
// except it wires in the userspace backend before starting. The amneziawg-go
// binary must be installed on the host; set AWG_AGENT_USERSPACE_BIN to point at
// it, otherwise "amneziawg-go" is resolved on $PATH.
package main

import (
	"os"

	"github.com/ks-tool/awg-admin/agent/internal/agent"
	"github.com/ks-tool/awg-admin/agent/internal/service"
	"github.com/ks-tool/awg-admin/agent/internal/userspace"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	service.SetBackend(userspace.New(os.Getenv("AWG_AGENT_USERSPACE_BIN")))
	agent.Run(version)
}
