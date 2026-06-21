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

import {get, patch, post, put, remove} from './api';
import {getCurrentApiMode} from './apiMode';
import {bindingsClient} from './bindingsClient';
import {throwIfPassphraseRequired} from './sshErrors';
import {reportError} from './errorReporting';
import type {Agent, DeployStatus, Interface, InterfaceConfig, MetricsSnapshot, Server, ServerInfo, SSHConfig, SystemHistory} from '@/types';

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

/**
 * Get a single server by ID
 */
export async function getServer(serverId: string): Promise<Server | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.getServer(serverId);
        if (error) {
            reportError(`fetch-server-${serverId}`, `Failed to fetch server ${serverId}`, error);
            return null;
        }
        return data as unknown as Server;
    }

    const {data, error} = await get<Server>(`/servers/${serverId}`);
    if (error) {
        reportError(`fetch-server-${serverId}`, `Failed to fetch server ${serverId}`, error);
        return null;
    }
    return data;
}

/**
 * Create a new server
 */
export async function createServer(input: ServerInput): Promise<Server | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.createServer(input);
        if (error) {
            throwIfPassphraseRequired(error);
            console.error('Failed to create server (bindings):', error);
            return null;
        }
        return data as unknown as Server;
    }

    const {data, error} = await post<Server, ServerInput>('/servers', input);
    if (error) {
        throwIfPassphraseRequired(error);
        console.error('Failed to create server:', error);
        return null;
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
 * Deploy (or redeploy) the agent binary + systemd unit + config to the
 * server's host over SSH, and enable/start the service.
 */
export async function deployAgent(serverId: string, agentSourceId: string): Promise<boolean> {
    const client = getClient();

    if (client) {
        const {error} = await client.deployAgent(serverId, agentSourceId);
        if (error) {
            throwIfPassphraseRequired(error);
            console.error(`Failed to deploy agent to ${serverId} (bindings):`, error);
            return false;
        }
        return true;
    }

    const {error} = await post<void, {agentSourceId: string}>(`/servers/${serverId}/deploy`, {agentSourceId});
    if (error) {
        throwIfPassphraseRequired(error);
        console.error(`Failed to deploy agent to ${serverId}:`, error);
        return false;
    }
    return true;
}

/**
 * Polls the progress of the most recent deployAgent call for a server (see
 * models.DeployStatus) — null if no deploy has been started yet (404,
 * normal right after opening the modal for a server that was never
 * deployed to before). Throws SSHPassphraseRequiredError if the deploy
 * that just finished failed because the SSH key needs a passphrase, same
 * as deployAgent itself would for an immediate failure.
 */
export async function getDeployStatus(serverId: string): Promise<DeployStatus | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.getDeployStatus(serverId);
        if (error) return null;
        throwIfPassphraseRequired(data?.error);
        return data as unknown as DeployStatus;
    }

    const {data, error} = await get<DeployStatus>(`/servers/${serverId}/deploy/status`);
    if (error) return null;
    throwIfPassphraseRequired(data?.error);
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
 * Delete a server
 */
export async function deleteServer(serverId: string): Promise<boolean> {
    const client = getClient();

    if (client) {
        const {error} = await client.deleteServer(serverId);
        if (error) {
            console.error(`Failed to delete server ${serverId} (bindings):`, error);
            return false;
        }
        return true;
    }

    const {error} = await remove<void>(`/servers/${serverId}`);
    if (error) {
        console.error(`Failed to delete server ${serverId}:`, error);
        return false;
    }
    return true;
}

/**
 * Resend every interface of a server to its agent (e.g. after a redeploy,
 * or to recover from a push that failed while the agent was unreachable).
 */
export async function syncServer(serverId: string): Promise<boolean> {
    const client = getClient();

    if (client) {
        const {error} = await client.syncServer(serverId);
        if (error) {
            console.error(`Failed to sync server ${serverId} (bindings):`, error);
            return false;
        }
        return true;
    }

    const {error} = await post<void, undefined>(`/servers/${serverId}/sync`, undefined);
    if (error) {
        console.error(`Failed to sync server ${serverId}:`, error);
        return false;
    }
    return true;
}

/**
 * Result of comparing a server's agent's actual interfaces against the
 * local DB's records for it (see Service.ReconcileServer) — non-empty only
 * when one side lost its state (DB restored from an old backup, or the
 * agent's own storage was lost/reinstalled).
 */
export interface ReconcileReport {
    agentOnly: InterfaceConfig[];
    dbOnly: Interface[];
}

/**
 * Compares serverId's agent's actual interfaces against this DB's records
 * for it, by interface name. Read-only — see importInterface/
 * deleteAgentInterface for resolving an "agent has it, DB doesn't"
 * mismatch, or use syncServer/deleteInterface for the reverse.
 */
