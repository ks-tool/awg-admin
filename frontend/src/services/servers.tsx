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

import * as AppBindings from '../../wailsjs/go/main/App';
import {get, post, put} from './api';
import {getCurrentApiMode} from './apiMode';
import {bindingsClient} from './bindingsClient';
import {throwIfPassphraseRequired} from './sshErrors';
import {reportError} from './errorReporting';
import type {Agent, ImportPreview, Interface, InterfaceHistory, Server, ServerInfo, SSHConfig, SystemHistory, UnmanagedInterface} from '@/types';

export interface ServerInput {
    name: string;
    info?: ServerInfo;
    ssh: SSHConfig;
    agent: Agent;
}

/**
 * Get the appropriate client based on current API mode
 */
function getClient() {
    return getCurrentApiMode() === 'bindings' ? bindingsClient : null;
}

/**
 * Get all servers
 */
export async function listServers(): Promise<Server[] | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.listServers();
        if (error) {
            reportError('fetch-servers', 'Failed to fetch servers', error);
            return null;
        }
        return data as unknown as Server[];
    }

    const {data, error} = await get<Server[]>('/servers/');
    if (error) {
        reportError('fetch-servers', 'Failed to fetch servers', error);
        return null;
    }
    return data;
}

export async function createServer(input: ServerInput): Promise<Server | null> {
    if (getCurrentApiMode() === 'bindings') {
        try {
            return await AppBindings.CreateServer(input as never) as unknown as Server;
        } catch (error) {
            const message = error instanceof Error ? error.message : String(error);
            throwIfPassphraseRequired(message);
            throw new Error(message);
        }
    }
    const {data, error} = await post<Server, ServerInput>('/servers', input);
    if (error) {
        throwIfPassphraseRequired(error);
        throw new Error(error);
    }
    return data;
}

/**
 * Update an existing server
 */
export async function updateServer(serverId: string, input: ServerInput): Promise<Server | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.updateServer(serverId, input);
        if (error) {
            throwIfPassphraseRequired(error);
            console.error(`Failed to update server ${serverId} (bindings):`, error);
            return null;
        }
        return data as unknown as Server;
    }

    const {data, error} = await put<Server, ServerInput>(`/servers/${serverId}`, input);
    if (error) {
        throwIfPassphraseRequired(error);
        console.error(`Failed to update server ${serverId}:`, error);
        return null;
    }
    return data;
}

/**
 * (Re)generate the CA, server and client mTLS certificates for a server's
 * agent and store them on the server record.
 */
export async function generateAgentTLS(serverId: string): Promise<Server | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.generateAgentTLS(serverId);
        if (error) {
            console.error(`Failed to generate agent TLS for ${serverId} (bindings):`, error);
            return null;
        }
        return data as unknown as Server;
    }

    const {data, error} = await post<Server, undefined>(`/servers/${serverId}/tls`, undefined);
    if (error) {
        console.error(`Failed to generate agent TLS for ${serverId}:`, error);
        return null;
    }
    return data;
}

/**
 * Caches passphrase as the SSH key's passphrase for serverId for the
 * remainder of the current session (never persisted) and immediately
 * retries connecting. When applyToAll is true, the passphrase also becomes
 * the fallback tried for any other server's key that needs one.
 */
export async function unlockServerSSH(
    serverId: string,
    passphrase: string,
    applyToAll: boolean,
): Promise<boolean> {
    const client = getClient();
    const body = {passphrase, applyToAll};

    if (client) {
        const {error} = await client.unlockServerSSH(serverId, passphrase, applyToAll);
        if (error) {
            console.error(`Failed to unlock SSH key for ${serverId} (bindings):`, error);
            return false;
        }
        return true;
    }

    const {error} = await post<void, typeof body>(`/servers/${serverId}/ssh/unlock`, body);
    if (error) {
        console.error(`Failed to unlock SSH key for ${serverId}:`, error);
        return false;
    }
    return true;
}

/**
 * Live WireGuard/AmneziaWG interfaces on the server that its agent doesn't
 * manage — brought up by wg-quick/awg-quick before the agent was installed.
 * Best-effort like getServerMetrics: null when the agent is unreachable (or
 * predates the endpoint), which callers treat as "unknown", no toast.
 */
