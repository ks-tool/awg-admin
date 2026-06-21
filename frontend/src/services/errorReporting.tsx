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

import {toast} from 'sonner';

/**
 * Logs and surfaces a backend/agent read failure (list/get calls) to the
 * user via a toast with the actual error message — these previously only
 * went to console.error, so the UI silently showed empty data with no
 * indication anything had failed.
 *
 * id is kept stable per call site (e.g. 'fetch-servers') so a repeatedly
 * failing background poll (see useAutoRefresh) replaces one toast instead
 * of stacking a new one on every tick.
 */
export function reportError(id: string, message: string, error: string): void {
    console.error(message, error);
    toast.error(`${message}: ${error}`, {id});
}
