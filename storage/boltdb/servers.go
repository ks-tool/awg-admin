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
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage"

	bolt "go.etcd.io/bbolt"
)

type servers struct {
	db *bolt.DB
}

func (s *servers) List() ([]models.Server, error) {
	return list[models.Server](s.db, bktServers)
}

func (s *servers) Get(id uuid.UUID) (*models.Server, error) {
	var srv models.Server
	err := s.db.View(func(tx *bolt.Tx) error {
		var err error
		srv, err = get[models.Server](tx.Bucket(bktServers), idKey(id))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &srv, err
}

func (s *servers) Set(srv *models.Server) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktServers)
		return put(b, idKey(srv.ID), srv)
	})
}

func (s *servers) Delete(id uuid.UUID) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		sb := tx.Bucket(bktServers)
		p, err := get[models.Server](sb, idKey(id))
		if err != nil {
			return err
		}

		if err := cascadeDeletePeers(tx.Bucket(bktUsers), p.Interfaces); err != nil {
			return fmt.Errorf("cascade delete peers: %w", err)
		}

		ib := tx.Bucket(bktInterfaces)
		for _, ifID := range p.Interfaces {
			if err := ib.Delete(idKey(ifID)); err != nil {
				return fmt.Errorf("cascade delete interface %s: %w", ifID, err)
			}
		}
		return sb.Delete(idKey(id))
	})
}

// cascadeDeletePeers removes every user's peer attached to one of ifaceIDs —
// called when those interfaces' owning server is being deleted, since
// peers are stored under users, not under the server/interface they
// belong to, and would otherwise be left dangling. Updates are collected
// during ForEach and applied afterwards: bbolt disallows mutating a
// bucket while iterating it.
func cascadeDeletePeers(ub *bolt.Bucket, ifaceIDs []uuid.UUID) error {
	if len(ifaceIDs) == 0 {
		return nil
	}
	ifaceSet := make(map[uuid.UUID]struct{}, len(ifaceIDs))
	for _, ifID := range ifaceIDs {
		ifaceSet[ifID] = struct{}{}
	}

	type update struct {
		key  []byte
		user models.User
	}
	var updates []update
	err := ub.ForEach(func(k, v []byte) error {
		var u models.User
		if err := json.Unmarshal(v, &u); err != nil {
			return err
		}

		filtered := make([]models.Peer, 0, len(u.Peers))
		changed := false
		for _, peer := range u.Peers {
			if _, ok := ifaceSet[peer.InterfaceId]; ok {
				changed = true
				continue
			}
			filtered = append(filtered, peer)
		}
		if changed {
			u.Peers = filtered
			updates = append(updates, update{key: append([]byte{}, k...), user: u})
		}
		return nil
	})
	if err != nil {
		return err
	}

	for _, upd := range updates {
		if err := put(ub, upd.key, upd.user); err != nil {
			return err
		}
	}
	return nil
}

func (s *servers) Interfaces(serverID uuid.UUID) storage.Interfaces {
	return &interfaces{db: s.db, serverID: serverID}
}
