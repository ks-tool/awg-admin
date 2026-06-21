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
)

func InterfaceCreate(cfg models.InterfaceConfig) error {
	dev := Device(cfg.Interface)

	// TODO PreUP
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

	// TODO PostUp
	return nil
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

// InterfaceDelete tears iface down and removes the OS-level link itself
// (netlink.LinkDel) — without the LinkDel call, DELETE /interfaces/{name}
// only dropped the agent's JSON record while the actual `ip link` survived
// on the host. Idempotent: a link that's already gone (e.g. removed by
// hand) is treated as already deleted rather than an error.
func InterfaceDelete(iface string) error {
	dev := Device(iface)

	if _, err := dev.Get(); err != nil {
		return nil
	}

	// TODO PreDown
	if err := dev.Down(); err != nil {
		return err
	}
	// TODO PostDown

	return dev.Delete()
}

func IsInterfaceExist(cfg models.InterfaceConfig) bool {
	dev := Device(cfg.Interface)
	_, err := dev.Get()
	return err == nil
}
