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
import {get, post} from './api';
import {getCurrentApiMode} from './apiMode';
import {throwIfPassphraseRequired, throwIfSudoPasswordRequired} from './sshErrors';
import type {DeployStatus, ProviderDeployment} from '@/types';

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

export async function deployAgentPack(serverId: string, agentArtifact: string): Promise<void> {
    if (isDesktop()) {
        await binding(() => AppBindings.DeployAgentPack(serverId, agentArtifact));
        return;
    }
    unwrap(await post<void, {agentArtifact: string}>(
        `/servers/${encodeURIComponent(serverId)}/deploy/pack`,
        {agentArtifact},
    ));
}

export async function deployProvider(serverId: string, provider: ProviderDeployment): Promise<void> {
    if (isDesktop()) {
        await binding(() => AppBindings.DeployProvider(serverId, provider));
        return;
    }
    unwrap(await post<void, ProviderDeployment>(
        `/servers/${encodeURIComponent(serverId)}/deploy/provider`,
        provider,
    ));
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
