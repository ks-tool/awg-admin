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

// Log capture is a desktop-only feature: the buffer only exists in the Wails
// process (see App.NewApp / internal/logbuffer). In http (web-server) mode
// there is no equivalent endpoint, so these resolve to empty/false and the
// Settings UI hides the whole section.
function bindings() {
  return getCurrentApiMode() === 'bindings' ? bindingsClient : null;
}

/**
 * Fetch the captured stdout logs as NDJSON text (one zerolog JSON object per
 * line, oldest first) for display in the Logs modal.
 */
export async function getLogs(): Promise<string> {
  const client = bindings();
  if (!client) return '';

  const { data, error } = await client.getLogs();
  if (error) {
    reportError('fetch-logs', 'Failed to fetch logs', error);
    return '';
  }
  return data;
}

/**
 * Save the captured logs to a JSON file via the OS-native save dialog (handled
 * in Go). Returns true if a file was written, false if the user cancelled the
 * dialog or an error was reported.
 */
export async function saveLogs(): Promise<boolean> {
  const client = bindings();
  if (!client) return false;

  const { data, error } = await client.saveLogs();
  if (error) {
    reportError('save-logs', 'Failed to save logs', error);
    return false;
  }
  return data;
}
