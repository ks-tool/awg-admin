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

	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

type users struct {
	db *bolt.DB
}

func (u *users) List() ([]models.User, error) {
	return list[models.User](u.db, bktUsers)
}

func (u *users) Get(id uuid.UUID) (*models.User, error) {
	var user models.User
	err := u.db.View(func(tx *bolt.Tx) error {
		var err error
		user, err = get[models.User](tx.Bucket(bktUsers), idKey(id))
		return err
	})
	return &user, err
}

func (u *users) Set(user *models.User) error {
	return u.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bktUsers)
		return put(b, idKey(user.ID), user)
	})
}

func (u *users) Delete(id uuid.UUID) error {
	return u.db.Update(func(tx *bolt.Tx) error {
		ub := tx.Bucket(bktUsers)
		user, err := get[models.User](ub, idKey(id))
		if err != nil {
			return err
		}
		ib := tx.Bucket(bktInterfaces)
		for _, peer := range user.Peers {
			if err = removePeerFromInterface(ib, peer.InterfaceId, peer.PrivateKey.PublicKey()); err != nil {
				return fmt.Errorf("cascade remove peer from interface %s: %w", peer.InterfaceId, err)
			}
		}
		return ub.Delete(idKey(id))
	})
}

func (u *users) Peers(userID uuid.UUID) storage.Peers {
	return &peers{db: u.db, userID: userID}
}
