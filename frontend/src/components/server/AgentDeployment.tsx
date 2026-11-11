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
import {FolderOpen, Minus, Pencil, Plus, RefreshCw, Rocket, Trash2} from 'lucide-react';
import {toast} from 'sonner';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs} from '@/components/common/controls';
import {ProviderDeploymentModal} from '@/components/server/ProviderDeploymentModal';
import {RegistrySourceModal} from '@/components/server/RegistrySourceModal';
import {SSHPassphraseModal} from '@/components/server/SSHPassphraseModal';
import {SudoPasswordModal} from '@/components/server/SudoPasswordModal';
import {
    deployAgentPack,
    deployProvider,
    getDeployStatus,
    removeProvider,
    selectAgentFile,
    unlockServerSudo,
} from '@/services/deployment';
import {deleteRegistrySource, listArtifactTags, listRegistrySources} from '@/services/registrySources';
import {getProviderManifest} from '@/services/resources';
import {unlockServerSSH} from '@/services/servers';
import {SSHPassphraseRequiredError, SudoPasswordRequiredError} from '@/services/sshErrors';
import {getCurrentApiMode} from '@/services/apiMode';
import {cn} from '@/lib/utils';
import type {
    AgentDeployment as AgentDeploymentRequest,
    ProviderDeployment,
    ProviderManifest,
    Registry,
    RegistrySource,
    Server,
} from '@/types';

