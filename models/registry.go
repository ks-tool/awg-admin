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

import "uuid"

// Registry contains per-deployment OCI registry settings. SourceID selects a
// saved registry; inline credentials are supported for one-off operations.
type Registry struct {
	SourceID  string `json:"sourceId,omitempty"`
	Username  string `json:"username,omitempty"`
	Password  string `json:"password,omitempty"`
	PlainHTTP bool   `json:"plainHttp,omitempty"`
}

// RegistrySource is a reusable OCI registry and its artifact repositories.
// Password is stored in the admin database, but list and get responses clear
// it and expose HasPassword.
type RegistrySource struct {
	ID                 uuid.UUID `json:"id"`
	Name               string    `json:"name"`
	URL                string    `json:"url"`
	AgentRepository    string    `json:"agentRepository"`
	ProviderRepository string    `json:"providerRepository"`
	Username           string    `json:"username,omitempty"`
	Password           string    `json:"password,omitempty"`
	HasPassword        bool      `json:"hasPassword,omitempty"`
	PlainHTTP          bool      `json:"plainHttp,omitempty"`
}

// RegistryRequest identifies an untagged OCI repository or repository prefix.
type RegistryRequest struct {
	Repository string   `json:"repository"`
	Registry   Registry `json:"registry,omitempty"`
}

// AgentDeployment selects either an OCI release or a local vpn-agent binary.
type AgentDeployment struct {
	Artifact   string   `json:"artifact,omitempty"`
	BinaryPath string   `json:"binaryPath,omitempty"`
	Registry   Registry `json:"registry,omitempty"`
}

// ProviderRepository is a provider found under an OCI repository prefix.
type ProviderRepository struct {
	Name       string `json:"name"`
	Repository string `json:"repository"`
}
