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

// Command awg-agent is the standard agent: it manages interfaces through the
// AmneziaWG/WireGuard kernel module (the kernel.Backend, netlink). For a
// userspace (amneziawg-go) build see cmd/awg-agent-userspace.
package main

import (
	"github.com/ks-tool/awg-admin/agent/internal/agent"
	"github.com/ks-tool/awg-admin/agent/internal/kernel"
	"github.com/ks-tool/awg-admin/agent/internal/service"
)

// version is set at build time via -ldflags "-X main.version=...". See the
// "agent" build in .goreleaser.yaml.
var version = "dev"

func main() {
	service.SetBackend(kernel.New())
	agent.Run(version)
}
