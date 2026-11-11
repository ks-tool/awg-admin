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

package hysteria2

import (
	"context"
	"fmt"
	"runtime"

	"github.com/ks-tool/vpn-agent/providers/proxy"
)

func (Adapter) Deploy(ctx context.Context, options map[string]any) (proxy.Deployment, error) {
	artifact, checksum, err := coreRelease(options)
	if err != nil {
		return proxy.Deployment{}, err
	}
	const corePath = "/usr/local/bin/hysteria"
	if err := proxy.DownloadArtifact(ctx, artifact, checksum, corePath, ""); err != nil {
		return proxy.Deployment{}, err
	}
	return proxy.Deployment{Port: 9093, CorePath: corePath, EnvPrefix: "VPN_PROVIDER_HYSTERIA2", Packages: []string{"iptables", "procps"}}, nil
}

func coreRelease(options map[string]any) (string, string, error) {
	var artifact, checksum string
	switch runtime.GOARCH {
	case "amd64":
		artifact = "https://github.com/HyNetworks/hysteria/releases/download/app/v2.13.0/hysteria-linux-amd64"
		checksum = "907ba8c9693edb104b20582681fb7dc15639d5b64a9cbb616a7b539190a86691"
	case "arm64":
		artifact = "https://github.com/HyNetworks/hysteria/releases/download/app/v2.13.0/hysteria-linux-arm64"
		checksum = "a68a61a84452ca250ce0368202521965ca9cc9d801a404f1dc9008ac6cf677a7"
	default:
		return "", "", fmt.Errorf("Hysteria 2 core does not support %s", runtime.GOARCH)
	}
	for name, value := range options {
		text, ok := value.(string)
		if !ok || len(text) == 0 {
			return "", "", fmt.Errorf("Hysteria 2 provider option %s must be a non-empty string", name)
		}
		switch name {
		case "coreArtifact":
			artifact = text
		case "coreSHA256":
			checksum = text
		default:
			return "", "", fmt.Errorf("unsupported Hysteria 2 provider option %q", name)
		}
	}
	return artifact, checksum, nil
}
