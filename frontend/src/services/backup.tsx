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

import { getCurrentApiMode } from './apiMode';
import { bindingsClient } from './bindingsClient';
import { reportError } from './errorReporting';
import { API_BASE_URL } from './api';

// Downloads a full backup of the admin database (same JSON dump format as
// `awg-migrate export`, restorable with `awg-migrate import`). Works in both
// modes: desktop opens a native save dialog in Go; http (web-server / browser)
// fetches GET /backup with the session cookie and triggers a browser download.
// Returns true on success, false on cancel or error (already reported).
export async function saveBackup(): Promise<boolean> {
  if (getCurrentApiMode() === 'bindings') {
    const { data, error } = await bindingsClient.saveBackup();
    if (error) {
      reportError('save-backup', 'Failed to save backup', error);
      return false;
    }
    return data;
  }

  try {
    const res = await fetch(`${API_BASE_URL}/backup`, { credentials: 'include' });
    if (!res.ok) {
      reportError('save-backup', 'Failed to save backup', `HTTP ${res.status}`);
      return false;
    }
    const blob = await res.blob();
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = 'awg-admin-backup.json';
    document.body.appendChild(a);
    a.click();
    a.remove();
    URL.revokeObjectURL(url);
    return true;
  } catch (error) {
    reportError('save-backup', 'Failed to save backup', String(error));
    return false;
  }
}
