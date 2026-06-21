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

package boltdb

import (
	"fmt"
	"net"

	"github.com/google/uuid"
	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage"
	bolt "go.etcd.io/bbolt"
)

type interfaces struct {
	db       *bolt.DB
	serverID uuid.UUID
}

func (i *interfaces) List() ([]models.Interface, error) {
	out := make([]models.Interface, 0)
	err := i.db.View(func(tx *bolt.Tx) error {
		srv, err := get[models.Server](tx.Bucket(bktServers), idKey(i.serverID))
		if err != nil {
			return err
		}
		ib := tx.Bucket(bktInterfaces)
		for _, ifID := range srv.Interfaces {
			iface, err := get[models.Interface](ib, idKey(ifID))
			if err != nil {
				continue // skip stale references
			}
			out = append(out, iface)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (i *interfaces) Get(id uuid.UUID) (*models.Interface, error) {
	var iface models.Interface
	err := i.db.View(func(tx *bolt.Tx) error {
		var err error
		iface, err = get[models.Interface](tx.Bucket(bktInterfaces), idKey(id))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &iface, nil
}

func (i *interfaces) Set(iface *models.Interface) error {
	return i.db.Update(func(tx *bolt.Tx) error {
		sb := tx.Bucket(bktServers)
		srv, err := get[models.Server](sb, idKey(i.serverID))
		if err != nil {
			return fmt.Errorf("server: %w", err)
		}

		exists := false
		for _, id := range srv.Interfaces {
			if id == iface.ID {
				exists = true
				break
			}
		}
		if !exists {
			// Brand new interface: peers are managed separately, never
			// accepted on create. On update, leave iface.Peers as the
			// caller set it (e.g. preserved from a prior Get) instead of
			// wiping it and instead of re-appending the ID to srv.Interfaces.
			iface.Peers = nil
			srv.Interfaces = append(srv.Interfaces, iface.ID)
			if err = put(sb, idKey(i.serverID), srv); err != nil {
				return err
			}
		}

		ib := tx.Bucket(bktInterfaces)
		return put(ib, idKey(iface.ID), iface)
	})
}

func (i *interfaces) Delete(id uuid.UUID) error {
	return i.db.Update(func(tx *bolt.Tx) error {
		sb := tx.Bucket(bktServers)
		srv, err := get[models.Server](sb, idKey(i.serverID))
		if err != nil {
			return err
		}

		ib := tx.Bucket(bktInterfaces)
		if ib.Get(idKey(id)) == nil {
			return storage.ErrNotFound
		}

		newList := make([]uuid.UUID, 0, len(srv.Interfaces))
		for _, existing := range srv.Interfaces {
			if existing != id {
				newList = append(newList, existing)
			}
		}
		srv.Interfaces = newList
		if err = put(sb, idKey(i.serverID), srv); err != nil {
			return err
		}
		return ib.Delete(idKey(id))
	})
}

func (i *interfaces) UsedIPs(ifaceID uuid.UUID) ([]net.IPNet, error) {
	iface, err := i.Get(ifaceID)
	if err != nil {
		return nil, err
	}

	var result []net.IPNet
	if _, ipNet, err := net.ParseCIDR(iface.Address); err == nil {
		result = append(result, *ipNet)
	}
	for _, peer := range iface.Peers {
		for _, allowedIP := range peer.AllowedIPs {
			_, ipNet, err := net.ParseCIDR(allowedIP)
			if err != nil {
				continue
			}
			result = append(result, *ipNet)
		}
	}

	return result, nil
}
