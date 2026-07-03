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

import {get} from './api';
import {getCurrentApiMode} from './apiMode';
import {bindingsClient} from './bindingsClient';

/**
 * The admin app's build version (e.g. "v1.0.0", or "dev" for a local build),
 * shown on the Settings page. Desktop reads it via the App.AppVersion Wails
 * binding, the web server via GET /version. Returns null on failure so the UI
 * can just hide the line rather than surfacing an error.
 */
export async function getAppVersion(): Promise<string | null> {
    if (getCurrentApiMode() === 'bindings') {
        const {data, error} = await bindingsClient.appVersion();
        if (error) return null;
        return data;
    }

    const {data, error} = await get<{version: string}>('/version');
    if (error || !data) return null;
    return data.version;
}
