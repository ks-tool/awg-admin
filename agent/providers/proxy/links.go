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

package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ks-tool/vpn-agent/models"
)

type linkRecord struct {
	Role   string              `json:"role"`
	Offer  models.LinkOffer    `json:"offer"`
	Peer   *Peer               `json:"peer,omitempty"`
	Driver *models.RouteDriver `json:"driver,omitempty"`
}

type proxyLinkStore struct {
	root string
}

func newProxyLinkStore(root string) (*proxyLinkStore, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &proxyLinkStore{root: root}, nil
}

func (s *proxyLinkStore) put(linkID string, record linkRecord) error {
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return writePrivate(s.filename(linkID), data)
}

func (s *proxyLinkStore) get(linkID string) (linkRecord, error) {
	data, err := os.ReadFile(s.filename(linkID))
	if err != nil {
		return linkRecord{}, err
	}
	var record linkRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return linkRecord{}, err
	}
	return record, nil
}

func (s *proxyLinkStore) delete(linkID string) error {
	if err := os.Remove(s.filename(linkID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *proxyLinkStore) list() (map[string]linkRecord, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, err
	}
	records := make(map[string]linkRecord)
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".link" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.root, entry.Name()))
		if err != nil {
			return nil, err
		}
		var record linkRecord
		if err := json.Unmarshal(data, &record); err != nil {
			return nil, err
		}
		records[record.Offer.LinkID] = record
	}
	return records, nil
}

func (s *proxyLinkStore) filename(linkID string) string {
	sum := sha256.Sum256([]byte(linkID))
	return filepath.Join(s.root, hex.EncodeToString(sum[:])+".link")
}
