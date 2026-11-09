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

package config

import (
	"os"
	"path/filepath"
)

func Env(name string) string {
	if value := os.Getenv("VPN_ADMIN_" + name); value != "" {
		return value
	}
	return os.Getenv("AWG_ADMIN_" + name)
}

func AdminDatabasePath(home string) string {
	root := filepath.Join(home, ".vpn-admin")
	current := filepath.Join(root, "db")
	if _, err := os.Stat(current); err == nil {
		return current
	}
	legacy := filepath.Join(home, ".awg-admin")
	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}
	_ = os.MkdirAll(root, 0700)
	return current
}

func AdminAutocertPath(home string) string {
	return existingPath(home, filepath.Join(".vpn-admin", "acme"), ".awg-admin-autocert")
}

func AdminPluginsPath(home string) string {
	return existingPath(home, filepath.Join(".vpn-admin", "plugins"), ".awg-admin-plugins")
}

func AdminCachePath(home string) string {
	return filepath.Join(home, ".vpn-admin", "cache")
}

func existingPath(home, current, legacy string) string {
	currentPath := filepath.Join(home, current)
	if _, err := os.Stat(currentPath); err == nil {
		return currentPath
	}
	legacyPath := filepath.Join(home, legacy)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}
	return currentPath
}
