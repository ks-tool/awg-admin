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
import {get, post, put, remove} from './api';
import {getCurrentApiMode} from './apiMode';
import type {
    NetworkConnection,
    NetworkInterface,
    NodePosition,
    PluginDescriptor,
    PluginUser,
    Topology,
    VPNService,
} from '@/types';

export interface TopologyServerInput {
    name: string;
    address: string;
    description: string;
    location: string;
    latitude: number | null;
    longitude: number | null;
    interfaces: NetworkInterfaceInput[];
}

export function topologyCoordinates(latitude: string, longitude: string): Pick<TopologyServerInput, 'latitude' | 'longitude'> {
    return {
        latitude: latitude.trim() === '' ? null : Number(latitude),
        longitude: longitude.trim() === '' ? null : Number(longitude),
    };
}

export interface NetworkInterfaceInput {
    name: string;
    address: string;
}

export interface VPNServiceInput {
    serverId: string;
    interfaceId: string;
    pluginId: string;
    name: string;
    config: Record<string, unknown>;
}

function errorMessage(error: unknown): string {
    if (error instanceof Error) return error.message;
    return typeof error === 'string' ? error : 'Unknown error';
}

async function binding<T>(operation: () => Promise<unknown>): Promise<T> {
    try {
        // Wails models UUIDs as number[], while the runtime and HTTP API carry
        // canonical UUID strings. Keep the generated transport types at this
        // boundary and use the application's domain types everywhere else.
        return await operation() as T;
    } catch (error) {
        throw new Error(errorMessage(error));
    }
}

function unwrap<T>(response: {data: T; error?: string}): T {
    if (response.error) throw new Error(response.error);
    return response.data;
}

const isDesktop = () => getCurrentApiMode() === 'bindings';

export async function getTopology(): Promise<Topology> {
    return isDesktop()
        ? binding(() => AppBindings.GetTopology()) as Promise<Topology>
        : unwrap(await get<Topology>('/topology'));
}

export async function listVPNPlugins(): Promise<PluginDescriptor[]> {
    return isDesktop()
        ? binding(() => AppBindings.ListVPNPlugins()) as Promise<PluginDescriptor[]>
        : unwrap(await get<PluginDescriptor[]>('/topology/plugins'));
}

export async function createTopologyServer(input: TopologyServerInput): Promise<Topology> {
    return isDesktop()
        ? binding(() => AppBindings.CreateTopologyServer(input as never)) as Promise<Topology>
        : unwrap(await post<Topology, TopologyServerInput>('/topology/servers', input));
}

export async function updateTopologyServer(id: string, input: TopologyServerInput): Promise<Topology> {
    return isDesktop()
        ? binding(() => AppBindings.UpdateTopologyServer(id, input as never)) as Promise<Topology>
        : unwrap(await put<Topology, TopologyServerInput>(`/topology/servers/${id}`, input));
}

export async function deleteTopologyServer(id: string): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.DeleteTopologyServer(id));
    unwrap(await remove<void>(`/topology/servers/${id}`));
}

export async function createNetworkInterface(serverId: string, input: NetworkInterfaceInput): Promise<Topology> {
    return isDesktop()
        ? binding(() => AppBindings.CreateNetworkInterface(serverId, input as never)) as Promise<Topology>
        : unwrap(await post<Topology, NetworkInterfaceInput>(`/topology/servers/${serverId}/interfaces`, input));
}

export async function updateNetworkInterface(id: string, input: NetworkInterfaceInput): Promise<Topology> {
    return isDesktop()
        ? binding(() => AppBindings.UpdateNetworkInterface(id, input as never)) as Promise<Topology>
        : unwrap(await put<Topology, NetworkInterfaceInput>(`/topology/interfaces/${id}`, input));
}

export async function deleteNetworkInterface(id: string): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.DeleteNetworkInterface(id));
    unwrap(await remove<void>(`/topology/interfaces/${id}`));
}

export async function createVPNService(input: VPNServiceInput): Promise<VPNService> {
    return isDesktop()
        ? binding(() => AppBindings.CreateVPNService(input as never)) as Promise<VPNService>
        : unwrap(await post<VPNService, VPNServiceInput>('/topology/services', input));
}

export async function updateVPNService(id: string, input: VPNServiceInput): Promise<VPNService> {
    return isDesktop()
        ? binding(() => AppBindings.UpdateVPNService(id, input as never)) as Promise<VPNService>
        : unwrap(await put<VPNService, VPNServiceInput>(`/topology/services/${id}`, input));
}

export async function deleteVPNService(id: string): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.DeleteVPNService(id));
    unwrap(await remove<void>(`/topology/services/${id}`));
}

export async function applyVPNService(id: string): Promise<VPNService> {
    return isDesktop()
        ? binding(() => AppBindings.ApplyVPNService(id)) as Promise<VPNService>
        : unwrap(await post<VPNService, unknown>(`/topology/services/${id}/apply`, {}));
}

export async function refreshVPNServiceStatus(id: string): Promise<VPNService> {
    return isDesktop()
        ? binding(() => AppBindings.RefreshVPNServiceStatus(id)) as Promise<VPNService>
        : unwrap(await post<VPNService, unknown>(`/topology/services/${id}/status`, {}));
}

export async function createNetworkConnection(sourceId: string, targetId: string): Promise<NetworkConnection> {
    return isDesktop()
        ? binding(() => AppBindings.CreateNetworkConnection(sourceId, targetId)) as Promise<NetworkConnection>
        : unwrap(await post<NetworkConnection, {sourceId: string; targetId: string}>('/topology/connections', {sourceId, targetId}));
}

export async function deleteNetworkConnection(id: string): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.DeleteNetworkConnection(id));
    unwrap(await remove<void>(`/topology/connections/${id}`));
}

export async function moveTopologyNodes(positions: NodePosition[]): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.MoveTopologyNodes(positions as never));
    unwrap(await put<void, {positions: NodePosition[]}>('/topology/positions', {positions}));
}

export async function listVPNServiceUsers(serviceId: string): Promise<PluginUser[]> {
    return isDesktop()
        ? binding(() => AppBindings.ListVPNServiceUsers(serviceId)) as Promise<PluginUser[]>
        : unwrap(await get<PluginUser[]>(`/topology/services/${serviceId}/users`));
}

export async function createVPNServiceUser(serviceId: string, input: Record<string, unknown>): Promise<PluginUser> {
    return isDesktop()
        ? binding(() => AppBindings.CreateVPNServiceUser(serviceId, input as never)) as Promise<PluginUser>
        : unwrap(await post<PluginUser, Record<string, unknown>>(`/topology/services/${serviceId}/users`, input));
}

export async function updateVPNServiceUser(serviceId: string, userId: string, input: Record<string, unknown>): Promise<PluginUser> {
    const encodedId = encodeURIComponent(userId);
    return isDesktop()
        ? binding(() => AppBindings.UpdateVPNServiceUser(serviceId, userId, input as never)) as Promise<PluginUser>
        : unwrap(await put<PluginUser, Record<string, unknown>>(`/topology/services/${serviceId}/users/${encodedId}`, input));
}

export async function deleteVPNServiceUser(serviceId: string, userId: string): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.DeleteVPNServiceUser(serviceId, userId));
    unwrap(await remove<void>(`/topology/services/${serviceId}/users/${encodeURIComponent(userId)}`));
}

export type {NetworkInterface};
