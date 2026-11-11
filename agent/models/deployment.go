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

// ProviderDeployment selects a provider and its OCI binary release. The
// provider's own pack defines dependencies, values and runtime registration.
type ProviderDeployment struct {
	Name     string `json:"name"`
	Artifact string `json:"artifact"`
	// Options contains provider-specific installation settings interpreted by
	// that provider's pack, for example {"userspace": true} for AWG.
	Options map[string]any `json:"options,omitempty"`
}
