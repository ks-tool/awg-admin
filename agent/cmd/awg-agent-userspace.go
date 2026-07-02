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

// Command awg-agent-userspace is the agent built to drive interfaces through an
// in-process userspace WireGuard device (the amneziawg-go library) instead of
// the kernel module — for hosts without a (matching) AmneziaWG kernel module.
// It's a self-contained binary (amneziawg-go is compiled in, no external
// process) and is otherwise identical to awg-agent; it just wires in the
// userspace backend before starting. It needs /dev/net/tun and CAP_NET_ADMIN
// at runtime to create and manage the TUN interfaces.
package main

import (
	"github.com/ks-tool/awg-admin/agent/internal/agent"
	"github.com/ks-tool/awg-admin/agent/internal/service"
	"github.com/ks-tool/awg-admin/agent/internal/userspace"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	service.SetBackend(userspace.New())
	agent.Run(version)
}
