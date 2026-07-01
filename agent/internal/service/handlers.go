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
		if err = h.One(cfg); err != nil {
			return err
		}
	}
	return nil
}

func (h *Handler) One(cfg models.InterfaceConfig) error {
	logger := log.Debug().Str("interface", cfg.Interface)

	if ok := IsInterfaceExist(cfg); ok {
		logger.Str("action", "update").Send()
		if err := InterfaceUpdate(cfg); err != nil {
			return err
		}
	} else {
		logger.Str("action", "create").Send()
		if err := InterfaceCreate(cfg); err != nil {
			return err
		}
	}

	logger.Str("action", "configure").Send()
	return h.awg.ConfigureDevice(cfg.Interface, wgtypes.Config{ReplacePeers: true, Peers: cfg.ToWireguardPeers()})
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
