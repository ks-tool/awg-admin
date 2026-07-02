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

// Package userspace provides a service.Backend that runs each interface as a
// userspace amneziawg-go process instead of a kernel device — for hosts without
// a (matching) AmneziaWG kernel module. It's wired in by the awg-agent-userspace
// binary via service.SetBackend; the rest of the agent is unchanged.
package userspace

import (
	"fmt"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/ks-tool/awg-admin/agent/internal/service"

	"github.com/rs/zerolog/log"
)

// linkReadyTimeout bounds how long Add waits for amneziawg-go to create the TUN
// (and, alongside it, its UAPI socket) before the agent proceeds to assign the
// address and push the device config over it.
const linkReadyTimeout = 5 * time.Second

// Backend runs interfaces as userspace amneziawg-go processes. It embeds
// service.NetlinkBackend so the address/MTU/up/down and existence operations run
// as plain netlink on the TUN amneziawg-go creates (a userspace TUN is an
// ordinary link there) — only Add (start the process) and Delete (stop it)
// differ from the kernel backend.
//
// The device config itself is still pushed and read back through wgctrl, which
// finds the process's UAPI socket under /var/run/amneziawg (or /var/run/wireguard)
// — the same code path used for the kernel module.
type Backend struct {
	service.NetlinkBackend

	// bin is the amneziawg-go executable (path or name resolved on $PATH).
	bin string

	mu    sync.Mutex
	procs map[string]*exec.Cmd
}

// New returns a userspace Backend that launches amneziawg-go from bin, falling
// back to "amneziawg-go" (resolved on $PATH) when bin is empty.
func New(bin string) *Backend {
	if bin == "" {
		bin = "amneziawg-go"
	}
	return &Backend{bin: bin, procs: make(map[string]*exec.Cmd)}
}

// Add starts an amneziawg-go process for iface and waits for its TUN to appear
// so the address/up/configure steps that follow have a link to act on.
func (b *Backend) Add(iface string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.procs[iface]; ok {
		return nil // already running
	}

	// -f keeps amneziawg-go in the foreground so this process stays its parent
	// and can stop it in Delete — a backgrounded daemon would fork away, leaving
	// no handle. The TUN is removed when the process exits.
	cmd := exec.Command(b.bin, "-f", iface)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s for %q: %w", b.bin, iface, err)
	}
	b.procs[iface] = cmd

	// Reap the process and drop it from the map when it exits (crash or Delete),
	// so a later Add for the same name starts a fresh one rather than short-
	// circuiting on a dead entry.
	go func() {
		waitErr := cmd.Wait()
		b.mu.Lock()
		if b.procs[iface] == cmd {
			delete(b.procs, iface)
		}
		b.mu.Unlock()
		log.Debug().Err(waitErr).Str("interface", iface).Msg("amneziawg-go process exited")
	}()

	deadline := time.Now().Add(linkReadyTimeout)
	for time.Now().Before(deadline) {
		if b.NetlinkBackend.Exists(iface) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("amneziawg-go did not create interface %q within %s", iface, linkReadyTimeout)
}

// Delete stops the amneziawg-go process for iface (if this backend started it),
// which removes the TUN. Idempotent: an interface this backend didn't start
// (e.g. left over from a previous run) is a no-op here — the caller's netlink
// teardown still runs.
func (b *Backend) Delete(iface string) error {
	b.mu.Lock()
	cmd, ok := b.procs[iface]
	if ok {
		delete(b.procs, iface)
	}
	b.mu.Unlock()

	if !ok {
		return nil
	}
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop amneziawg-go for %q: %w", iface, err)
	}
	return nil
}
