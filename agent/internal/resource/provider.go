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

package resource

import (
	"context"
	"encoding/json"

	"github.com/ks-tool/awg-admin/agent/models"
)

type Provider interface {
	Name() string
	Version() string
	Definitions() []models.ResourceDefinition
	Validate(context.Context, models.Resource) error
	Reconcile(context.Context, models.Resource) (json.RawMessage, error)
	Delete(context.Context, models.Resource) error
}

type ActionProvider interface {
	Action(context.Context, models.Resource, string, json.RawMessage) (models.ActionResult, error)
}

type Event struct {
	Type       string                `json:"type"`
	Provider   string                `json:"provider"`
	APIVersion string                `json:"apiVersion"`
	Kind       string                `json:"kind"`
	Name       string                `json:"name"`
	Generation int64                 `json:"generation"`
	Status     models.ResourceStatus `json:"status"`
}
