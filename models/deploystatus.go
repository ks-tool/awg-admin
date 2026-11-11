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

// DeployStatus reports an agent or provider deployment for one server, polled
// by the frontend through Service.GetDeployStatus.
type DeployStatus struct {
	// Step is a machine-readable deployment key such as "install_agent" or
	// "install_provider_awg"; the frontend localizes it.
	Step string `json:"step"`
	Done bool   `json:"done"`
	// Error is the deploy's final error message, set only once Done is
	// true and the deploy failed.
	Error string `json:"error,omitempty"`
}
