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

type resources struct {
	db       *bolt.DB
	serverID uuid.UUID
}

func (db *BoltDB) Resources(serverID uuid.UUID) storage.Resources {
	return &resources{db: db.db, serverID: serverID}
}

func (r *resources) List() ([]models.Resource, error) {
	items, err := list[models.Resource](r.db, bktResources)
	if err != nil {
		return nil, err
	}
	result := make([]models.Resource, 0)
	for _, item := range items {
		if item.ServerID == r.serverID.String() {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *resources) Get(id string) (*models.Resource, error) {
	var item models.Resource
	err := r.db.View(func(tx *bolt.Tx) error {
		var err error
		item, err = get[models.Resource](tx.Bucket(bktResources), []byte(id))
		return err
	})
	if err != nil {
		return nil, err
	}
	if item.ServerID != r.serverID.String() {
		return nil, storage.ErrNotFound
	}
	return &item, nil
}

func (r *resources) Set(resource *models.Resource) error {
	if resource.ServerID != r.serverID.String() {
		return fmt.Errorf("resource belongs to another server")
	}
	return r.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bktResources), []byte(resource.ID), resource)
	})
}

func (r *resources) Delete(id string) error {
	return r.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bktResources)
		item, err := get[models.Resource](bucket, []byte(id))
		if err != nil {
			return err
		}
		if item.ServerID != r.serverID.String() {
			return storage.ErrNotFound
		}
		return bucket.Delete([]byte(id))
	})
}

type links struct{ db *bolt.DB }

func (db *BoltDB) Links() storage.Links { return &links{db: db.db} }

func (l *links) List() ([]models.Link, error) { return list[models.Link](l.db, bktLinks) }
func (l *links) Get(id string) (*models.Link, error) {
	var item models.Link
	err := l.db.View(func(tx *bolt.Tx) error {
		var err error
		item, err = get[models.Link](tx.Bucket(bktLinks), []byte(id))
		return err
	})
	return &item, err
}
func (l *links) Set(link *models.Link) error {
	return l.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bktLinks), []byte(link.ID), link)
	})
}
func (l *links) Delete(id string) error {
	return l.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bktLinks)
		if bucket.Get([]byte(id)) == nil {
			return storage.ErrNotFound
		}
		return bucket.Delete([]byte(id))
	})
}

type providers struct{ db *bolt.DB }

func (db *BoltDB) Providers() storage.Providers { return &providers{db: db.db} }

func providerKey(serverID uuid.UUID, name string) []byte {
	return []byte(serverID.String() + "\x00" + name)
}

func (p *providers) List(serverID uuid.UUID) ([]models.ProviderRecord, error) {
	items, err := list[models.ProviderRecord](p.db, bktProviders)
	if err != nil {
		return nil, err
	}
	result := make([]models.ProviderRecord, 0)
	for _, item := range items {
		if item.ServerID == serverID.String() {
			result = append(result, item)
		}
	}
	return result, nil
}

func (p *providers) Get(serverID uuid.UUID, name string) (*models.ProviderRecord, error) {
	var item models.ProviderRecord
	err := p.db.View(func(tx *bolt.Tx) error {
		var err error
		item, err = get[models.ProviderRecord](tx.Bucket(bktProviders), providerKey(serverID, name))
		return err
	})
	return &item, err
}

func (p *providers) Set(record *models.ProviderRecord) error {
	serverID, err := uuid.Parse(record.ServerID)
	if err != nil {
		return err
	}
	return p.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bktProviders), providerKey(serverID, record.Definition.Name), record)
	})
}

func (p *providers) Delete(serverID uuid.UUID, name string) error {
	return p.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(bktProviders)
		key := providerKey(serverID, name)
		if bucket.Get(key) == nil {
			return storage.ErrNotFound
		}
		return bucket.Delete(key)
	})
}
