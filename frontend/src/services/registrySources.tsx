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
import type {ProviderRepository, Registry, RegistrySource} from '@/types';

const isDesktop = () => getCurrentApiMode() === 'bindings';

function unwrap<T>(response: {data: T; error?: string}): T {
    if (response.error) throw new Error(response.error);
    return response.data;
}

async function binding<T>(operation: () => Promise<T>): Promise<T> {
    try {
        return await operation();
    } catch (error) {
        throw new Error(error instanceof Error ? error.message : typeof error === 'string' ? error : 'Unknown error');
    }
}

export async function listRegistrySources(): Promise<RegistrySource[]> {
    const result = isDesktop()
        ? await binding(async () => await AppBindings.ListRegistrySources() as unknown as RegistrySource[])
        : unwrap(await get<RegistrySource[]>('/registry-sources/'));
    return result ?? [];
}

export async function createRegistrySource(input: Omit<RegistrySource, 'id' | 'hasPassword'>): Promise<RegistrySource> {
    if (isDesktop()) {
        return binding(async () => await AppBindings.CreateRegistrySource(input.name, input.url, input.agentRepository, input.providerRepository, input.username ?? '', input.password ?? '', input.plainHttp ?? false) as unknown as RegistrySource);
    }
    return unwrap(await post<RegistrySource, typeof input>('/registry-sources', input));
}

export async function updateRegistrySource(id: string, input: Omit<RegistrySource, 'id' | 'hasPassword'>): Promise<RegistrySource> {
    if (isDesktop()) {
        return binding(async () => await AppBindings.UpdateRegistrySource(id, input.name, input.url, input.agentRepository, input.providerRepository, input.username ?? '', input.password ?? '', input.plainHttp ?? false) as unknown as RegistrySource);
    }
    return unwrap(await put<RegistrySource, typeof input>(`/registry-sources/${encodeURIComponent(id)}`, input));
}

export async function deleteRegistrySource(id: string): Promise<void> {
    if (isDesktop()) return binding(() => AppBindings.DeleteRegistrySource(id));
    unwrap(await remove<void>(`/registry-sources/${encodeURIComponent(id)}`));
}

export async function listArtifactTags(repository: string, registry: Registry): Promise<string[]> {
    const result = isDesktop()
        ? await binding(() => AppBindings.ListArtifactTags(repository, registry as never))
        : unwrap(await post<string[], {repository: string; registry: Registry}>('/artifacts/tags', {repository, registry}));
    return result ?? [];
}

export async function listProviderRepositories(repository: string, registry: Registry): Promise<ProviderRepository[]> {
    const result = isDesktop()
        ? await binding(async () => await AppBindings.ListProviderRepositories(repository, registry as never) as unknown as ProviderRepository[])
        : unwrap(await post<ProviderRepository[], {repository: string; registry: Registry}>('/artifacts/providers', {repository, registry}));
    return result ?? [];
}