export async function listUnmanagedInterfaces(serverId: string): Promise<UnmanagedInterface[] | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.listUnmanagedInterfaces(serverId);
        if (error) {
            console.error(`Failed to list unmanaged interfaces for server ${serverId} (bindings):`, error);
            return null;
        }
        return data as unknown as UnmanagedInterface[];
    }

    const {data, error} = await get<UnmanagedInterface[]>(`/servers/${serverId}/unmanaged`);
    if (error) {
        console.error(`Failed to list unmanaged interfaces for server ${serverId}:`, error);
        return null;
    }
    return data;
}

export interface ImportRequest {
    interface: string;
    peerMap: string;
}

/**
 * Dry-run of importing a wg-quick/awg-quick interface from the server (the
 * agent reads its conf from disk): what would be created, the translated
 * hooks, warnings. Writes nothing. Throws the backend's message (a validation
 * problem in the conf or peer map) so the modal can show the specific reason.
 */
export async function previewInterfaceImport(serverId: string, iface: string, peerMap: string): Promise<ImportPreview> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.previewImport(serverId, iface, peerMap);
        if (error) throw new Error(String(error));
        return data as unknown as ImportPreview;
    }

    const {data, error} = await post<ImportPreview, ImportRequest>(`/servers/${serverId}/import/preview`, {interface: iface, peerMap});
    if (error) throw new Error(String(error));
    return data;
}

/**
 * Imports a wg-quick/awg-quick interface from the server into the database
 * (interface + users + peers in one transaction) and pushes it to the agent.
 * Throws the backend's message on failure, like previewInterfaceImport.
 */
export async function importInterfaceFromServer(serverId: string, iface: string, peerMap: string): Promise<Interface> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.importInterfaceFromServer(serverId, iface, peerMap);
        if (error) throw new Error(String(error));
        return data as unknown as Interface;
    }

    const {data, error} = await post<Interface, ImportRequest>(`/servers/${serverId}/import`, {interface: iface, peerMap});
    if (error) throw new Error(String(error));
    return data;
}

/**
 * Fetch every host-level sample still retained in a server's agent's
 * in-memory ring buffer (up to 48h, oldest first) for charting. Returns null
 * if the agent is unreachable.
 */
export async function getServerMetricsHistory(serverId: string): Promise<SystemHistory | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.getServerMetricsHistory(serverId);
        if (error) {
            console.error(`Failed to fetch metrics history for server ${serverId} (bindings):`, error);
            return null;
        }
        return data as unknown as SystemHistory;
    }

    const {data, error} = await get<SystemHistory>(`/servers/${serverId}/metrics/history`);
    if (error) {
        console.error(`Failed to fetch metrics history for server ${serverId}:`, error);
        return null;
    }
    return data;
}

export async function getServerInterfaceMetricsHistory(serverId: string, interfaceId: string): Promise<InterfaceHistory | null> {
    const client = getClient();
    if (client) {
        const {data, error} = await client.getServerInterfaceMetricsHistory(serverId, interfaceId);
        if (error) {
            console.error(`Failed to fetch interface metrics history for ${serverId}/${interfaceId} (bindings):`, error);
            return null;
        }
        return data as unknown as InterfaceHistory;
    }
    const {data, error} = await get<InterfaceHistory>(`/servers/${serverId}/interfaces/${interfaceId}/metrics/history`);
    if (error) {
        console.error(`Failed to fetch interface metrics history for ${serverId}/${interfaceId}:`, error);
        return null;
    }
    return data;
}

/**
 * Agent health and migration state: 'ok', 'down', 'degraded', or 'unmanaged'
 * while migrated inventory waits for a v2 agent with the AWG provider.
 */
export type AgentStatus = 'ok' | 'down' | 'degraded' | 'unmanaged';

export async function getServerAgentStatus(serverId: string): Promise<AgentStatus | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.getServerAgentStatus(serverId);
        if (error) {
            console.error(`Failed to fetch agent status for server ${serverId} (bindings):`, error);
            return null;
        }
        return data as AgentStatus;
    }

    const {data, error} = await get<{status: AgentStatus}>(`/servers/${serverId}/agent-status`);
    if (error) {
        console.error(`Failed to fetch agent status for server ${serverId}:`, error);
        return null;
    }
    return data.status;
}
