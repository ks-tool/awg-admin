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

	"github.com/ks-tool/vpn-agent/providers/proxy"
)

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
	install := exec.CommandContext(ctx, "apt-get", "install", "-y", "iproute2", "procps")
	install.Stdout, install.Stderr = os.Stdout, os.Stderr
	if err := install.Run(); err != nil {
		return fmt.Errorf("install AWG dependencies: %w", err)
	}
	const sysctlPath = "/etc/sysctl.d/99-vpn-provider-awg.conf"
	if err := os.WriteFile(sysctlPath, []byte("net.ipv4.ip_forward=1\n"), 0o644); err != nil {
		return err
	}
	sysctl := exec.CommandContext(ctx, "sysctl", "--system")
	sysctl.Stdout, sysctl.Stderr = os.Stdout, os.Stderr
	if err := sysctl.Run(); err != nil {
		return fmt.Errorf("apply AWG sysctls: %w", err)
	}
	deployment := proxy.Deployment{Port: 9091, EnvPrefix: "VPN_PROVIDER_AWG"}
	if userspace {
		deployment.Args = []string{"--userspace"}
	}
	return proxy.WriteRegistration("awg", config.AgentDB, deployment)
}
