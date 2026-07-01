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
	"fmt"
	"os/exec"
	"strings"

	"github.com/ks-tool/awg-admin/agent/models"

	"github.com/rs/zerolog/log"
)

func InterfaceCreate(cfg models.InterfaceConfig) error {
	dev := Device(cfg.Interface)

	if err := runHooks(cfg.Interface, "PreUp", cfg.PreUp); err != nil {
		return err
	}
	if err := dev.Add(); err != nil {
		return err
	}
	if err := dev.AddrAdd(cfg.Address); err != nil {
		return err
	}
	if cfg.MTU > 0 {
		if err := dev.SetMTU(cfg.MTU); err != nil {
			return err
		}
	}
	if err := dev.Up(); err != nil {
		return err
	}

	return runHooks(cfg.Interface, "PostUp", cfg.PostUp)
}

func InterfaceUpdate(cfg models.InterfaceConfig) error {
	dev := Device(cfg.Interface)

	if err := dev.SyncAddr(cfg.Address); err != nil {
		return err
	}
	if cfg.MTU > 0 {
		if err := dev.SetMTU(cfg.MTU); err != nil {
			return err
		}
	}
	return dev.Up()
}

// InterfaceDelete tears cfg's interface down and removes the OS-level link
// itself (netlink.LinkDel) — without the LinkDel call, DELETE
// /interfaces/{name} only dropped the agent's JSON record while the actual
// `ip link` survived on the host. Idempotent: a link that's already gone
// (e.g. removed by hand) is treated as already deleted rather than an error.
//
// PreDown/PostDown hooks run around the teardown but, unlike InterfaceCreate's
// PreUp/PostUp (which abort on failure), are best-effort: removing the link is
// the whole point here, so a failing hook is logged and teardown continues.
func InterfaceDelete(cfg models.InterfaceConfig) error {
	dev := Device(cfg.Interface)

	if _, err := dev.Get(); err != nil {
		return nil
	}

	if err := runHooks(cfg.Interface, "PreDown", cfg.PreDown); err != nil {
		log.Warn().Err(err).Str("interface", cfg.Interface).Msg("PreDown hook failed, continuing teardown")
	}
	if err := dev.Down(); err != nil {
		return err
	}
	if err := runHooks(cfg.Interface, "PostDown", cfg.PostDown); err != nil {
		log.Warn().Err(err).Str("interface", cfg.Interface).Msg("PostDown hook failed, continuing teardown")
	}

	return dev.Delete()
}

// runHooks runs the shell commands of one lifecycle phase (PreUp/PostUp/
// PreDown/PostDown) for iface, in order. Each command is executed via
// `sh -c`, so it may be an arbitrary shell snippet, with wg-quick's `%i`
// placeholder replaced by the interface name (e.g.
// "iptables -A FORWARD -i %i -j ACCEPT"). It stops at the first failing
// command and returns its error with the command's combined output attached.
func runHooks(iface, phase string, cmds []string) error {
	for _, raw := range cmds {
		cmd := strings.ReplaceAll(raw, "%i", iface)
		out, err := exec.Command("sh", "-c", cmd).CombinedOutput()
		output := strings.TrimSpace(string(out))
		if err != nil {
			return fmt.Errorf("%s hook %q: %w: %s", phase, cmd, err, output)
		}
		log.Debug().Str("interface", iface).Str("phase", phase).
			Str("cmd", cmd).Str("output", output).Msg("ran interface hook command")
	}
	return nil
}

func IsInterfaceExist(cfg models.InterfaceConfig) bool {
	dev := Device(cfg.Interface)
	_, err := dev.Get()
	return err == nil
}
