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
	"encoding/json"
	"time"
)

const ProtocolVersion = "v1"

type Resource struct {
	ID         string          `json:"id"`
	ServerID   string          `json:"serverId"`
	APIVersion string          `json:"apiVersion"`
	Kind       string          `json:"kind"`
	Name       string          `json:"name"`
	Generation int64           `json:"generation"`
	Spec       json.RawMessage `json:"spec"`
	Status     ResourceStatus  `json:"status"`
}

type ResourceDefinition struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	SpecSchema json.RawMessage    `json:"specSchema"`
	UISchema   json.RawMessage    `json:"uiSchema,omitempty"`
	SchemaHash string             `json:"schemaHash"`
	Ports      []PortDefinition   `json:"ports,omitempty"`
	Actions    []ActionDefinition `json:"actions,omitempty"`
}

type ResourceStatus struct {
	ObservedGeneration int64           `json:"observedGeneration"`
	Phase              ResourcePhase   `json:"phase"`
	Conditions         []Condition     `json:"conditions,omitempty"`
	Provider           json.RawMessage `json:"provider,omitempty"`
}

type ResourcePhase string

const (
	ResourcePending  ResourcePhase = "Pending"
	ResourceReady    ResourcePhase = "Ready"
	ResourceDegraded ResourcePhase = "Degraded"
	ResourceDeleting ResourcePhase = "Deleting"
)

type Condition struct {
	Type               string          `json:"type"`
	Status             ConditionStatus `json:"status"`
	Reason             string          `json:"reason"`
	Message            string          `json:"message"`
	ObservedGeneration int64           `json:"observedGeneration,omitempty"`
	LastTransitionTime time.Time       `json:"lastTransitionTime,omitempty"`
}

type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)

type Manifest struct {
	ProtocolVersion string               `json:"protocolVersion"`
	AgentID         string               `json:"agentId"`
	Providers       []ProviderDefinition `json:"providers"`
}

type ProviderDefinition struct {
	Name      string               `json:"name"`
	Version   string               `json:"version"`
	Resources []ResourceDefinition `json:"resources"`
}

type PortDefinition struct {
	Name        string   `json:"name"`
	Title       string   `json:"title,omitempty"`
	Direction   string   `json:"direction,omitempty"`
	LinkKinds   []string `json:"linkKinds,omitempty"`
	RemotePorts []string `json:"remotePorts,omitempty"`
}

type ActionDefinition struct {
	Name                 string           `json:"name"`
	Title                string           `json:"title"`
	Description          string           `json:"description,omitempty"`
	ArgumentsSchema      json.RawMessage  `json:"argumentsSchema,omitempty"`
	UISchema             json.RawMessage  `json:"uiSchema,omitempty"`
	ResultType           ActionResultType `json:"resultType"`
	TimeoutSeconds       int              `json:"timeoutSeconds,omitempty"`
	Mutating             bool             `json:"mutating,omitempty"`
	RequiresConfirmation bool             `json:"requiresConfirmation,omitempty"`
}

type ActionResultType string

const (
	ActionResultJSON  ActionResultType = "json"
	ActionResultFile  ActionResultType = "file"
	ActionResultAsync ActionResultType = "async"
)

type ActionRequest struct {
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type ActionResult struct {
	Type      ActionResultType `json:"type"`
	Data      json.RawMessage  `json:"data,omitempty"`
	FileName  string           `json:"fileName,omitempty"`
	MediaType string           `json:"mediaType,omitempty"`
	TaskID    string           `json:"taskId,omitempty"`
}

type Link struct {
	ID         string          `json:"id"`
	Name       string          `json:"name,omitempty"`
	Kind       string          `json:"kind"`
	Generation int64           `json:"generation"`
	Source     LinkEndpoint    `json:"source"`
	Target     LinkEndpoint    `json:"target"`
	Spec       json.RawMessage `json:"spec,omitempty"`
	Status     LinkStatus      `json:"status"`
}

type LinkEndpoint struct {
	ServerID   string `json:"serverId"`
	ResourceID string `json:"resourceId"`
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Port       string `json:"port"`
}

type LinkStatus struct {
	ObservedGeneration int64       `json:"observedGeneration"`
	Phase              LinkPhase   `json:"phase"`
	Conditions         []Condition `json:"conditions,omitempty"`
}

type LinkPhase string

const (
	LinkPending   LinkPhase = "Pending"
	LinkPreparing LinkPhase = "Preparing"
	LinkReady     LinkPhase = "Ready"
	LinkDegraded  LinkPhase = "Degraded"
)

type LinkOffer struct {
	LinkID   string          `json:"linkId"`
	Endpoint LinkEndpoint    `json:"endpoint"`
	Data     json.RawMessage `json:"data"`
}
