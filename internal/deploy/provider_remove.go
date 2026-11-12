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
	"context"
	"fmt"
	"os"

	"github.com/horchestra/pack/sdk"
	"github.com/horchestra/pack/sdk/packschema"
	"github.com/ks-tool/vpn-admin/internal/config"
	"github.com/ks-tool/vpn-admin/models"
)

type providerRemovalPack struct {
	name string
}

func (*providerRemovalPack) SetDefaults() {}

func (p *providerRemovalPack) Meta() sdk.Meta {
	return sdk.Meta{Name: "vpn-provider-" + p.name + "-remove", Version: "1.0.0", Description: "Remove VPN provider", License: "Apache-2.0"}
}

func (p *providerRemovalPack) PackRoot() packschema.Root {
	commands := [][]string{
		{"rm", "-f", "/usr/local/bin/vpn-provider-" + p.name, "/var/lib/vpn-agent/providers.d/" + p.name + ".conf"},
	}
	return packschema.Root{Packages: map[string]packschema.OSInstall{
		"debian": {Commands: commands},
		"rhel":   {Commands: commands},
	}}
}

func RemoveProviderFromPack(ctx context.Context, srv models.Server, name, passphrase, sudoPassword string) error {
	if !validProviderName(name) {
		return fmt.Errorf("invalid provider name %q", name)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	item := packRelease{name: name, step: "remove_provider_" + name, pack: &providerRemovalPack{name: name}, values: map[string]any{}}
	if err := applyPack(ctx, srv, item, passphrase, sudoPassword, config.AdminCachePath(home)); err != nil {
		return fmt.Errorf("remove %s provider: %w", name, err)
	}
	return nil
}

func validProviderName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, char := range name {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}
