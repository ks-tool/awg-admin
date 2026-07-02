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
	"github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/agent/storage"

	"github.com/Jipok/wgctrl-go"
	"github.com/Jipok/wgctrl-go/wgtypes"
	"github.com/rs/zerolog/log"
)

type Handler struct {
	awg   *wgctrl.Client
	store storage.Storage
}

func NewHandler(store storage.Storage, awg *wgctrl.Client) *Handler {
	return &Handler{awg, store}
}

func (h *Handler) All() error {
	configs, err := h.store.List()
	if err != nil {
		return err
	}

	for _, cfg := range configs {
		// Startup re-apply: no distinct "previous" config to reconcile against,
		// so idempotent up-hooks just (re)assert the rules.
		if err = h.One(nil, cfg); err != nil {
			return err
		}
	}
	return nil
}

// StopEnabled tears down the OS link of every enabled interface (best effort),
// leaving the stored configs intact. It's called on agent shutdown so tunnels
// don't keep carrying traffic — and lifecycle PreDown/PostDown rules stay in
// effect — while the agent is gone. Disabled interfaces are skipped (their link
// is already down). On the next start, All → One re-creates the enabled ones.
func (h *Handler) StopEnabled() {
	configs, err := h.store.List()
	if err != nil {
		log.Warn().Err(err).Msg("shutdown: failed to list interfaces to stop")
		return
	}
	for _, cfg := range configs {
		if cfg.Disabled {
			continue
		}
		if err := InterfaceDelete(cfg); err != nil {
			log.Warn().Err(err).Str("interface", cfg.Interface).Msg("shutdown: failed to stop interface")
		}
	}
}

// One applies cfg to the host: creates or (in place) updates the link, then
// pushes the full device config. prev is the previously-stored config for this
// interface (nil for a brand-new one), used by InterfaceUpdate to reconcile
// hooks on an in-place edit.
func (h *Handler) One(prev *models.InterfaceConfig, cfg models.InterfaceConfig) error {
	logger := log.Debug().Str("interface", cfg.Interface)

	// A disabled interface must not be present in the kernel: tear its link down
	// if it exists (the stored config stays — this is desired state) and don't
	// push any device config. This is also what makes startup (All → One for
	// every stored config) bring up only the enabled interfaces.
	if cfg.Disabled {
		if IsInterfaceExist(cfg) {
			logger.Str("action", "deactivate").Send()
			return InterfaceDelete(cfg)
		}
		logger.Str("action", "skip-disabled").Send()
		return nil
	}

	if ok := IsInterfaceExist(cfg); ok {
		logger.Str("action", "update").Send()
		if err := InterfaceUpdate(prev, cfg); err != nil {
			return err
		}
	} else {
		logger.Str("action", "create").Send()
		if err := InterfaceCreate(cfg); err != nil {
			return err
		}
	}

	// Apply the FULL device config — private key, listen port, firewall mark,
	// AmneziaWG obfuscation params AND peers — not just the peer set.
	// ToAmneziaConfig builds all of it; ReplacePeers makes the peers authoritative.
	logger.Str("action", "configure").Send()
	awgCfg := cfg.ToAmneziaConfig()
	awgCfg.ReplacePeers = true
	return h.awg.ConfigureDevice(cfg.Interface, *awgCfg)
}

func (h *Handler) Delete(iface string) error {
	logger := log.Debug().Str("interface", iface)
	logger.Str("action", "delete").Send()

	// Load the stored config so InterfaceDelete can run its PreDown/PostDown
	// hooks — it's still present here; the API handler removes the JSON record
	// only after this returns. If it's already gone (an orphan link, or a
	// record removed by hand), tear the interface down anyway, hooklessly.
	cfg, err := h.store.Get(iface)
	if err != nil {
		return InterfaceDelete(models.InterfaceConfig{Interface: iface})
	}
	return InterfaceDelete(*cfg)
}

// DetectOrphans returns the name of every live WireGuard interface on the
// host that h's storage has no record of — e.g. a DELETE /interfaces/{name}
// call's JSON write succeeded but its netlink teardown didn't (shouldn't
// happen anymore, see InterfaceDelete, but old data or a manual `ip link`
// could still produce this), or the agent's storage was wiped/replaced
// without the corresponding interfaces being torn down first.
//
// Deliberately read-only: the agent can't tell "this is my own orphan" from
// "an administrator created an unrelated WireGuard interface by hand, with
// nothing to do with awg-admin" — deciding what to do about a mismatch is
// left to a human (see awg-admin's agent↔DB reconciliation), this just
// surfaces the list.
func (h *Handler) DetectOrphans() ([]string, error) {
	configs, err := h.store.List()
	if err != nil {
		return nil, err
	}

	devices, err := h.awg.Devices()
	if err != nil {
		return nil, err
	}

	return orphanInterfaces(configs, devices), nil
}

// orphanInterfaces is DetectOrphans's pure diffing logic, split out so it's
// testable without a real WireGuard device (h.awg.Devices() needs netlink
// and CAP_NET_ADMIN, neither available to a unit test).
func orphanInterfaces(configs []models.InterfaceConfig, devices []*wgtypes.Device) []string {
	known := make(map[string]bool, len(configs))
	for _, cfg := range configs {
		known[cfg.Interface] = true
	}

	var orphans []string
	for _, dev := range devices {
		if !known[dev.Name] {
			orphans = append(orphans, dev.Name)
		}
	}
	return orphans
}
