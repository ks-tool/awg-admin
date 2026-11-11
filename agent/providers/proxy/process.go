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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type coreProcess struct {
	cmd  *exec.Cmd
	done chan error
	hash [32]byte
}

type processManager struct {
	core       string
	root       string
	mu         sync.Mutex
	processes  map[string]*coreProcess
	startDelay time.Duration
}

func newProcessManager(core, root string) (*processManager, error) {
	if len(core) == 0 {
		return nil, errors.New("core binary path is required")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	return &processManager{core: core, root: root, processes: make(map[string]*coreProcess), startDelay: 200 * time.Millisecond}, nil
}

func (m *processManager) apply(resourceID string, config CoreConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash := sha256.Sum256(config.Data)
	if current := m.processes[resourceID]; current != nil && current.hash == hash {
		select {
		case <-current.done:
			delete(m.processes, resourceID)
		default:
			return nil
		}
	}
	if err := m.stopLocked(resourceID); err != nil {
		return err
	}
	dir := m.resourceDir(resourceID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	configPath := filepath.Join(dir, "config.json")
	if err := writePrivate(configPath, config.Data); err != nil {
		return err
	}
	logFile, err := os.OpenFile(filepath.Join(dir, "core.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	binary := m.core
	if len(config.Binary) > 0 {
		binary = config.Binary
	}
	cmd := exec.Command(binary, config.Arguments(configPath)...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start core: %w", err)
	}
	process := &coreProcess{cmd: cmd, done: make(chan error, 1), hash: hash}
	go func() {
		process.done <- cmd.Wait()
		_ = logFile.Close()
	}()
	select {
	case err := <-process.done:
		return fmt.Errorf("core exited during startup: %w; see %s", err, filepath.Join(dir, "core.log"))
	case <-time.After(m.startDelay):
	}
	m.processes[resourceID] = process
	return nil
}

func (m *processManager) stop(resourceID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stopLocked(resourceID)
}

func (m *processManager) stopLocked(resourceID string) error {
	process := m.processes[resourceID]
	if process == nil {
		return nil
	}
	delete(m.processes, resourceID)
	select {
	case <-process.done:
		return nil
	default:
	}
	if err := process.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		_ = process.cmd.Process.Kill()
		<-process.done
		return err
	}
	select {
	case <-process.done:
		return nil
	case <-time.After(5 * time.Second):
		if err := process.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		<-process.done
		return nil
	}
}

func (m *processManager) close() error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	for _, id := range ids {
		if err := m.stop(id); err != nil {
			return err
		}
	}
	return nil
}

func (m *processManager) resourceDir(resourceID string) string {
	sum := sha256.Sum256([]byte(resourceID))
	return filepath.Join(m.root, hex.EncodeToString(sum[:]))
}
