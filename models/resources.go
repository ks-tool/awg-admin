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

import agentmodels "github.com/ks-tool/awg-admin/agent/models"

const ProtocolVersion = agentmodels.ProtocolVersion

const (
	ResourcePending  = agentmodels.ResourcePending
	ResourceReady    = agentmodels.ResourceReady
	ResourceDegraded = agentmodels.ResourceDegraded
	ResourceDeleting = agentmodels.ResourceDeleting

	ConditionTrue    = agentmodels.ConditionTrue
	ConditionFalse   = agentmodels.ConditionFalse
	ConditionUnknown = agentmodels.ConditionUnknown

	ActionResultJSON  = agentmodels.ActionResultJSON
	ActionResultFile  = agentmodels.ActionResultFile
	ActionResultAsync = agentmodels.ActionResultAsync

	LinkPending   = agentmodels.LinkPending
	LinkPreparing = agentmodels.LinkPreparing
	LinkReady     = agentmodels.LinkReady
	LinkDegraded  = agentmodels.LinkDegraded
)

type Resource = agentmodels.Resource
type ResourceDefinition = agentmodels.ResourceDefinition
type ResourceStatus = agentmodels.ResourceStatus
type ResourcePhase = agentmodels.ResourcePhase
type Condition = agentmodels.Condition
type ConditionStatus = agentmodels.ConditionStatus
type Manifest = agentmodels.Manifest
type ProviderDefinition = agentmodels.ProviderDefinition
type PortDefinition = agentmodels.PortDefinition
type ActionDefinition = agentmodels.ActionDefinition
type ActionResultType = agentmodels.ActionResultType
type ActionRequest = agentmodels.ActionRequest
type ActionResult = agentmodels.ActionResult
type Link = agentmodels.Link
type LinkEndpoint = agentmodels.LinkEndpoint
type LinkStatus = agentmodels.LinkStatus
type LinkPhase = agentmodels.LinkPhase
type LinkOffer = agentmodels.LinkOffer

type ProviderRecord struct {
	ServerID   string             `json:"serverId"`
	Definition ProviderDefinition `json:"definition"`
}
