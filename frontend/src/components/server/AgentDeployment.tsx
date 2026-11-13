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

import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Minus, Plus, RefreshCw, Rocket} from 'lucide-react';
import {toast} from 'sonner';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {buttons} from '@/components/common/controls';
import {ArtifactDeploymentModal} from '@/components/server/ArtifactDeploymentModal';
import {SSHPassphraseModal} from '@/components/server/SSHPassphraseModal';
import {SudoPasswordModal} from '@/components/server/SudoPasswordModal';
import {deployAgentPack, deployProvider, getDeployStatus, removeProvider, unlockServerSudo} from '@/services/deployment';
import {getProviderManifest} from '@/services/resources';
import {unlockServerSSH} from '@/services/servers';
import {SSHPassphraseRequiredError, SudoPasswordRequiredError} from '@/services/sshErrors';
import {cn} from '@/lib/utils';
import type {AgentDeployment as AgentDeploymentRequest, ProviderDeployment, ProviderManifest, Registry, Server} from '@/types';

type DeploymentRequest =
    | {kind: 'agent'; deployment: AgentDeploymentRequest}
    | {kind: 'provider'; provider: ProviderDeployment; registry: Registry}
    | {kind: 'remove'; name: string};

function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
}

export function AgentDeployment({server, onChanged, onBusyChange}: {
    server: Server;
    onChanged: () => void;
    onBusyChange: (busy: boolean) => void;
}) {
    const {t} = useTranslation();
    const [manifest, setManifest] = useState<ProviderManifest | null>(null);
    const [addingAgent, setAddingAgent] = useState(false);
    const [addingProvider, setAddingProvider] = useState(false);
    const [removingProvider, setRemovingProvider] = useState<string | null>(null);
    const [request, setRequest] = useState<DeploymentRequest | null>(null);
    const [starting, setStarting] = useState(false);
    const [deploying, setDeploying] = useState(false);
    const [step, setStep] = useState('');
    const [deployError, setDeployError] = useState('');
    const [unlock, setUnlock] = useState<'ssh' | 'sudo' | null>(null);
    const [unlocking, setUnlocking] = useState(false);
    const [unlockError, setUnlockError] = useState('');
    const busy = starting || deploying;
    const providerStep = /^(install|remove)_provider_(.+)$/.exec(step);
    const stepText = providerStep
        ? t(`servers.deploySteps.${providerStep[1]}_provider`, {name: providerStep[2]})
        : t(`servers.deploySteps.${step}`, step);

    const loadManifest = useCallback(async (refresh = false) => {
        try {
            setManifest(await getProviderManifest(server.id, refresh));
        } catch {
            setManifest(null);
        }
    }, [server.id]);

    useEffect(() => {
        const timeout = window.setTimeout(() => void loadManifest(), 0);
        return () => window.clearTimeout(timeout);
    }, [loadManifest]);

    useEffect(() => {
        onBusyChange(busy);
        return () => onBusyChange(false);
    }, [busy, onBusyChange]);

    const handleError = useCallback((error: unknown) => {
        setStarting(false);
        setDeploying(false);
        if (error instanceof SSHPassphraseRequiredError) {
            setUnlock('ssh');
            return;
        }
        if (error instanceof SudoPasswordRequiredError) {
            setUnlock('sudo');
            return;
        }
        const text = message(error);
        setDeployError(text);
        toast.error(text || t('servers.deployError'));
    }, [t]);

    const start = useCallback(async (nextRequest: DeploymentRequest) => {
        setRequest(nextRequest);
        setStarting(true);
        setDeployError('');
        setStep('connect');
        try {
            if (nextRequest.kind === 'agent') {
                await deployAgentPack(server.id, nextRequest.deployment);
            } else if (nextRequest.kind === 'provider') {
                await deployProvider(server.id, nextRequest.provider, nextRequest.registry);
            } else {
                await removeProvider(server.id, nextRequest.name);
            }
            setDeploying(true);
        } catch (error) {
            handleError(error);
        } finally {
            setStarting(false);
        }
    }, [handleError, server.id]);

    useEffect(() => {
        if (!deploying) return;
        let cancelled = false;
        const poll = async () => {
            try {
                const status = await getDeployStatus(server.id);
                if (cancelled) return;
                setStep(status.step);
                if (!status.done) return;
                setDeploying(false);
                if (status.error) {
                    setDeployError(status.error);
                    toast.error(status.error, {duration: 10000});
                    return;
                }
                toast.success(request?.kind === 'remove' ? t('servers.providerRemovedSuccess') : t('servers.deployedSuccess'));
                await loadManifest(true);
                onChanged();
            } catch (error) {
                if (!cancelled) handleError(error);
            }
        };
        void poll();
        const interval = window.setInterval(() => void poll(), 1000);
        return () => {
            cancelled = true;
            window.clearInterval(interval);
        };
    }, [deploying, handleError, loadManifest, onChanged, request?.kind, server.id, t]);

    const submitUnlock = async (secret: string, applyToAll = false) => {
        if (!request) return;
        setUnlocking(true);
        setUnlockError('');
        try {
            if (unlock === 'ssh') {
                if (!await unlockServerSSH(server.id, secret, applyToAll)) {
                    setUnlockError(t('auth.unlockError'));
                    return;
                }
            } else {
                await unlockServerSudo(server.id, secret);
            }
            setUnlock(null);
            await start(request);
        } catch (error) {
            setUnlockError(message(error));
        } finally {
            setUnlocking(false);
        }
    };

    return (
        <div className="rounded-xl border border-border bg-card p-6 dark:border-white/5 dark:bg-white/3">
            <div className="mb-1 flex items-center gap-2 text-lg font-semibold"><Rocket size={20} />{t('servers.deployAgentTitle')}</div>
            <p className="mb-5 text-sm text-muted-foreground">{t('servers.agentModal.packDeployHint')}</p>
            <div className="flex justify-end">
                <button type="button" onClick={() => setAddingAgent(true)} disabled={busy} className={cn(buttons.primary, 'inline-flex items-center gap-2')}>
                    <Rocket size={14} />{busy && request?.kind === 'agent' ? t('servers.deploying') : t('servers.deployAgent')}
                </button>
            </div>

            <div className="mt-6 rounded-lg border border-border p-4 dark:border-white/10">
                <div className="mb-3 flex items-center justify-between">
                    <h3 className="text-sm font-semibold">{t('servers.installedProviders')}</h3>
                    <button type="button" onClick={() => setAddingProvider(true)} disabled={busy} className={cn(buttons.primary, 'inline-flex items-center gap-1')} title={t('servers.addProvider')}><Plus size={14} />{t('servers.addProvider')}</button>
                </div>
                {manifest?.providers.length ? (
                    <div className="space-y-2">
                        {manifest.providers.map(provider => (
                            <div key={provider.name} className="flex items-center justify-between rounded-lg border border-border bg-background px-3 py-2 dark:border-white/10 dark:bg-white/5">
                                <div><span className="text-sm font-medium">{provider.name}</span><span className="ml-2 font-mono text-xs text-muted-foreground">{provider.version}</span></div>
                                <button type="button" onClick={() => setRemovingProvider(provider.name)} disabled={busy} className={cn(buttons.secondary, 'inline-flex items-center gap-1 px-3')} title={t('servers.removeProvider')}><Minus size={14} />{t('servers.removeProvider')}</button>
                            </div>
                        ))}
                    </div>
                ) : (
                    <p className="text-sm text-muted-foreground">{t('servers.noInstalledProviders')}</p>
                )}
            </div>

            {busy && step && <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground"><RefreshCw size={12} className="animate-spin" />{stepText}</div>}
            {deployError && !busy && <p className="mt-3 whitespace-pre-wrap break-words rounded-lg bg-red-500/10 p-3 text-xs text-red-600 dark:text-red-400">{deployError}</p>}

            {addingAgent && (
                <ArtifactDeploymentModal kind="agent" server={server} busy={busy} onClose={() => setAddingAgent(false)} onDeploy={deployment => {
                    setAddingAgent(false);
                    void start({kind: 'agent', deployment});
                }} />
            )}
            {addingProvider && (
                <ArtifactDeploymentModal kind="provider" busy={busy} onClose={() => setAddingProvider(false)} onDeploy={(provider, registry) => {
                    setAddingProvider(false);
                    void start({kind: 'provider', provider, registry});
                }} />
            )}
            {removingProvider && (
                <ConfirmModal title={t('servers.removeProvider')} message={t('servers.confirmRemoveProvider', {name: removingProvider})} onConfirm={() => {
                    const name = removingProvider;
                    setRemovingProvider(null);
                    void start({kind: 'remove', name});
                }} onCancel={() => setRemovingProvider(null)} />
            )}
            {unlock === 'ssh' && <SSHPassphraseModal onSubmit={(secret, applyToAll) => submitUnlock(secret, applyToAll)} onClose={() => setUnlock(null)} loading={unlocking} error={unlockError} />}
            {unlock === 'sudo' && <SudoPasswordModal onSubmit={secret => submitUnlock(secret)} onClose={() => setUnlock(null)} loading={unlocking} error={unlockError} />}
        </div>
    );
}
