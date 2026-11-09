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

// Package awg adapts the existing AWG agent to the generic plugin contract.
// Protocol-specific fields and peer mapping live here, not in the topology core.
package awg

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	agentmodels "github.com/ks-tool/vpn-admin/agent/models"
	"github.com/ks-tool/vpn-admin/internal/agentclient"
	"github.com/ks-tool/vpn-admin/internal/plugin"
	"github.com/ks-tool/vpn-admin/models"
)

type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (p *Plugin) Describe(context.Context) (models.PluginDescriptor, error) {
	return models.PluginDescriptor{
		ID:          "vpn-agent",
		Name:        "AmneziaWG / WireGuard",
		Version:     "1.0.0",
		Description: "Manages an interface through an existing vpn-agent HTTP API.",
		ConfigSchema: map[string]any{
			"type":     "object",
			"required": []string{"agentUrl", "interface", "privateKey", "address", "listenPort"},
			"properties": map[string]any{
				"agentUrl":   map[string]any{"type": "string", "title": "Agent URL", "default": "http://127.0.0.1:8081"},
				"interface":  map[string]any{"type": "string", "title": "Interface name", "default": "awg0"},
				"privateKey": map[string]any{"type": "string", "title": "Private key"},
				"address":    map[string]any{"type": "string", "title": "Interface CIDR", "default": "10.0.0.1/24"},
				"listenPort": map[string]any{"type": "integer", "title": "Listen port", "minimum": 1, "maximum": 65535, "default": 51820},
				"mtu":        map[string]any{"type": "integer", "title": "MTU", "minimum": 576, "maximum": 9000, "default": 1420},
				"amnezia":    map[string]any{"type": "boolean", "title": "Enable AmneziaWG obfuscation", "default": true},
			},
		},
		UserSchema: map[string]any{
			"type":     "object",
			"required": []string{"allowedIPs"},
			"properties": map[string]any{
				"privateKey": map[string]any{"type": "string", "title": "Private key (empty to generate)"},
				"allowedIPs": map[string]any{"type": "array", "title": "Allowed IPs", "items": map[string]any{"type": "string"}, "minItems": 1},
				"endpoint":   map[string]any{"type": "string", "title": "Endpoint (optional)"},
				"keepalive":  map[string]any{"type": "integer", "title": "Keepalive, seconds", "minimum": 0, "maximum": 3600, "default": 25},
				"disabled":   map[string]any{"type": "boolean", "title": "Disabled", "default": false},
			},
		},
		UISchema: map[string]any{"privateKey": map[string]any{"ui:widget": "password"}},
	}, nil
}

type config struct {
	AgentURL   string `json:"agentUrl"`
	Interface  string `json:"interface"`
	PrivateKey string `json:"privateKey"`
	Address    string `json:"address"`
	ListenPort uint16 `json:"listenPort"`
	MTU        int    `json:"mtu"`
	Amnezia    bool   `json:"amnezia"`
}

type userConfig struct {
	PrivateKey string   `json:"privateKey"`
	AllowedIPs []string `json:"allowedIPs"`
	Endpoint   string   `json:"endpoint"`
	Keepalive  int      `json:"keepalive"`
	Disabled   bool     `json:"disabled"`
}

func decode[T any](value map[string]any) (T, error) {
	var result T
	data, err := json.Marshal(value)
	if err != nil {
		return result, err
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("invalid plugin configuration: %w", err)
	}
	return result, nil
}

func deploymentConfig(deployment plugin.Deployment) (config, error) {
	cfg, err := decode[config](deployment.Service.Config)
	if err != nil {
		return cfg, err
	}
	parsed, err := url.Parse(cfg.AgentURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return cfg, fmt.Errorf("agentUrl must be an absolute HTTP(S) URL")
	}
	if strings.TrimSpace(cfg.Interface) == "" {
		return cfg, fmt.Errorf("interface is required")
	}
	return cfg, nil
}

func clientFor(cfg config) *agentclient.Client {
	return agentclient.New(&http.Client{Timeout: 15 * time.Second}, cfg.AgentURL)
}

func interfaceConfig(cfg config) (agentmodels.InterfaceConfig, error) {
	result := agentmodels.InterfaceConfig{Interface: cfg.Interface, Address: cfg.Address, ListenPort: cfg.ListenPort, MTU: cfg.MTU}
	var err error
	if cfg.PrivateKey == "" {
		result.PrivateKey, err = agentmodels.GeneratePrivateKey()
	} else {
		result.PrivateKey, err = agentmodels.ParseKey(cfg.PrivateKey)
	}
	if err != nil {
		return result, fmt.Errorf("private key: %w", err)
	}
	if cfg.Amnezia {
		agentmodels.GenerateAmneziaParams(&result)
	}
	return result, nil
}

func (p *Plugin) Apply(ctx context.Context, deployment plugin.Deployment) (models.PluginStatus, error) {
	cfg, err := deploymentConfig(deployment)
	if err != nil {
		return models.PluginStatus{}, err
	}
	iface, err := interfaceConfig(cfg)
	if err != nil {
		return models.PluginStatus{}, err
	}
	client := clientFor(cfg)
	if current, getErr := client.Get(ctx, cfg.Interface); getErr == nil {
		iface.Peers = current.Peers
	}
	if err := client.Set(ctx, iface); err != nil {
		return models.PluginStatus{}, err
	}
	return models.PluginStatus{State: "running", Message: "Configuration applied to vpn-agent"}, nil
}

