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

	"github.com/ks-tool/vpn-agent/storage"
)

type linkState struct {
	Interface     string   `json:"interface"`
	Role          string   `json:"role"`
	PeerPublicKey string   `json:"peerPublicKey"`
	Table         int      `json:"table,omitempty"`
	Priority      int      `json:"priority,omitempty"`
	Mark          string   `json:"mark,omitempty"`
	PreUp         []string `json:"preUp,omitempty"`
	PostDown      []string `json:"postDown,omitempty"`
}

type linkStore struct{ root string }

func newLinkStore(root string) (*linkStore, error) {
	if err := os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	return &linkStore{root: root}, nil
}

func (s *linkStore) get(linkID string) (linkState, error) {
	data, err := os.ReadFile(s.filename(linkID))
	if err != nil {
		if os.IsNotExist(err) {
			return linkState{}, storage.ErrNotFound
		}
		return linkState{}, err
	}
	var state linkState
	if err := json.Unmarshal(data, &state); err != nil {
		return linkState{}, err
	}
	return state, nil
}

func (s *linkStore) put(linkID string, state linkState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(s.root, ".link-*")
	if err != nil {
		return err
	}
	name := file.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.filename(linkID))
}

func (s *linkStore) delete(linkID string) error {
	if err := os.Remove(s.filename(linkID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (s *linkStore) filename(linkID string) string {
	sum := sha256.Sum256([]byte(linkID))
	return filepath.Join(s.root, hex.EncodeToString(sum[:])+".link")
}
