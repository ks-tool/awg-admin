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

import { get, post, remove } from './api';
import { getCurrentApiMode } from './apiMode';
import { bindingsClient } from './bindingsClient';
import { reportError } from './errorReporting';
import type { Tunnel, TunnelStep } from '@/types';

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

/**
 * Builds a tunnel from the given ordered interfaces (entry first, exit last).
 * subnet is the shared tunnel subnet; empty means auto. Returns the tunnel, or
 * an error string (surfaced to the user — BuildTunnel validates the selection).
 */
export async function buildTunnel(
  steps: TunnelStep[],
  subnet: string,
): Promise<{ tunnel: Tunnel | null; error?: string }> {
  const client = getClient();
  if (client) {
    const { data, error } = await client.buildTunnel(steps, subnet);
    if (error) return { tunnel: null, error };
    return { tunnel: data as unknown as Tunnel };
  }

  const { data, error } = await post<Tunnel, { steps: TunnelStep[]; subnet: string }>('/tunnels', { steps, subnet });
  if (error) return { tunnel: null, error };
  return { tunnel: data ?? null };
}

/** Removes a tunnel, leaving its interfaces empty. */
export async function removeTunnel(id: string): Promise<{ ok: boolean; error?: string }> {
  const client = getClient();
  if (client) {
    const { error } = await client.removeTunnel(id);
    return { ok: !error, error };
  }

  const { error } = await remove<void>(`/tunnels/${id}`);
  return { ok: !error, error };
}
