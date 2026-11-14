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
	"embed"
	"io/fs"

	"github.com/horchestra/targets/host/local"
)

//go:embed agentbin/pack-host-agent-linux-amd64 agentbin/pack-host-agent-linux-arm64
var hostAgentFiles embed.FS

var hostAgents = hostAgentSubsystem()

func hostAgentSubsystem() fs.FS {
	agents, err := fs.Sub(hostAgentFiles, "agentbin")
	if err != nil {
		panic(err)
	}
	return agents
}

func init() {
	local.SetAgents(hostAgents)
	local.Register()
}
