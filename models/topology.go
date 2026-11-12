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

package models

import (
	"time"
	"uuid"
)

// Topology is the protocol-neutral desired network configuration. VPN service
// configs are opaque to the core and interpreted only by their plugin.
type Topology struct {
	Servers     []TopologyServer    `json:"servers"`
	Interfaces  []NetworkInterface  `json:"interfaces"`
	Services    []VPNService        `json:"services"`
	Connections []NetworkConnection `json:"connections"`
}

type TopologyServer struct {
	ID                uuid.UUID `json:"id"`
	Name              string    `json:"name"`
	Address           string    `json:"address,omitempty"`
	Description       string    `json:"description,omitempty"`
	Location          string    `json:"location,omitempty"`
	LocationError     string    `json:"locationError,omitempty"`
	Legacy            bool      `json:"legacy,omitempty"`
	Latitude          *float64  `json:"latitude,omitempty"`
	Longitude         *float64  `json:"longitude,omitempty"`
	CoordinatesManual bool      `json:"coordinatesManual,omitempty"`
	X                 float64   `json:"x"`
	Y                 float64   `json:"y"`
}

type NetworkInterface struct {
	ID        uuid.UUID  `json:"id"`
	ServerID  uuid.UUID  `json:"serverId"`
	Name      string     `json:"name"`
	Address   string     `json:"address,omitempty"`
	Legacy    bool       `json:"legacy,omitempty"`
	ServiceID *uuid.UUID `json:"serviceId,omitempty"`
	X         float64    `json:"x"`
	Y         float64    `json:"y"`
}

type VPNService struct {
	ID            uuid.UUID      `json:"id"`
	ServerID      uuid.UUID      `json:"serverId"`
	InterfaceID   *uuid.UUID     `json:"interfaceId,omitempty"`
	PluginID      string         `json:"pluginId"`
	Name          string         `json:"name"`
	Config        map[string]any `json:"config"`
	Status        string         `json:"status"`
	StatusMessage string         `json:"statusMessage,omitempty"`
	LastAppliedAt *time.Time     `json:"lastAppliedAt,omitempty"`
}

type NetworkConnection struct {
	ID       uuid.UUID `json:"id"`
	SourceID uuid.UUID `json:"sourceId"`
	TargetID uuid.UUID `json:"targetId"`
}

type NodePosition struct {
	ID uuid.UUID `json:"id"`
	X  float64   `json:"x"`
	Y  float64   `json:"y"`
}

// PluginDescriptor describes a deployment plugin and the schemas consumed by
// RJSF. ConfigSchema and UserSchema are deliberately independent: one describes
// the application configuration, the other a single service user.
type PluginDescriptor struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Description  string         `json:"description,omitempty"`
	ConfigSchema map[string]any `json:"configSchema"`
	UserSchema   map[string]any `json:"userSchema"`
	UISchema     map[string]any `json:"uiSchema,omitempty"`
	Available    bool           `json:"available"`
	Error        string         `json:"error,omitempty"`
}

type PluginStatus struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type PluginUser struct {
	ID   string         `json:"id"`
	Data map[string]any `json:"data"`
}
