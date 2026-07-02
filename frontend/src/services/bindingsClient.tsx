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
import { models, service } from '../../wailsjs/go/models';

interface ApiResponse<T> {
    data: T;
    error?: string;
}

// Wails rejects bound-method promises with the Go error's message as a bare
// string, not an Error instance — `error instanceof Error` is always false
// here, so checking only that would hide every real backend error message
// behind a generic "Unknown error".
function extractErrorMessage(error: unknown): string {
    if (error instanceof Error) return error.message;
    if (typeof error === 'string') return error;
    return 'Unknown error';
}

/**
 * Wrapper for Wails bindings that converts them to the same interface as HTTP API
 */
export const bindingsClient = {
    async listUsers(): Promise<ApiResponse<models.User[]>> {
        try {
            const data = await AppBindings.ListUsers();
            return { data: data || [] };
        } catch (error) {
            console.error('Bindings ListUsers failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async getUser(id: string): Promise<ApiResponse<models.User>> {
        try {
            const data = await AppBindings.GetUser(id);
            return { data };
        } catch (error) {
            console.error('Bindings GetUser failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async createUser(input: any): Promise<ApiResponse<models.User>> {
        try {
            const data = await AppBindings.CreateUser(input);
            return { data };
        } catch (error) {
            console.error('Bindings CreateUser failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async updateUser(id: string, input: any): Promise<ApiResponse<models.User>> {
        try {
            const data = await AppBindings.UpdateUser(id, input);
            return { data };
        } catch (error) {
            console.error('Bindings UpdateUser failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async deleteUser(id: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.DeleteUser(id);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings DeleteUser failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async listServers(): Promise<ApiResponse<models.Server[]>> {
        try {
            const data = await AppBindings.ListServers();
            return { data: data || [] };
        } catch (error) {
            console.error('Bindings ListServers failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async getServer(id: string): Promise<ApiResponse<models.Server>> {
        try {
            const data = await AppBindings.GetServer(id);
            return { data };
        } catch (error) {
            console.error('Bindings GetServer failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async createServer(input: any): Promise<ApiResponse<models.Server>> {
        try {
            const data = await AppBindings.CreateServer(input);
            return { data };
        } catch (error) {
            console.error('Bindings CreateServer failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async updateServer(id: string, input: any): Promise<ApiResponse<models.Server>> {
        try {
            const data = await AppBindings.UpdateServer(id, input);
            return { data };
        } catch (error) {
            console.error('Bindings UpdateServer failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async generateAgentTLS(id: string): Promise<ApiResponse<models.Server>> {
        try {
            const data = await AppBindings.GenerateAgentTLS(id);
            return { data };
        } catch (error) {
            console.error('Bindings GenerateAgentTLS failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async deployAgent(id: string, agentSourceId: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.DeployAgent(id, agentSourceId);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings DeployAgent failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async listAgentSources(): Promise<ApiResponse<models.AgentSource[]>> {
        try {
            const data = await AppBindings.ListAgentSources();
            return { data: data || [] };
        } catch (error) {
            console.error('Bindings ListAgentSources failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async createAgentSource(name: string, url: string, path: string, image: string, cacheLocally: boolean): Promise<ApiResponse<models.AgentSource>> {
        try {
            const data = await AppBindings.CreateAgentSource(name, url, path, image, cacheLocally);
            return { data };
        } catch (error) {
            console.error('Bindings CreateAgentSource failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async deleteAgentSource(id: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.DeleteAgentSource(id);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings DeleteAgentSource failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async refreshAgentSourceCache(id: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.RefreshAgentSourceCache(id);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings RefreshAgentSourceCache failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async getDeployStatus(serverId: string): Promise<ApiResponse<models.DeployStatus>> {
        try {
            const data = await AppBindings.GetDeployStatus(serverId);
            return { data };
        } catch (error) {
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async deleteServer(id: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.DeleteServer(id);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings DeleteServer failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async syncServer(id: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.SyncServer(id);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings SyncServer failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async reconcileServer(id: string): Promise<ApiResponse<service.ReconcileReport>> {
        try {
            const data = await AppBindings.ReconcileServer(id);
            return { data };
        } catch (error) {
            console.error('Bindings ReconcileServer failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async importInterface(id: string, iface: string): Promise<ApiResponse<models.Interface>> {
        try {
            const data = await AppBindings.ImportInterface(id, iface);
            return { data };
        } catch (error) {
            console.error('Bindings ImportInterface failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async deleteAgentInterface(id: string, iface: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.DeleteAgentInterface(id, iface);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings DeleteAgentInterface failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async setServerMonitoring(id: string, enabled: boolean): Promise<ApiResponse<models.Server>> {
        try {
            const data = await AppBindings.SetServerMonitoring(id, enabled);
            return { data };
        } catch (error) {
            console.error('Bindings SetServerMonitoring failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async unlockServerSSH(id: string, passphrase: string, applyToAll: boolean): Promise<ApiResponse<void>> {
        try {
            await AppBindings.UnlockServerSSH(id, passphrase, applyToAll);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings UnlockServerSSH failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async getServerMetrics(id: string): Promise<ApiResponse<models.MetricsSnapshot>> {
        try {
            const data = await AppBindings.GetServerMetrics(id);
            return { data };
        } catch (error) {
            console.error('Bindings GetServerMetrics failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async getServerAgentStatus(id: string): Promise<ApiResponse<string>> {
        try {
            const data = await AppBindings.ServerAgentStatus(id);
            return { data };
        } catch (error) {
            console.error('Bindings ServerAgentStatus failed:', error);
            return {
                data: '',
                error: extractErrorMessage(error),
            };
        }
    },

    async getServerHostInfo(id: string): Promise<ApiResponse<models.HostInfo>> {
        try {
            const data = await AppBindings.ServerHostInfo(id);
            return { data };
        } catch (error) {
            console.error('Bindings ServerHostInfo failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async getServerMetricsHistory(id: string): Promise<ApiResponse<models.SystemHistory>> {
        try {
            const data = await AppBindings.GetServerMetricsHistory(id);
            return { data };
        } catch (error) {
            console.error('Bindings GetServerMetricsHistory failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async listInterfaces(serverId: string): Promise<ApiResponse<models.Interface[]>> {
        try {
            const data = await AppBindings.ListInterfaces(serverId);
            return { data: data || [] };
        } catch (error) {
            console.error('Bindings ListInterfaces failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async listTunnels(): Promise<ApiResponse<models.Tunnel[]>> {
        try {
            const data = await AppBindings.ListTunnels();
            return { data: data || [] };
        } catch (error) {
            console.error('Bindings ListTunnels failed:', error);
            return { data: null as any, error: extractErrorMessage(error) };
        }
    },

    // steps carry string uuids at runtime (Go marshals uuid.UUID as a string),
    // but Wails' generated models.TunnelStep types them as number[] — so accept
    // the real shape and cast for the binding call.
    async buildTunnel(steps: { serverId: string; ifaceId: string }[], subnet: string): Promise<ApiResponse<models.Tunnel>> {
        try {
            const data = await AppBindings.BuildTunnel(steps as unknown as models.TunnelStep[], subnet);
            return { data };
        } catch (error) {
            console.error('Bindings BuildTunnel failed:', error);
            return { data: null as any, error: extractErrorMessage(error) };
        }
    },

    async removeTunnel(id: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.RemoveTunnel(id);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings RemoveTunnel failed:', error);
            return { data: undefined, error: extractErrorMessage(error) };
        }
    },

    async getInterface(serverId: string, interfaceId: string): Promise<ApiResponse<models.Interface>> {
        try {
            const data = await AppBindings.GetInterface(serverId, interfaceId);
            return { data };
        } catch (error) {
            console.error('Bindings GetInterface failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async createInterface(serverId: string, config: any): Promise<ApiResponse<models.Interface>> {
        try {
            const data = await AppBindings.CreateInterface(serverId, config);
            return { data };
        } catch (error) {
            console.error('Bindings CreateInterface failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async getInterfaceDefaults(): Promise<ApiResponse<models.InterfaceConfig>> {
        try {
            const data = await AppBindings.GenerateInterfaceDefaults();
            return { data };
        } catch (error) {
            console.error('Bindings GenerateInterfaceDefaults failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async updateInterfaceConfig(serverId: string, interfaceId: string, config: any): Promise<ApiResponse<models.Interface>> {
        try {
            const data = await AppBindings.UpdateInterfaceConfig(serverId, interfaceId, config);
            return { data };
        } catch (error) {
            console.error('Bindings UpdateInterfaceConfig failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async deleteInterface(serverId: string, interfaceId: string): Promise<ApiResponse<void>> {
        try {
            await AppBindings.DeleteInterface(serverId, interfaceId);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings DeleteInterface failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    async listPeers(userId: string): Promise<ApiResponse<models.Peer[]>> {
        try {
            const data = await AppBindings.ListPeers(userId);
            return { data: data || [] };
        } catch (error) {
            console.error('Bindings ListPeers failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async getPeer(userId: string, publicKey: string): Promise<ApiResponse<models.Peer>> {
        try {
            const data = await AppBindings.GetPeer(userId, publicKey);
            return { data };
        } catch (error) {
            console.error('Bindings GetPeer failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async addPeer(userId: string, input: any): Promise<ApiResponse<models.User>> {
        try {
            const data = await AppBindings.AddPeer(userId, input);
            return { data };
        } catch (error) {
            console.error('Bindings AddPeer failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async getPeerConfig(userId: string, publicKey: string): Promise<ApiResponse<string>> {
        try {
            const data = await AppBindings.GetPeerConfig(userId, publicKey);
            return { data };
        } catch (error) {
            console.error('Bindings GetPeerConfig failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async getPeerQRCode(userId: string, publicKey: string): Promise<ApiResponse<string>> {
        try {
            const data = await AppBindings.GetPeerQRCode(userId, publicKey);
            return { data };
        } catch (error) {
            console.error('Bindings GetPeerQRCode failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    // Desktop-only "save QR as PNG": Go opens a native save dialog and writes
    // the file (the webview can't download a data: URL). Resolves true if a
    // file was written, false if the dialog was cancelled.
    async savePeerQRCode(userId: string, publicKey: string, defaultName: string): Promise<ApiResponse<boolean>> {
        try {
            const data = await AppBindings.SavePeerQRCode(userId, publicKey, defaultName);
            return { data };
        } catch (error) {
            console.error('Bindings SavePeerQRCode failed:', error);
            return {
                data: false,
                error: extractErrorMessage(error),
            };
        }
    },

    // Desktop-only: the captured stdout logs (NDJSON text) for the Settings
    // "Logs" modal.
    async getLogs(): Promise<ApiResponse<string>> {
        try {
            const data = await AppBindings.GetLogs();
            return { data };
        } catch (error) {
            console.error('Bindings GetLogs failed:', error);
            return {
                data: '',
                error: extractErrorMessage(error),
            };
        }
    },

    // Desktop-only: read whether debug-level log capture is currently on.
    async debugLoggingEnabled(): Promise<ApiResponse<boolean>> {
        try {
            const data = await AppBindings.DebugLoggingEnabled();
            return { data };
        } catch (error) {
            console.error('Bindings DebugLoggingEnabled failed:', error);
            return {
                data: false,
                error: extractErrorMessage(error),
            };
        }
    },

    // Desktop-only: turn debug-level log capture on or off at runtime.
    async setDebugLogging(enabled: boolean): Promise<ApiResponse<void>> {
        try {
            await AppBindings.SetDebugLogging(enabled);
            return { data: undefined };
        } catch (error) {
            console.error('Bindings SetDebugLogging failed:', error);
            return {
                data: undefined,
                error: extractErrorMessage(error),
            };
        }
    },

    // Desktop-only: Go opens a native save dialog and writes the captured logs
    // as a JSON file. Resolves true if a file was written, false if cancelled.
    async saveLogs(): Promise<ApiResponse<boolean>> {
        try {
            const data = await AppBindings.SaveLogs();
            return { data };
        } catch (error) {
            console.error('Bindings SaveLogs failed:', error);
            return {
                data: false,
                error: extractErrorMessage(error),
            };
        }
    },

    // Desktop-only: Go builds a full DB snapshot and writes it to a user-chosen
    // file via a native save dialog. Resolves true if a file was written,
    // false if cancelled.
    async saveBackup(): Promise<ApiResponse<boolean>> {
        try {
            const data = await AppBindings.SaveBackup();
            return { data };
        } catch (error) {
            console.error('Bindings SaveBackup failed:', error);
            return {
                data: false,
                error: extractErrorMessage(error),
            };
        }
    },

    // Desktop-only: open a native file picker and return the chosen absolute
    // path ('' if cancelled). No HTTP equivalent — a browser can't hand back a
    // real filesystem path.
    async selectFile(title: string): Promise<ApiResponse<string>> {
        try {
            const data = await AppBindings.SelectFile(title);
            return { data };
        } catch (error) {
            console.error('Bindings SelectFile failed:', error);
            return {
                data: '',
                error: extractErrorMessage(error),
            };
        }
    },

    async deletePeer(userId: string, publicKey: string): Promise<ApiResponse<models.User>> {
        try {
            const data = await AppBindings.DeletePeer(userId, publicKey);
            return { data };
        } catch (error) {
            console.error('Bindings DeletePeer failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },

    async migratePeer(userId: string, publicKey: string, interfaceId: string): Promise<ApiResponse<models.User>> {
        try {
            const data = await AppBindings.MigratePeer(userId, publicKey, interfaceId);
            return { data };
        } catch (error) {
            console.error('Bindings MigratePeer failed:', error);
            return {
                data: null as any,
                error: extractErrorMessage(error),
            };
        }
    },
};
