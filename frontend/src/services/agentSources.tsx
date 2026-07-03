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

import {get, post, remove} from './api';
import {getCurrentApiMode} from './apiMode';
import {bindingsClient} from './bindingsClient';
import {reportError} from './errorReporting';
import type {AgentSource} from '@/types';

function getClient() {
    return getCurrentApiMode() === 'bindings' ? bindingsClient : null;
}

/**
 * Opens a native file picker to choose the awg-agent binary on awg-admin's own
 * filesystem, for a Path-based agent source. Desktop (Wails) only — a browser
 * can't resolve a real filesystem path, so this returns null in http mode (the
 * caller hides the button). Returns the chosen absolute path, or null if the
 * dialog was cancelled or the picker is unavailable.
 */
export async function selectAgentFile(title: string): Promise<string | null> {
    const client = getClient();
    if (!client) return null;

    const {data, error} = await client.selectFile(title);
    if (error) {
        reportError('select-agent-file', 'Failed to open file picker', error);
        return null;
    }
    return data || null; // '' means the dialog was cancelled
}

/** Saved agent-binary deploy presets, shown in the "Deploy agent" modal. */
export async function listAgentSources(): Promise<AgentSource[] | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.listAgentSources();
        if (error) {
            reportError('fetch-agent-sources', 'Failed to fetch agent sources', error);
            return null;
        }
        return data as unknown as AgentSource[];
    }

    const {data, error} = await get<AgentSource[]>('/agent-sources/');
    if (error) {
        reportError('fetch-agent-sources', 'Failed to fetch agent sources', error);
        return null;
    }
    return data;
}

/**
 * Saves a new deploy preset. Exactly one of url/path/image should be non-empty:
 * url is fetched over the network (by the managed server itself, or by
 * awg-admin when cacheLocally is set), path reads the binary directly from
 * awg-admin's own filesystem, image is a Docker image run as a container on the
 * server (cacheLocally is ignored for path/image).
 */
export async function createAgentSource(
    name: string,
    url: string,
    path: string,
    image: string,
    cacheLocally: boolean,
    userspace: boolean,
): Promise<AgentSource | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.createAgentSource(name, url, path, image, cacheLocally, userspace);
        if (error) {
            console.error('Failed to create agent source (bindings):', error);
            return null;
        }
        return data as unknown as AgentSource;
    }

    const {data, error} = await post<AgentSource, {name: string; url: string; path: string; image: string; cacheLocally: boolean; userspace: boolean}>(
        '/agent-sources',
        {name, url, path, image, cacheLocally, userspace},
    );
    if (error) {
        console.error('Failed to create agent source:', error);
        return null;
    }
    return data;
}

export async function deleteAgentSource(id: string): Promise<boolean> {
    const client = getClient();

    if (client) {
        const {error} = await client.deleteAgentSource(id);
        if (error) {
            console.error(`Failed to delete agent source ${id} (bindings):`, error);
            return false;
        }
        return true;
    }

    const {error} = await remove<void>(`/agent-sources/${id}`);
    if (error) {
        console.error(`Failed to delete agent source ${id}:`, error);
        return false;
    }
    return true;
}

/** Re-downloads a CacheLocally source's binary, replacing the cached copy. */
export async function refreshAgentSourceCache(id: string): Promise<boolean> {
    const client = getClient();

    if (client) {
        const {error} = await client.refreshAgentSourceCache(id);
        if (error) {
            console.error(`Failed to refresh agent source cache ${id} (bindings):`, error);
            return false;
        }
        return true;
    }

    const {error} = await post<void, undefined>(`/agent-sources/${id}/refresh`, undefined);
    if (error) {
        console.error(`Failed to refresh agent source cache ${id}:`, error);
        return false;
    }
    return true;
}
