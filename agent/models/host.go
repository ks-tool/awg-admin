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

package models

// HostInfo is what the agent discovers about the host it runs on, gathered once
// at startup and served unchanged over GET /info (these facts don't change under
// a running agent). The admin uses it to show what a server supports — which
// interface kinds can be created, whether Docker is available — without having
// to SSH in and probe by hand.
type HostInfo struct {
	// Backend identifies which link backend this agent build drives: "kernel"
	// (amneziawg-dkms over netlink) or "userspace" (in-process amneziawg-go).
	Backend string `json:"backend"`
	// Version is the agent build version (see each main's -ldflags).
	Version string `json:"version"`
	// Docker reports whether Docker is usable on the host (the docker CLI is
	// present and `docker info` succeeds) — i.e. whether a docker-image agent
	// deploy would work there.
	Docker bool `json:"docker"`
	// InDocker reports whether the agent process itself runs inside a Docker
	// container, detected via the /.dockerenv file. Only the userspace agent
	// ships as a container image, so this is only ever true for it.
	InDocker bool `json:"inDocker"`
	// KernelModule reports whether the AmneziaWG kernel module is available on
	// the host (modinfo amneziawg succeeds — e.g. installed via dkms). Only
	// meaningful for the kernel backend; always false for userspace.
	KernelModule bool `json:"kernelModule"`
	// InterfaceKinds lists the interface variants this agent can create on this
	// host: "amneziawg" and/or "wireguard".
	InterfaceKinds []string `json:"interfaceKinds"`
}
