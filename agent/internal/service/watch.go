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
	"context"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

// configExt matches agent/storage/fs.go's own constant (kept separate
// rather than imported — fs.go's is unexported and this package can't
// reach into another module's storage backend's internals; the agent only
// ever has the one fs.Dir implementation anyway).
const configExt = ".json"

// deleteInterface is InterfaceDelete by default; tests override it to
// observe handleStorageEvent's behavior without touching netlink.
var deleteInterface = InterfaceDelete

// WatchStorage watches dir (the agent's config storage root, see
// agent/storage/fs.Dir) for interface config files disappearing while the
// agent is already running, and tears the corresponding link down
// immediately via InterfaceDelete — instead of leaving it dangling until
// the agent happens to restart (see Handler.DetectOrphans, which only
// catches this at startup).
//
// Unlike DetectOrphans, this needs no "is this really mine" judgment call:
// dir is a directory this agent exclusively writes (*iface*.json) into via
// Set/Delete (agent/storage/fs.go) — there is no other source for files in
// it, so any file disappearing from it by definition was one of this
// agent's own interface records, however it disappeared (DELETE
// /interfaces/{name}, which already calls InterfaceDelete itself before
// removing the file — so this is a harmless idempotent no-op for that
// case, see InterfaceDelete's own existence check — or someone removing
// the file by hand while the agent is up, which is the actual gap this
// closes). A plain function rather than a Handler method: tearing down a
// link only needs netlink (InterfaceDelete), not storage.Storage or a
// wgctrl.Client, so there's no reason to make the caller open one just
// for this.
//
// Runs until ctx is cancelled; logs (rather than returns) errors from
// individual events so a single bad event doesn't tear down the watch.
func WatchStorage(ctx context.Context, dir string) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return err
	}

	go func() {
		defer func() { _ = watcher.Close() }()
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				handleStorageEvent(event)
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Warn().Err(err).Msg("storage watcher error")
			}
		}
	}()
	return nil
}

// handleStorageEvent reacts to a single fsnotify event from WatchStorage.
// Split out from the goroutine above so it's unit-testable without an
// actual filesystem watch.
func handleStorageEvent(event fsnotify.Event) {
	if !event.Op.Has(fsnotify.Remove) {
		return // creates/writes are config pushes (PUT /interfaces), not deletions
	}

	iface := ifaceNameFromConfigPath(event.Name)
	if iface == "" {
		return // not one of our <interface>.json files
	}

	log.Info().Str("interface", iface).Msg(
		"interface config file removed while agent was running — tearing down the link")
	if err := deleteInterface(iface); err != nil {
		log.Error().Err(err).Str("interface", iface).Msg("failed to delete interface after its config file disappeared")
	}
}

// ifaceNameFromConfigPath returns the interface name encoded in path
// (agent/storage/fs.go names files "<interface>.json"), or "" if path
// doesn't look like one of those files at all.
func ifaceNameFromConfigPath(path string) string {
	base := filepath.Base(path)
	iface, ok := strings.CutSuffix(base, configExt)
	if !ok || iface == "" {
		return ""
	}
	return iface
}
