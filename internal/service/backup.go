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

package service

import "bytes"

// Backup returns a JSON snapshot of the entire admin database in the same
// bucket-dump format as `awg-migrate export`, so it can be restored with
// `awg-migrate import` (or by copying it to another machine). Returned as
// bytes rather than streamed to a writer so the method stays Wails-bindable
// for the desktop "Backup" button (App.SaveBackup); the database is small
// enough (a handful of servers/users/peers) to hold in memory.
//
// The dump contains secrets (SSH private keys, agent mTLS keys, peer PSKs,
// the admin bcrypt hash) — it's a full backup, so callers must treat the
// result as sensitive.
func (s *Service) Backup() ([]byte, error) {
	debugOp("Backup").Msg("creating database backup")
	var buf bytes.Buffer
	if err := s.store.Backup(&buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
