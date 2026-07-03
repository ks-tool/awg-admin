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

package api

import (
	"net/http"

	"github.com/uptrace/bunrouter"
)

// appVersion reports the admin app's build version, shown on the Settings page.
// The desktop app reads the same value via the App.AppVersion Wails binding.
func (h *Handler) appVersion(w http.ResponseWriter, _ bunrouter.Request) error {
	return bunrouter.JSON(w, map[string]any{"version": h.svc.AppVersion()})
}
