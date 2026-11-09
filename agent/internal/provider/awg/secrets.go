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

package awg

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ks-tool/vpn-admin/agent/models"
	"github.com/ks-tool/vpn-admin/agent/storage"
)

type secretRecord struct {
	Interface         string     `json:"interface"`
	PrivateKey        models.Key `json:"privateKey"`
	PreviousPublicKey string     `json:"previousPublicKey,omitempty"`
}

type secretStore struct {
	root string
}

func newSecretStore(root string) (*secretStore, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &secretStore{root: root}, nil
}

func (s *secretStore) get(resourceID string) (secretRecord, error) {
	data, err := os.ReadFile(s.filename(resourceID))
	if err != nil {
		if os.IsNotExist(err) {
			return secretRecord{}, storage.ErrNotFound
		}
		return secretRecord{}, err
	}
	var record secretRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return secretRecord{}, err
	}
	return record, nil
}

func (s *secretStore) put(resourceID string, record secretRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.root, ".key-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.filename(resourceID))
}

func (s *secretStore) delete(resourceID string) error {
	if err := os.Remove(s.filename(resourceID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *secretStore) filename(resourceID string) string {
	sum := sha256.Sum256([]byte(resourceID))
	return filepath.Join(s.root, hex.EncodeToString(sum[:])+".secret")
}