export async function reconcileServer(serverId: string): Promise<ReconcileReport | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.reconcileServer(serverId);
        if (error) {
            reportError(`reconcile-server-${serverId}`, `Failed to reconcile server ${serverId}`, error);
            return null;
        }
        return data as unknown as ReconcileReport;
    }

    const {data, error} = await get<ReconcileReport>(`/servers/${serverId}/reconcile`);
    if (error) {
        reportError(`reconcile-server-${serverId}`, `Failed to reconcile server ${serverId}`, error);
        return null;
    }
    return data;
}

/**
 * Creates a new DB record for ifaceName from what the agent already has
 * configured for it — the "agent has it, DB doesn't" reconciliation case.
 * Only the interface shell is recovered; see Service.ImportInterface's doc
 * comment for what isn't (per-user peer associations).
 */
export async function importInterface(serverId: string, iface: string): Promise<boolean> {
    const client = getClient();

    if (client) {
        const {error} = await client.importInterface(serverId, iface);
        if (error) {
            console.error(`Failed to import interface ${iface} for server ${serverId} (bindings):`, error);
            return false;
        }
        return true;
    }

    const {error} = await post<void, {interface: string}>(`/servers/${serverId}/reconcile/import`, {interface: iface});
    if (error) {
        console.error(`Failed to import interface ${iface} for server ${serverId}:`, error);
        return false;
    }
    return true;
}

/**
 * Removes ifaceName directly from serverId's agent, without touching the
 * DB (there's nothing there to remove) — the other "agent has it, DB
 * doesn't" choice, alongside importInterface.
 */
export async function deleteAgentInterface(serverId: string, iface: string): Promise<boolean> {
    const client = getClient();

    if (client) {
        const {error} = await client.deleteAgentInterface(serverId, iface);
        if (error) {
            console.error(`Failed to delete agent interface ${iface} for server ${serverId} (bindings):`, error);
            return false;
        }
        return true;
    }

    const {error} = await post<void, {interface: string}>(`/servers/${serverId}/reconcile/delete-agent`, {interface: iface});
    if (error) {
        console.error(`Failed to delete agent interface ${iface} for server ${serverId}:`, error);
        return false;
    }
    return true;
}

/**
 * Enable/disable the agent's metrics collection for a server, and persist
 * the choice so it's re-applied by syncServer (e.g. after a redeploy).
 */
export async function setServerMonitoring(serverId: string, enabled: boolean): Promise<Server | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.setServerMonitoring(serverId, enabled);
        if (error) {
            console.error(`Failed to set monitoring state for ${serverId} (bindings):`, error);
            return null;
        }
        return data as unknown as Server;
    }

    const {data, error} = await patch<Server, {enabled: boolean}>(`/servers/${serverId}/monitoring`, {enabled});
    if (error) {
        console.error(`Failed to set monitoring state for ${serverId}:`, error);
        return null;
    }
    return data;
}

/**
 * Fetch a server's latest CPU/RAM/load/network/peer metrics snapshot from
 * its agent. Returns null if the agent is unreachable or hasn't collected
 * anything yet — callers should treat that as "no data" rather than an
 * error needing a toast (metrics are best-effort, secondary information).
 */
export async function getServerMetrics(serverId: string): Promise<MetricsSnapshot | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.getServerMetrics(serverId);
        if (error) {
            console.error(`Failed to fetch metrics for server ${serverId} (bindings):`, error);
            return null;
        }
        return data as unknown as MetricsSnapshot;
    }

    const {data, error} = await get<MetricsSnapshot>(`/servers/${serverId}/metrics`);
    if (error) {
        console.error(`Failed to fetch metrics for server ${serverId}:`, error);
        return null;
    }
    return data;
}

/**
 * Fetch every host-level sample still retained in a server's agent's
 * in-memory ring buffer (up to 48h, oldest first) — for charting, unlike
 * getServerMetrics which only returns the latest value. Returns null if the
 * agent is unreachable, same best-effort treatment as getServerMetrics.
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

/**
 * Check whether a server currently has a live SSH tunnel open. Only
 * meaningful for servers without mTLS configured (see models.Agent.TLS) —
 * mTLS servers are reached directly and never have a tunnel, so callers
 * should check that first rather than relying on this returning false for
 * them. Best-effort like getServerMetrics: returns null rather than
 * throwing on failure.
 */
export async function getServerTunnelStatus(serverId: string): Promise<boolean | null> {
    const client = getClient();

    if (client) {
        const {data, error} = await client.getServerTunnelStatus(serverId);
        if (error) {
            console.error(`Failed to fetch tunnel status for server ${serverId} (bindings):`, error);
            return null;
        }
        return data;
    }

    const {data, error} = await get<{open: boolean}>(`/servers/${serverId}/tunnel-status`);
    if (error) {
        console.error(`Failed to fetch tunnel status for server ${serverId}:`, error);
        return null;
    }
    return data.open;
}
