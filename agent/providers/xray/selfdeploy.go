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

package xray

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
	const corePath = "/usr/local/bin/xray"
	if err := proxy.DownloadArtifact(ctx, artifact, checksum, corePath, "xray"); err != nil {
		return proxy.Deployment{}, err
	}
	return proxy.Deployment{Port: 9092, CorePath: corePath, EnvPrefix: "VPN_PROVIDER_XRAY", Packages: []string{"iptables", "procps"}}, nil
}

func coreRelease(options map[string]any) (string, string, error) {
	var artifact, checksum string
	switch runtime.GOARCH {
	case "amd64":
		artifact = "https://github.com/XTLS/Xray-core/releases/download/v26.11.9/Xray-linux-64.zip"
		checksum = "b651f7e1d9966ee2c1b1ee1bf8b52c72771824d3065dce96bbc8bcc19ed95f82"
	case "arm64":
		artifact = "https://github.com/XTLS/Xray-core/releases/download/v26.11.9/Xray-linux-arm64-v8a.zip"
		checksum = "64ed6858982a7f5eb85d88b811089063bd4f17561f4c8f44e0f1dc8e6f6430be"
	default:
		return "", "", fmt.Errorf("Xray core does not support %s", runtime.GOARCH)
	}
	for name, value := range options {
		text, ok := value.(string)
		if !ok || len(text) == 0 {
			return "", "", fmt.Errorf("Xray provider option %s must be a non-empty string", name)
		}
		switch name {
		case "coreArtifact":
			artifact = text
		case "coreSHA256":
			checksum = text
		default:
			return "", "", fmt.Errorf("unsupported Xray provider option %q", name)
		}
	}
	return artifact, checksum, nil
}
