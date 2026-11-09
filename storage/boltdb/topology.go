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
	"github.com/ks-tool/vpn-admin/models"
	"github.com/ks-tool/vpn-admin/storage"

	bolt "go.etcd.io/bbolt"
)

var topologyRecordKey = []byte("main")

func emptyTopology() *models.Topology {
	return &models.Topology{
		Servers:     make([]models.TopologyServer, 0),
		Interfaces:  make([]models.NetworkInterface, 0),
		Services:    make([]models.VPNService, 0),
		Connections: make([]models.NetworkConnection, 0),
	}
}

func (db *BoltDB) Topology() storage.Topology {
	return &topology{db: db.db}
}

type topology struct {
	db *bolt.DB
}

func (t *topology) Get() (*models.Topology, error) {
	result := emptyTopology()
	err := t.db.View(func(tx *bolt.Tx) error {
		value := tx.Bucket(bktTopology).Get(topologyRecordKey)
		if value == nil {
			return nil
		}
		decoded, err := get[models.Topology](tx.Bucket(bktTopology), topologyRecordKey)
		result = &decoded
		return err
	})
	if err != nil {
		return nil, err
	}
	normalizeTopology(result)
	return result, nil
}

func normalizeTopology(t *models.Topology) {
	if t.Servers == nil {
		t.Servers = make([]models.TopologyServer, 0)
	}
	if t.Interfaces == nil {
		t.Interfaces = make([]models.NetworkInterface, 0)
	}
	if t.Services == nil {
		t.Services = make([]models.VPNService, 0)
	}
	if t.Connections == nil {
		t.Connections = make([]models.NetworkConnection, 0)
	}
}

func (t *topology) Set(value *models.Topology) error {
	normalizeTopology(value)
	return t.db.Update(func(tx *bolt.Tx) error {
		return put(tx.Bucket(bktTopology), topologyRecordKey, value)
	})
}
