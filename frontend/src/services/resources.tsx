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
import type {ActionResult, ProviderDefinition, ProviderLink, ProviderManifest, ProviderResource, ResourceStatus} from '@/types';

function unwrap<T>(response: {data: T; error?: string}): T {
    if (response.error) throw new Error(response.error);
    return response.data;
}

async function binding<T>(operation: () => Promise<unknown>): Promise<T> {
    try {
        return await operation() as T;
    } catch (error) {
        throw new Error(error instanceof Error ? error.message : typeof error === 'string' ? error : 'Unknown error');
    }
}

const isDesktop = () => getCurrentApiMode() === 'bindings';
const base = (serverId: string, provider: string) => `/servers/${encodeURIComponent(serverId)}/providers/${encodeURIComponent(provider)}`;
const resourcePath = (serverId: string, provider: string, kind: string, name: string) =>
    `${base(serverId, provider)}/resources/${encodeURIComponent(kind)}/${encodeURIComponent(name)}`;

export async function getProviderManifest(serverId: string): Promise<ProviderManifest> {
    return isDesktop()
        ? binding(() => AppBindings.GetProviderManifest(serverId))
        : unwrap(await get<ProviderManifest>(`/servers/${encodeURIComponent(serverId)}/providers/manifest`));
}

export async function getProviderSchema(serverId: string, provider: string): Promise<ProviderDefinition> {
    return isDesktop()
        ? binding(() => AppBindings.GetProviderSchema(serverId, provider))
        : unwrap(await get<ProviderDefinition>(`${base(serverId, provider)}/schema`));
}

export async function listProviderResources(serverId: string, provider: string): Promise<ProviderResource[]> {
    const result = isDesktop()
        ? await binding<ProviderResource[]>(() => AppBindings.ListProviderResources(serverId, provider))
        : unwrap(await get<ProviderResource[]>(`${base(serverId, provider)}/resources`));
    return result ?? [];
}

export async function putProviderResource(serverId: string, provider: string, kind: string, name: string, spec: Record<string, unknown>): Promise<ProviderResource> {
    return isDesktop()
        ? binding(() => AppBindings.PutProviderResource(serverId, provider, kind, name, spec as never))
        : unwrap(await put<ProviderResource, {spec: Record<string, unknown>}>(resourcePath(serverId, provider, kind, name), {spec}));
}

export async function deleteProviderResource(serverId: string, provider: string, kind: string, name: string): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.DeleteProviderResource(serverId, provider, kind, name));
    unwrap(await remove<void>(resourcePath(serverId, provider, kind, name)));
}

export async function getProviderResourceStatus(serverId: string, provider: string, kind: string, name: string): Promise<ResourceStatus> {
    return isDesktop()
        ? binding(() => AppBindings.GetProviderResourceStatus(serverId, provider, kind, name))
        : unwrap(await get<ResourceStatus>(`${resourcePath(serverId, provider, kind, name)}/status`));
}

export async function runProviderResourceAction(serverId: string, provider: string, kind: string, name: string, action: string, args: Record<string, unknown>): Promise<ActionResult> {
    return isDesktop()
        ? binding(() => AppBindings.RunProviderResourceAction(serverId, provider, kind, name, action, args as never))
        : unwrap(await post<ActionResult, {arguments: Record<string, unknown>}>(`${resourcePath(serverId, provider, kind, name)}/actions/${encodeURIComponent(action)}`, {arguments: args}));
}

export async function listProviderLinks(): Promise<ProviderLink[]> {
    const result = isDesktop()
        ? await binding<ProviderLink[]>(() => AppBindings.ListProviderLinks())
        : unwrap(await get<ProviderLink[]>('/links'));
    return result ?? [];
}

export async function createProviderLink(sourceId: string, targetId: string): Promise<ProviderLink> {
    return isDesktop()
        ? binding(() => AppBindings.CreateProviderLink(sourceId, targetId))
        : unwrap(await post<ProviderLink, {sourceId: string; targetId: string}>('/links', {sourceId, targetId}));
}

export async function moveProviderLink(id: string, sourceId: string, targetId: string): Promise<ProviderLink> {
    return isDesktop()
        ? binding(() => AppBindings.MoveProviderLink(id, sourceId, targetId))
        : unwrap(await put<ProviderLink, {sourceId: string; targetId: string}>(`/links/${encodeURIComponent(id)}`, {sourceId, targetId}));
}

export async function applyProviderLink(id: string): Promise<ProviderLink> {
    return isDesktop()
        ? binding(() => AppBindings.ApplyProviderLink(id))
        : unwrap(await post<ProviderLink, unknown>(`/links/${encodeURIComponent(id)}/apply`, {}));
}

export async function deleteProviderLink(id: string): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.DeleteProviderLink(id));
    unwrap(await remove<void>(`/links/${encodeURIComponent(id)}`));
}
