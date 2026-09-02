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

package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/agent/storage"
)

// importConfDirs lists, in order of precedence, where awg-quick and wg-quick
// keep their per-interface configuration files (<dir>/<name>.conf). The
// AmneziaWG location is tried first: a host running both would have the
// AmneziaWG copy for an interface it brought up with awg-quick.
var importConfDirs = []string{"/etc/amnezia/amneziawg", "/etc/wireguard"}

// ErrInvalidInterfaceName is returned by ReadImportSource for a name that
// isn't a valid Linux interface name — notably anything with a path separator
// or ".." — so the name can never escape the conf directories.
var ErrInvalidInterfaceName = errors.New("invalid interface name")

// ifaceNameRe is the Linux interface-name alphabet (IFNAMSIZ-1 = 15 chars, no
// '/' and no whitespace); "." and ".." are excluded below.
var ifaceNameRe = regexp.MustCompile(`^[A-Za-z0-9_.=+-]{1,15}$`)

// validImportName reports whether name is safe to join onto a conf directory.
func validImportName(name string) bool {
	return name != "." && name != ".." && ifaceNameRe.MatchString(name)
}

// ReadImportSource returns the wg-quick/awg-quick conf file for the named
// interface from the host's standard locations (importConfDirs), the first one
// found winning. storage.ErrNotFound when none exists (→ HTTP 404),
// ErrInvalidInterfaceName for a name that could escape those directories.
func ReadImportSource(name string) (*models.ImportSource, error) {
	return readImportSource(importConfDirs, name)
}

// readImportSource is ReadImportSource with the search dirs injected, so tests
// can point it at a temp directory.
func readImportSource(dirs []string, name string) (*models.ImportSource, error) {
	if !validImportName(name) {
		return nil, fmt.Errorf("%w: %q", ErrInvalidInterfaceName, name)
	}
	for _, dir := range dirs {
		path := filepath.Join(dir, name+".conf")
		data, err := os.ReadFile(path)
		if err == nil {
			return &models.ImportSource{Path: path, Content: string(data)}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
	}
	return nil, fmt.Errorf("no wg-quick config for interface %q in %v: %w", name, dirs, storage.ErrNotFound)
}