func (p *Plugin) Delete(ctx context.Context, deployment plugin.Deployment) error {
	cfg, err := deploymentConfig(deployment)
	if err != nil {
		return err
	}
	return clientFor(cfg).Delete(ctx, cfg.Interface)
}

func (p *Plugin) Status(ctx context.Context, deployment plugin.Deployment) (models.PluginStatus, error) {
	cfg, err := deploymentConfig(deployment)
	if err != nil {
		return models.PluginStatus{}, err
	}
	if _, err := clientFor(cfg).Get(ctx, cfg.Interface); err != nil {
		return models.PluginStatus{State: "error"}, err
	}
	return models.PluginStatus{State: "running"}, nil
}

func peerUser(peer agentmodels.InterfacePeer) models.PluginUser {
	return models.PluginUser{ID: peer.Key.String(), Data: map[string]any{
		"publicKey":  peer.Key.String(),
		"allowedIPs": peer.AllowedIPs,
		"endpoint":   peer.Endpoint,
		"keepalive":  int(peer.KeepaliveInterval / time.Second),
		"disabled":   peer.Disabled,
	}}
}

func (p *Plugin) ListUsers(ctx context.Context, deployment plugin.Deployment) ([]models.PluginUser, error) {
	cfg, err := deploymentConfig(deployment)
	if err != nil {
		return nil, err
	}
	iface, err := clientFor(cfg).Get(ctx, cfg.Interface)
	if err != nil {
		return nil, err
	}
	users := make([]models.PluginUser, 0, len(iface.Peers))
	for _, peer := range iface.Peers {
		users = append(users, peerUser(peer))
	}
	return users, nil
}

func decodePeer(input map[string]any) (agentmodels.InterfacePeer, string, error) {
	cfg, err := decode[userConfig](input)
	if err != nil {
		return agentmodels.InterfacePeer{}, "", err
	}
	if len(cfg.AllowedIPs) == 0 {
		return agentmodels.InterfacePeer{}, "", fmt.Errorf("at least one allowed IP is required")
	}
	var privateKey agentmodels.Key
	if cfg.PrivateKey == "" {
		privateKey, err = agentmodels.GeneratePrivateKey()
	} else {
		privateKey, err = agentmodels.ParseKey(cfg.PrivateKey)
	}
	if err != nil {
		return agentmodels.InterfacePeer{}, "", fmt.Errorf("private key: %w", err)
	}
	return agentmodels.InterfacePeer{
		Key:               privateKey.PublicKey(),
		AllowedIPs:        cfg.AllowedIPs,
		Endpoint:          cfg.Endpoint,
		KeepaliveInterval: time.Duration(cfg.Keepalive) * time.Second,
		Disabled:          cfg.Disabled,
	}, privateKey.String(), nil
}

func (p *Plugin) CreateUser(ctx context.Context, deployment plugin.Deployment, input map[string]any) (models.PluginUser, error) {
	cfg, err := deploymentConfig(deployment)
	if err != nil {
		return models.PluginUser{}, err
	}
	peer, privateKey, err := decodePeer(input)
	if err != nil {
		return models.PluginUser{}, err
	}
	client := clientFor(cfg)
	iface, err := client.Get(ctx, cfg.Interface)
	if err != nil {
		return models.PluginUser{}, err
	}
	for _, existing := range iface.Peers {
		if existing.Key == peer.Key {
			return models.PluginUser{}, fmt.Errorf("user with this public key already exists")
		}
	}
	iface.Peers = append(iface.Peers, peer)
	if err := client.Set(ctx, *iface); err != nil {
		return models.PluginUser{}, err
	}
	result := peerUser(peer)
	result.Data["privateKey"] = privateKey
	return result, nil
}

func (p *Plugin) UpdateUser(ctx context.Context, deployment plugin.Deployment, id string, input map[string]any) (models.PluginUser, error) {
	cfg, err := deploymentConfig(deployment)
	if err != nil {
		return models.PluginUser{}, err
	}
	updated, _, err := decodePeer(input)
	if err != nil {
		return models.PluginUser{}, err
	}
	client := clientFor(cfg)
	iface, err := client.Get(ctx, cfg.Interface)
	if err != nil {
		return models.PluginUser{}, err
	}
	found := false
	for index := range iface.Peers {
		if iface.Peers[index].Key.String() != id {
			continue
		}
		if _, hasPrivateKey := input["privateKey"]; !hasPrivateKey || input["privateKey"] == "" {
			updated.Key = iface.Peers[index].Key
		}
		iface.Peers[index] = updated
		found = true
		break
	}
	if !found {
		return models.PluginUser{}, fmt.Errorf("user %q not found", id)
	}
	if err := client.Set(ctx, *iface); err != nil {
		return models.PluginUser{}, err
	}
	return peerUser(updated), nil
}

func (p *Plugin) DeleteUser(ctx context.Context, deployment plugin.Deployment, id string) error {
	cfg, err := deploymentConfig(deployment)
	if err != nil {
		return err
	}
	client := clientFor(cfg)
	iface, err := client.Get(ctx, cfg.Interface)
	if err != nil {
		return err
	}
	peers := make([]agentmodels.InterfacePeer, 0, len(iface.Peers))
	found := false
	for _, peer := range iface.Peers {
		if peer.Key.String() == id {
			found = true
			continue
		}
		peers = append(peers, peer)
	}
	if !found {
		return fmt.Errorf("user %q not found", id)
	}
	iface.Peers = peers
	return client.Set(ctx, *iface)
}
