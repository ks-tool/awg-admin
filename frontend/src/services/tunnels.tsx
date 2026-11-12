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

import { get } from './api';
import { getCurrentApiMode } from './apiMode';
import { bindingsClient } from './bindingsClient';
import { reportError } from './errorReporting';
import type { Tunnel } from '@/types';

function getClient() {
  return getCurrentApiMode() === 'bindings' ? bindingsClient : null;
}

/** Every configured multi-hop tunnel (a group of interfaces sharing an id). */
export async function listTunnels(): Promise<Tunnel[] | null> {
  const client = getClient();
  if (client) {
    const { data, error } = await client.listTunnels();
    if (error) {
      reportError('fetch-tunnels', 'Failed to fetch tunnels', error);
      return null;
    }
    return data as unknown as Tunnel[];
  }

  const { data, error } = await get<Tunnel[]>('/tunnels/');
  if (error) {
    reportError('fetch-tunnels', 'Failed to fetch tunnels', error);
    return null;
  }
  return data ?? null;
}
