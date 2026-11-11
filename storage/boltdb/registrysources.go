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
	"sort"
	"uuid"

	"github.com/ks-tool/vpn-admin/models"
	bolt "go.etcd.io/bbolt"
)

type registrySources struct {
	db *bolt.DB
}

func (r *registrySources) List() ([]models.RegistrySource, error) {
	sources, err := list[models.RegistrySource](r.db, bktRegistrySources)
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	return sources, err
}

func (r *registrySources) Get(id uuid.UUID) (*models.RegistrySource, error) {
	var source models.RegistrySource
	err := r.db.View(func(tx *bolt.Tx) error {
		var err error
		source, err = get[models.RegistrySource](tx.Bucket(bktRegistrySources), idKey(id))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &source, nil
}

func (r *registrySources) Set(source *models.RegistrySource) error {
	return r.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bktRegistrySources), idKey(source.ID), source)
	})
}

func (r *registrySources) Delete(id uuid.UUID) error {
	return r.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktRegistrySources).Delete(idKey(id))
	})
}
