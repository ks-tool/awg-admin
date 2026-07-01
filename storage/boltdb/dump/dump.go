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

// Package dump reads/writes awg-admin's boltdb as a portable JSON snapshot of
// every bucket's raw key/value pairs — the format shared by the in-app backup
// (Service.Backup / the desktop "Backup" button / the standalone GET /backup)
// and the standalone awg-migrate CLI, so a backup taken from one can be
// restored with the other. It's deliberately schema-agnostic: it copies bytes,
// not models, so it doesn't need to know anything about the application schema
// (and isn't tripped up by the historical bktUsers/bktInterfaces bucket-name
// mismatch in storage/boltdb). The binary format of the values is an internal
// implementation detail, not a stable cross-version contract — a dump is only
// guaranteed to restore into the same awg-admin version it was taken from.
package dump

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	bolt "go.etcd.io/bbolt"
)

// Dump is the on-disk JSON shape: bucket name -> (hex-encoded key -> raw JSON
// value). Keys are hex-encoded rather than left as plain strings because
// several buckets (servers, interfaces, users) key their entries by raw
// 16-byte UUIDs, which aren't valid UTF-8/JSON string content as-is.
type Dump struct {
	Buckets map[string]map[string]json.RawMessage `json:"buckets"`
}

// Export writes a consistent snapshot of every bucket in db to w as indented
// JSON (bolt's db.View gives a point-in-time read transaction, so this is a
// safe hot backup while the database is in use).
func Export(db *bolt.DB, w io.Writer) error {
	d := Dump{Buckets: make(map[string]map[string]json.RawMessage)}
	err := db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
			entries := make(map[string]json.RawMessage)
			if err := b.ForEach(func(k, v []byte) error {
				entries[hex.EncodeToString(k)] = append(json.RawMessage{}, v...)
				return nil
			}); err != nil {
				return fmt.Errorf("read bucket %q: %w", name, err)
			}
			d.Buckets[string(name)] = entries
			return nil
		})
	})
	if err != nil {
		return err
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(d)
}

// Import reads a Dump from r and writes every key/value into db. With force
// false it aborts on the first key that already exists in the destination;
// with force true it overwrites.
func Import(db *bolt.DB, r io.Reader, force bool) error {
	var d Dump
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return fmt.Errorf("parse dump: %w", err)
	}

	return db.Update(func(tx *bolt.Tx) error {
		for bucketName, entries := range d.Buckets {
			b, err := tx.CreateBucketIfNotExists([]byte(bucketName))
			if err != nil {
				return fmt.Errorf("create bucket %q: %w", bucketName, err)
			}
			for hexKey, value := range entries {
				key, err := hex.DecodeString(hexKey)
				if err != nil {
					return fmt.Errorf("bucket %q: decode key %q: %w", bucketName, hexKey, err)
				}
				if !force && b.Get(key) != nil {
					return fmt.Errorf("bucket %q: key %s already exists (use -force to overwrite)", bucketName, hexKey)
				}
				if err := b.Put(key, value); err != nil {
					return fmt.Errorf("bucket %q: write key %s: %w", bucketName, hexKey, err)
				}
			}
		}
		return nil
	})
}
