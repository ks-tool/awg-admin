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
	"github.com/ks-tool/awg-admin/models"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
)

type agentSources struct {
	db *bolt.DB
}

func (a *agentSources) List() ([]models.AgentSource, error) {
	return list[models.AgentSource](a.db, bktAgentSources)
}

func (a *agentSources) Get(id uuid.UUID) (*models.AgentSource, error) {
	var src models.AgentSource
	err := a.db.View(func(tx *bolt.Tx) error {
		var err error
		src, err = get[models.AgentSource](tx.Bucket(bktAgentSources), idKey(id))
		return err
	})
	if err != nil {
		return nil, err
	}
	return &src, nil
}

func (a *agentSources) Set(src *models.AgentSource) error {
	return a.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bktAgentSources), idKey(src.ID), src)
	})
}

func (a *agentSources) Delete(id uuid.UUID) error {
	return a.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bktAgentSources).Delete(idKey(id))
	})
}