const LOCAL_SOURCE = '__local__';

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
    const [sources, setSources] = useState<RegistrySource[]>([]);
    const [sourceId, setSourceId] = useState('');
    const [repository, setRepository] = useState('');
    const [tags, setTags] = useState<string[]>([]);
    const [tag, setTag] = useState('');
    const [binaryPath, setBinaryPath] = useState('');
    const [manifest, setManifest] = useState<ProviderManifest | null>(null);
    const [addingProvider, setAddingProvider] = useState(false);
    const [editingRegistry, setEditingRegistry] = useState<RegistrySource | undefined>();
    const [registryModalOpen, setRegistryModalOpen] = useState(false);
    const [deletingRegistry, setDeletingRegistry] = useState<RegistrySource | null>(null);
    const [removingProvider, setRemovingProvider] = useState<string | null>(null);
    const [request, setRequest] = useState<DeploymentRequest | null>(null);
    const [starting, setStarting] = useState(false);
    const [deploying, setDeploying] = useState(false);
    const [loadingTags, setLoadingTags] = useState(false);
    const [step, setStep] = useState('');
    const [deployError, setDeployError] = useState('');
    const [unlock, setUnlock] = useState<'ssh' | 'sudo' | null>(null);
    const [unlocking, setUnlocking] = useState(false);
    const [unlockError, setUnlockError] = useState('');
    const busy = starting || deploying;

    const loadManifest = useCallback(async () => {
        try {
            setManifest(await getProviderManifest(server.id));
        } catch {
            setManifest(null);
        }
    }, [server.id]);

    const loadTags = useCallback(async (nextRepository: string, registry: Registry) => {
        setLoadingTags(true);
        try {
            const list = await listArtifactTags(nextRepository, registry);
            setTags(list);
            setTag(current => list.includes(current) ? current : (list[0] ?? ''));
        } catch (error) {
            setTags([]);
            setTag('');
            toast.error(message(error));
        } finally {
            setLoadingTags(false);
        }
    }, []);

    useEffect(() => {
        void listRegistrySources().then(items => {
            setSources(items);
            const source = items[0];
            if (source) {
                const nextRepository = `${source.url.replace(/\/$/, '')}/ks-tool/vpn-agent`;
                setSourceId(source.id);
                setRepository(nextRepository);
                void loadTags(nextRepository, {sourceId: source.id});
            }
        }).catch(error => toast.error(message(error)));
        const timeout = window.setTimeout(() => void loadManifest(), 0);
        return () => window.clearTimeout(timeout);
    }, [loadManifest, loadTags]);

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
                await loadManifest();
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

    const selectSource = (id: string) => {
        setSourceId(id);
        setDeployError('');
        if (id === LOCAL_SOURCE) return;
        const source = sources.find(item => item.id === id);
        if (!source) {
            setRepository('');
            setTags([]);
            setTag('');
            return;
        }
        const nextRepository = `${source.url.replace(/\/$/, '')}/ks-tool/vpn-agent`;
        setRepository(nextRepository);
        void loadTags(nextRepository, {sourceId: id});
    };

    const deployAgent = () => {
        if (sourceId === LOCAL_SOURCE) {
            if (!binaryPath.trim()) {
                toast.error(t('servers.sourcePathRequired'));
                return;
            }
            void start({kind: 'agent', deployment: {binaryPath: binaryPath.trim()}});
            return;
        }
        if (!repository.trim() || !tag) {
            toast.error(t('servers.artifactRequired'));
            return;
        }
        void start({
            kind: 'agent',
            deployment: {
                artifact: `${repository.trim().replace(/\/$/, '')}:${tag}`,
                registry: sourceId ? {sourceId} : {},
            },
        });
    };

    const confirmDeleteRegistry = async () => {
        if (!deletingRegistry) return;
        try {
            await deleteRegistrySource(deletingRegistry.id);
            setSources(current => current.filter(item => item.id !== deletingRegistry.id));
            if (sourceId === deletingRegistry.id) selectSource('');
            setDeletingRegistry(null);
        } catch (error) {
            toast.error(message(error));
        }
    };

    const selectedRegistry = sources.find(source => source.id === sourceId);
    const agentReady = sourceId === LOCAL_SOURCE ? !!binaryPath.trim() : !!repository.trim() && !!tag;

    return (
        <div className="rounded-xl border border-border bg-card p-6 dark:border-white/5 dark:bg-white/3">
            <div className="mb-1 flex items-center gap-2 text-lg font-semibold"><Rocket size={20} />{t('servers.deployAgentTitle')}</div>
            <p className="mb-5 text-sm text-muted-foreground">{t('servers.agentModal.packDeployHint')}</p>
            <div className="space-y-4">
                <FormField label={t('servers.agentSource')}>
                    <div className="flex gap-2">
                        <select value={sourceId} onChange={event => {
                            if (event.target.value === '__add__') {
                                setEditingRegistry(undefined);
                                setRegistryModalOpen(true);
                                return;
                            }
                            selectSource(event.target.value);
                        }} disabled={busy} className={inputs.primary}>
                            <option value="">{t('servers.selectRegistry')}</option>
                            <option value={LOCAL_SOURCE}>{t('servers.localBinary')}</option>
                            {sources.map(source => <option key={source.id} value={source.id}>{source.name} — {source.url}</option>)}
                            <option value="__add__">{t('servers.addRegistry')}</option>
                        </select>
                        {selectedRegistry && (
                            <>
                                <button type="button" onClick={() => {
                                    setEditingRegistry(selectedRegistry);
                                    setRegistryModalOpen(true);
                                }} disabled={busy} className={buttons.secondary} title={t('servers.editSource')}><Pencil size={14} /></button>
                                <button type="button" onClick={() => setDeletingRegistry(selectedRegistry)} disabled={busy} className={buttons.secondary} title={t('common.delete')}><Trash2 size={14} /></button>
                            </>
                        )}
                    </div>
                </FormField>

                {sourceId === LOCAL_SOURCE ? (
                    <FormField label={t('servers.agentBinaryPath')}>
                        <div className="flex gap-2">
                            <input value={binaryPath} onChange={event => setBinaryPath(event.target.value)} disabled={busy} className={cn(inputs.primary, 'font-mono text-xs')} placeholder="/usr/local/bin/vpn-agent" />
                            {getCurrentApiMode() === 'bindings' && (
                                <button type="button" onClick={() => void selectAgentFile(t('servers.selectAgentBinaryDialog')).then(path => {
                                    if (path) setBinaryPath(path);
                                })} disabled={busy} className={buttons.secondary}><FolderOpen size={14} />{t('common.browse')}</button>
                            )}
                        </div>
                    </FormField>
                ) : (
                    <>
                        {sourceId && (
                            <FormField label={t('servers.agentRepository')}>
                                <input value={repository} onChange={event => setRepository(event.target.value)} disabled={busy || loadingTags} className={cn(inputs.primary, 'font-mono text-xs')} />
                            </FormField>
                        )}
                        <FormField label={t('servers.version')}>
                            <div className="flex gap-2">
                                <select value={tag} onChange={event => setTag(event.target.value)} disabled={busy || loadingTags || tags.length === 0} className={inputs.primary}>
                                    {tags.length === 0 && <option value="">{loadingTags ? t('common.loading') : t('servers.noArtifactTags')}</option>}
                                    {tags.map(item => <option key={item} value={item}>{item}</option>)}
                                </select>
                                <button type="button" onClick={() => void loadTags(repository, sourceId ? {sourceId} : {})} disabled={busy || loadingTags || !repository.trim()} className={buttons.secondary} title={t('common.refresh')}><RefreshCw size={14} className={loadingTags ? 'animate-spin' : ''} /></button>
                            </div>
                        </FormField>
                    </>
                )}
                <div className="flex justify-end">
                    <button type="button" onClick={deployAgent} disabled={busy || !agentReady} className={cn(buttons.primary, 'inline-flex items-center gap-2')}>
                        <Rocket size={14} />{busy && request?.kind === 'agent' ? t('servers.deploying') : t('servers.deployAgent')}
                    </button>
                </div>
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

            {busy && step && <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground"><RefreshCw size={12} className="animate-spin" />{t(`servers.deploySteps.${step}`, step)}</div>}
            {deployError && !busy && <p className="mt-3 whitespace-pre-wrap break-words rounded-lg bg-red-500/10 p-3 text-xs text-red-600 dark:text-red-400">{deployError}</p>}

            {addingProvider && (
                <ProviderDeploymentModal busy={busy} onClose={() => setAddingProvider(false)} onDeploy={(provider, registry) => {
                    setAddingProvider(false);
                    void start({kind: 'provider', provider, registry});
                }} />
            )}
            {registryModalOpen && (
                <RegistrySourceModal source={editingRegistry} onClose={() => setRegistryModalOpen(false)} onSaved={source => {
                    setRegistryModalOpen(false);
                    setSources(current => [...current.filter(item => item.id !== source.id), source].sort((a, b) => a.name.localeCompare(b.name)));
                    if (!editingRegistry) {
                        const nextRepository = `${source.url.replace(/\/$/, '')}/ks-tool/vpn-agent`;
                        setSourceId(source.id);
                        setRepository(nextRepository);
                        void loadTags(nextRepository, {sourceId: source.id});
                    }
                }} />
            )}
            {deletingRegistry && (
                <ConfirmModal title={t('servers.registrySources')} message={t('servers.confirmDeleteRegistry')} onConfirm={() => void confirmDeleteRegistry()} onCancel={() => setDeletingRegistry(null)} />
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
