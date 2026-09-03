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
	"os"
	"path/filepath"
	"testing"

	"github.com/ks-tool/awg-admin/agent/storage"
)

func TestReadImportSourcePrefersAmneziaDir(t *testing.T) {
	amnezia := t.TempDir()
	wg := t.TempDir()
	if err := os.WriteFile(filepath.Join(amnezia, "wg0.conf"), []byte("[Interface]\n# awg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wg, "wg0.conf"), []byte("[Interface]\n# wg\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wg, "wg1.conf"), []byte("[Interface]\n# wg1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	dirs := []string{amnezia, wg}

	src, err := readImportSource(dirs, "wg0")
	if err != nil {
		t.Fatalf("wg0: %v", err)
	}
	if src.Path != filepath.Join(amnezia, "wg0.conf") || src.Content != "[Interface]\n# awg\n" {
		t.Fatalf("wg0 = %+v, want the amnezia copy", src)
	}

	src, err = readImportSource(dirs, "wg1")
	if err != nil {
		t.Fatalf("wg1: %v", err)
	}
	if src.Path != filepath.Join(wg, "wg1.conf") {
		t.Fatalf("wg1 path = %q, want the wireguard dir fallback", src.Path)
	}

	_, err = readImportSource(dirs, "wg9")
	if !storage.IsNotFound(err) {
		t.Fatalf("wg9: err = %v, want ErrNotFound", err)
	}
}

// The name is joined onto the conf directories, so anything that could walk
// out of them (or isn't an interface name at all) must be rejected before any
// filesystem access.
func TestReadImportSourceRejectsUnsafeNames(t *testing.T) {
	dirs := []string{t.TempDir()}
	for _, name := range []string{"", ".", "..", "../etc/passwd", "a/b", "wg 0", "this-name-is-way-too-long"} {
		_, err := readImportSource(dirs, name)
		if !errors.Is(err, ErrInvalidInterfaceName) {
			t.Errorf("%q: err = %v, want ErrInvalidInterfaceName", name, err)
		}
	}
	for _, name := range []string{"wg0", "awg-vpn.1", "a_b=c+d"} {
		if !validImportName(name) {
			t.Errorf("%q: expected a valid interface name", name)
		}
	}
}
