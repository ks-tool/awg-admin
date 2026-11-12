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
import {get, post, remove} from './api';
import {getCurrentApiMode} from './apiMode';
import {bindingsClient} from './bindingsClient';
import {throwIfPassphraseRequired, throwIfSudoPasswordRequired} from './sshErrors';
import type {AgentDeployment, DeployStatus, ProviderDeployment, Registry} from '@/types';

const isDesktop = () => getCurrentApiMode() === 'bindings';

function throwError(error: unknown): never {
    const message = error instanceof Error ? error.message : typeof error === 'string' ? error : 'Unknown error';
    throwIfPassphraseRequired(message);
    throwIfSudoPasswordRequired(message);
    throw new Error(message);
}

async function binding<T>(operation: () => Promise<T>): Promise<T> {
    try {
        return await operation();
    } catch (error) {
        throwError(error);
    }
}

function unwrap<T>(response: {data: T; error?: string}): T {
    if (response.error) throwError(response.error);
    return response.data;
}

export async function deployAgentPack(serverId: string, deployment: AgentDeployment): Promise<void> {
    if (isDesktop()) {
        await binding(() => AppBindings.DeployAgentPack(serverId, deployment as never));
        return;
    }
    unwrap(await post<void, AgentDeployment>(`/servers/${encodeURIComponent(serverId)}/deploy/pack`, deployment));
}

export async function deployProvider(serverId: string, provider: ProviderDeployment, registry: Registry): Promise<void> {
    if (isDesktop()) {
        await binding(() => AppBindings.DeployProvider(serverId, provider, registry as never));
        return;
    }
    unwrap(await post<void, ProviderDeployment & {registry: Registry}>(
        `/servers/${encodeURIComponent(serverId)}/deploy/provider`,
        {...provider, registry},
    ));
}

export async function removeProvider(serverId: string, provider: string): Promise<void> {
    if (isDesktop()) {
        await binding(() => AppBindings.RemoveProvider(serverId, provider));
        return;
    }
    unwrap(await remove<void>(`/servers/${encodeURIComponent(serverId)}/deploy/providers/${encodeURIComponent(provider)}`));
}

export async function getDeployStatus(serverId: string): Promise<DeployStatus> {
    return isDesktop()
        ? binding(async () => await AppBindings.GetDeployStatus(serverId) as unknown as DeployStatus)
        : unwrap(await get<DeployStatus>(`/servers/${encodeURIComponent(serverId)}/deploy/status`));
}

export async function unlockServerSudo(serverId: string, password: string): Promise<void> {
    const body = {password, applyToAll: false};
    if (isDesktop()) {
        await binding(() => AppBindings.UnlockServerSudo(serverId, password, false));
        return;
    }
    unwrap(await post<void, typeof body>(`/servers/${encodeURIComponent(serverId)}/sudo/unlock`, body));
}

export async function selectAgentFile(title: string): Promise<string> {
    if (!isDesktop()) return '';
    const response = await bindingsClient.selectFile(title);
    if (response.error) throw new Error(response.error);
    return response.data;
}
