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

// Package plugin defines the protocol-neutral VPN application contract. The
// core owns topology and desired configuration; plugins own deployment,
// runtime state and service users.
package plugin

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/ks-tool/awg-admin/models"
)

type Deployment struct {
	Service     models.VPNService          `json:"service"`
	Server      models.TopologyServer      `json:"server"`
	Interface   *models.NetworkInterface   `json:"interface,omitempty"`
	Connections []models.NetworkConnection `json:"connections"`
}

type Plugin interface {
	Describe(context.Context) (models.PluginDescriptor, error)
	Apply(context.Context, Deployment) (models.PluginStatus, error)
	Delete(context.Context, Deployment) error
	Status(context.Context, Deployment) (models.PluginStatus, error)
	ListUsers(context.Context, Deployment) ([]models.PluginUser, error)
	CreateUser(context.Context, Deployment, map[string]any) (models.PluginUser, error)
	UpdateUser(context.Context, Deployment, string, map[string]any) (models.PluginUser, error)
	DeleteUser(context.Context, Deployment, string) error
}

type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
}

func NewRegistry() *Registry {
	return &Registry{plugins: make(map[string]Plugin)}
}

func (r *Registry) Register(p Plugin) error {
	descriptor, err := p.Describe(context.Background())
	if err != nil {
		return fmt.Errorf("describe plugin: %w", err)
	}
	if descriptor.ID == "" {
		return fmt.Errorf("plugin id is required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.plugins[descriptor.ID]; exists {
		return fmt.Errorf("plugin %q is already registered", descriptor.ID)
	}
	r.plugins[descriptor.ID] = p
	return nil
}

func (r *Registry) Get(id string) (Plugin, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.plugins[id]
	if !ok {
		return nil, fmt.Errorf("VPN plugin %q is not installed", id)
	}
	return p, nil
}

func (r *Registry) List(ctx context.Context) []models.PluginDescriptor {
	r.mu.RLock()
	plugins := make(map[string]Plugin, len(r.plugins))
	for id, p := range r.plugins {
		plugins[id] = p
	}
	r.mu.RUnlock()

	result := make([]models.PluginDescriptor, 0, len(plugins))
	for id, p := range plugins {
		descriptor, err := p.Describe(ctx)
		if err != nil {
			descriptor = models.PluginDescriptor{ID: id, Name: id, Available: false, Error: err.Error()}
		} else {
			descriptor.Available = true
		}
		result = append(result, descriptor)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}
