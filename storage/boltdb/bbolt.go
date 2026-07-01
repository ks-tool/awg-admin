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
	"io"
	"time"

	"github.com/ks-tool/awg-admin/models"
	"github.com/ks-tool/awg-admin/storage"
	"github.com/ks-tool/awg-admin/storage/boltdb/dump"

	"github.com/google/uuid"
	bolt "go.etcd.io/bbolt"
	"golang.org/x/crypto/bcrypt"
)

const (
	configKey              = "config"
	serversBucketName      = "servers"
	usersBucketName        = "users"
	interfacesBucketName   = "interfaces"
	authBucketName         = "auth"
	agentSourcesBucketName = "agent_sources"
)

var (
	bktServers      = []byte(serversBucketName)
	bktInterfaces   = []byte(interfacesBucketName)
	bktUsers        = []byte(usersBucketName)
	bktAuth         = []byte(authBucketName)
	bktAgentSources = []byte(agentSourcesBucketName)
)

// authRecordKey is the auth bucket's one and only key — there's a single
// admin account, not a collection.
var authRecordKey = []byte("credentials")

// Default admin account seeded on first run (empty auth bucket) so
// standalone web-server mode has something to log in with before the user
// changes it via Service.ChangeCredentials.
const (
	defaultAdminUsername = "admin"
	defaultAdminPassword = "admin"
)

type BoltDB struct {
	db *bolt.DB
}

func Open(path string) (*BoltDB, error) {
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, err
	}

	err = db.Update(func(tx *bolt.Tx) error {
		if _, err = tx.CreateBucketIfNotExists(bktServers); err != nil {
			return err
		}
		if _, err = tx.CreateBucketIfNotExists(bktInterfaces); err != nil {
			return err
		}
		if _, err = tx.CreateBucketIfNotExists(bktUsers); err != nil {
			return err
		}

		if _, err = tx.CreateBucketIfNotExists(bktAgentSources); err != nil {
			return err
		}

		ab, err := tx.CreateBucketIfNotExists(bktAuth)
		if err != nil {
			return err
		}
		if ab.Get(authRecordKey) != nil {
			return nil
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(defaultAdminPassword), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		return put(ab, authRecordKey, &models.AuthCredentials{
			Username:     defaultAdminUsername,
			PasswordHash: string(hash),
		})
	})
	if err != nil {
		return nil, err
	}

	return &BoltDB{db: db}, nil
}

func (db *BoltDB) Close() error                       { return db.db.Close() }
func (db *BoltDB) Backup(w io.Writer) error           { return dump.Export(db.db, w) }
func (db *BoltDB) Users() storage.Users               { return &users{db.db} }
func (db *BoltDB) Servers() storage.Servers           { return &servers{db: db.db} }
func (db *BoltDB) Auth() storage.Auth                 { return &auth{db: db.db} }
func (db *BoltDB) AgentSources() storage.AgentSources { return &agentSources{db: db.db} }

// ─────────────────────────────── helpers ───────────────────────────────────

func idKey(id uuid.UUID) []byte {
	b := [16]byte(id)
	return b[:]
}

func get[T any](b *bolt.Bucket, key []byte) (T, error) {
	var out T
	v := b.Get(key)
	if v == nil {
		return out, storage.ErrNotFound
	}

	if err := json.Unmarshal(v, &out); err != nil {
		return out, err
	}
	return out, nil
}

func put(b *bolt.Bucket, key []byte, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return b.Put(key, data)
}

func list[T any](db *bolt.DB, bucket []byte) ([]T, error) {
	out := make([]T, 0)
	err := db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucket).ForEach(func(_, v []byte) error {
			var user T
			if err := json.Unmarshal(v, &user); err != nil {
				return err
			}
			out = append(out, user)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
