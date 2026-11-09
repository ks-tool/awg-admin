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

package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ks-tool/vpn-admin/models"
)

const ProtocolVersion = 1

type Request struct {
	Version    int            `json:"version"`
	Action     string         `json:"action"`
	Deployment Deployment     `json:"deployment"`
	UserID     string         `json:"userId,omitempty"`
	User       map[string]any `json:"user,omitempty"`
}

type Response struct {
	Descriptor *models.PluginDescriptor `json:"descriptor,omitempty"`
	Status     *models.PluginStatus     `json:"status,omitempty"`
	Users      []models.PluginUser      `json:"users,omitempty"`
	User       *models.PluginUser       `json:"user,omitempty"`
	Error      string                   `json:"error,omitempty"`
}

// ProcessPlugin invokes an external executable once per operation. Each call
// receives one JSON request on stdin and returns one JSON response on stdout.
// This keeps plugins language-neutral and avoids Go ABI/plugin-loader limits.
type ProcessPlugin struct {
	id   string
	path string
}

func NewProcessPlugin(id, path string) *ProcessPlugin {
	return &ProcessPlugin{id: id, path: path}
}

func (p *ProcessPlugin) call(ctx context.Context, action string, deployment Deployment, userID string, user map[string]any) (Response, error) {
	request, err := json.Marshal(Request{Version: ProtocolVersion, Action: action, Deployment: deployment, UserID: userID, User: user})
	if err != nil {
		return Response{}, err
	}
	command := exec.CommandContext(ctx, p.path)
	command.Stdin = bytes.NewReader(request)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		if stderr.Len() > 0 {
			return Response{}, fmt.Errorf("plugin %s: %w: %s", p.id, err, stderr.String())
		}
		return Response{}, fmt.Errorf("plugin %s: %w", p.id, err)
	}
	var response Response
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return Response{}, fmt.Errorf("plugin %s returned invalid JSON: %w", p.id, err)
	}
	if response.Error != "" {
		return Response{}, fmt.Errorf("plugin %s: %s", p.id, response.Error)
	}
	return response, nil
}

func (p *ProcessPlugin) Describe(ctx context.Context) (models.PluginDescriptor, error) {
	response, err := p.call(ctx, "describe", Deployment{}, "", nil)
	if err != nil {
		return models.PluginDescriptor{}, err
	}
	if response.Descriptor == nil {
		return models.PluginDescriptor{}, fmt.Errorf("plugin %s returned no descriptor", p.id)
	}
	descriptor := *response.Descriptor
	if descriptor.ID != p.id {
		return models.PluginDescriptor{}, fmt.Errorf("plugin id mismatch: manifest %q, executable %q", p.id, descriptor.ID)
	}
	return descriptor, nil
}

func (p *ProcessPlugin) Apply(ctx context.Context, deployment Deployment) (models.PluginStatus, error) {
	response, err := p.call(ctx, "apply", deployment, "", nil)
	if err != nil {
		return models.PluginStatus{}, err
	}
	if response.Status == nil {
		return models.PluginStatus{}, fmt.Errorf("plugin %s returned no status", p.id)
	}
	return *response.Status, nil
}

func (p *ProcessPlugin) Delete(ctx context.Context, deployment Deployment) error {
	_, err := p.call(ctx, "delete", deployment, "", nil)
	return err
}

func (p *ProcessPlugin) Status(ctx context.Context, deployment Deployment) (models.PluginStatus, error) {
	response, err := p.call(ctx, "status", deployment, "", nil)
	if err != nil {
		return models.PluginStatus{}, err
	}
	if response.Status == nil {
		return models.PluginStatus{}, fmt.Errorf("plugin %s returned no status", p.id)
	}
	return *response.Status, nil
}

func (p *ProcessPlugin) ListUsers(ctx context.Context, deployment Deployment) ([]models.PluginUser, error) {
	response, err := p.call(ctx, "users.list", deployment, "", nil)
	if err != nil {
		return nil, err
	}
	if response.Users == nil {
		response.Users = make([]models.PluginUser, 0)
	}
	return response.Users, nil
}

func (p *ProcessPlugin) CreateUser(ctx context.Context, deployment Deployment, user map[string]any) (models.PluginUser, error) {
	response, err := p.call(ctx, "users.create", deployment, "", user)
	if err != nil {
		return models.PluginUser{}, err
	}
	if response.User == nil {
		return models.PluginUser{}, fmt.Errorf("plugin %s returned no user", p.id)
	}
	return *response.User, nil
}

func (p *ProcessPlugin) UpdateUser(ctx context.Context, deployment Deployment, id string, user map[string]any) (models.PluginUser, error) {
	response, err := p.call(ctx, "users.update", deployment, id, user)
	if err != nil {
		return models.PluginUser{}, err
	}
	if response.User == nil {
		return models.PluginUser{}, fmt.Errorf("plugin %s returned no user", p.id)
	}
	return *response.User, nil
}

func (p *ProcessPlugin) DeleteUser(ctx context.Context, deployment Deployment, id string) error {
	_, err := p.call(ctx, "users.delete", deployment, id, nil)
	return err
}

type manifest struct {
	ID         string `json:"id"`
	Executable string `json:"executable"`
}

// LoadDir registers every plugin.json manifest directly inside dir. A missing
// directory is valid and simply means no external plugins are installed.
func (r *Registry) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read plugin manifest %s: %w", path, err)
		}
		var item manifest
		if err := json.Unmarshal(data, &item); err != nil {
			return fmt.Errorf("decode plugin manifest %s: %w", path, err)
		}
		if item.ID == "" || item.Executable == "" {
			return fmt.Errorf("plugin manifest %s requires id and executable", path)
		}
		executable := item.Executable
		if !filepath.IsAbs(executable) {
			executable = filepath.Join(dir, executable)
		}
		r.mu.Lock()
		if _, exists := r.plugins[item.ID]; exists {
			r.mu.Unlock()
			return fmt.Errorf("plugin %q is already registered", item.ID)
		}
		r.plugins[item.ID] = NewProcessPlugin(item.ID, executable)
		r.mu.Unlock()
	}
	return nil
}
