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

	"github.com/rs/zerolog/log"
	"github.com/uptrace/bunrouter"
)

// backupDownload streams a full snapshot of the admin database as a JSON file
// download (same dump format as `awg-migrate export`, restorable with
// `awg-migrate import`). Behind requireAuth — the dump contains SSH keys,
// PSKs and the admin password hash.
func (h *Handler) backupDownload(w http.ResponseWriter, r bunrouter.Request) error {
	fields := map[string]any{"method": r.Method, "path": r.URL.Path}
	log.Debug().Fields(fields).Msg("backing up admin database")

	data, err := h.svc.Backup()
	if err != nil {
		return handleErr(err, fields)
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", `attachment; filename="awg-admin-backup.json"`)
	_, err = w.Write(data)
	return err
}
