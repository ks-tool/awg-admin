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

package providerdeploy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ks-tool/vpn-agent/providers/proxy"
)

func run(ctx context.Context, action, name string, args ...string) error {
	command := exec.CommandContext(ctx, name, args...)
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}

func Deploy(ctx context.Context, configPath string) error {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	var config proxy.DeployConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	if len(config.AgentDB) == 0 {
		return errors.New("agentDB is required")
	}
	userspace := false
	for name, value := range config.Options {
		if name != "userspace" {
			return fmt.Errorf("unsupported AWG provider option %q", name)
		}
		enabled, ok := value.(bool)
		if !ok {
			return errors.New("AWG provider option userspace must be a boolean")
		}
		userspace = enabled
	}
	packages := []string{"ca-certificates", "iproute2", "procps"}
	if !userspace {
		kernel, err := exec.CommandContext(ctx, "uname", "-r").Output()
		if err != nil {
			return fmt.Errorf("detect running kernel: %w", err)
		}
		packages = append(packages, "gnupg2", "kmod", "linux-headers-"+strings.TrimSpace(string(kernel)))
	}
	if err := run(ctx, "install AWG dependencies", "apt-get", append([]string{"install", "-y"}, packages...)...); err != nil {
		return err
	}
	if !userspace {
		if err := run(ctx, "trust Amnezia PPA", "apt-key", "adv", "--keyserver", "keyserver.ubuntu.com", "--recv-keys", "75C9DD72C799870E310542E24166F2C257290828"); err != nil {
			return err
		}
		const repository = "deb https://ppa.launchpadcontent.net/amnezia/ppa/ubuntu focal main\ndeb-src https://ppa.launchpadcontent.net/amnezia/ppa/ubuntu focal main\n"
		if err := os.WriteFile("/etc/apt/sources.list.d/amneziawg.list", []byte(repository), 0o644); err != nil {
			return err
		}
		if err := run(ctx, "refresh APT repositories", "apt-get", "update"); err != nil {
			return err
		}
		if err := run(ctx, "install AmneziaWG kernel module", "apt-get", "install", "-y", "amneziawg"); err != nil {
			return err
		}
		if err := os.WriteFile("/etc/modules-load.d/amneziawg.conf", []byte("amneziawg\n"), 0o644); err != nil {
			return err
		}
		if err := run(ctx, "load AmneziaWG kernel module", "modprobe", "amneziawg"); err != nil {
			return err
		}
	}
	const sysctlPath = "/etc/sysctl.d/99-vpn-provider-awg.conf"
	if err := os.WriteFile(sysctlPath, []byte("net.ipv4.ip_forward=1\n"), 0o644); err != nil {
		return err
	}
	if err := run(ctx, "apply AWG sysctls", "sysctl", "--system"); err != nil {
		return err
	}
	deployment := proxy.Deployment{Port: 9091, EnvPrefix: "VPN_PROVIDER_AWG"}
	if userspace {
		deployment.Args = []string{"--userspace"}
	}
	return proxy.WriteRegistration("awg", config.AgentDB, deployment)
}
