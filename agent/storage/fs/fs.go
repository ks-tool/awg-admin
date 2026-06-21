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

package fs

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"

	agentmodels "github.com/ks-tool/awg-admin/agent/models"
	"github.com/ks-tool/awg-admin/agent/storage"

	"github.com/natefinch/atomic"
)

const ext = ".json"

type Dir struct {
	root string
}

func New(root string) (*Dir, error) {
	if err := os.MkdirAll(root, 0755); err != nil {
		return nil, err
	}
	return &Dir{root: root}, nil
}

func (dir *Dir) Close() error {
	return nil
}

func (dir *Dir) List() ([]agentmodels.InterfaceConfig, error) {
	list := make([]agentmodels.InterfaceConfig, 0)
	err := filepath.Walk(dir.root, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(info.Name()) != ext {
			return nil
		}

		cfg, err := dir.get(path)
		if err != nil {
			return err
		}
		list = append(list, *cfg)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return list, nil
}

func (dir *Dir) Set(cfg *agentmodels.InterfaceConfig) error {
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}

	return atomic.WriteFile(filepath.Join(dir.root, cfg.Interface)+ext, &buf)
}

func (dir *Dir) Get(iface string) (*agentmodels.InterfaceConfig, error) {
	filename := filepath.Join(dir.root, iface) + ext
	return dir.get(filename)
}

func (dir *Dir) get(filename string) (*agentmodels.InterfaceConfig, error) {
	fi, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, storage.ErrNotFound
		}
		return nil, err
	}
	defer func() { _ = fi.Close() }()

	var cfg agentmodels.InterfaceConfig
	if err = json.NewDecoder(fi).Decode(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (dir *Dir) Delete(iface string) error {
	return os.Remove(filepath.Join(dir.root, iface) + ext)
}
