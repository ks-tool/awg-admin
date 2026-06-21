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

// DeployStatus reports the progress of an in-flight (or just-finished)
// Service.DeployAgent call for one server, polled by the frontend (see
// Service.GetDeployStatus) to show step-by-step progress instead of a
// plain spinner.
type DeployStatus struct {
	// Step is one of internal/deploy.ToAgent's step-name keys ("connect",
	// "upload_binary", "upload_unit", "upload_env", "upload_tls",
	// "start_service") — deliberately not human-readable text, so the
	// frontend localizes it itself rather than displaying whatever
	// language the backend happens to log in.
	Step string `json:"step"`
	Done bool   `json:"done"`
	// Error is the deploy's final error message, set only once Done is
	// true and the deploy failed.
	Error string `json:"error,omitempty"`
}
