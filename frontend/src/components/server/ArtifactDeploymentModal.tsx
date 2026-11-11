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
import {RefreshCw} from 'lucide-react';
import {toast} from 'sonner';
import {FormField} from '@/components/common/FormField';
import {Modal} from '@/components/common/Modal';
import {buttons, inputs} from '@/components/common/controls';
import {listArtifactTags, listProviderRepositories, listRegistrySources} from '@/services/registrySources';
import type {AgentDeployment, ProviderDeployment, ProviderRepository, Registry, RegistrySource} from '@/types';

type Props = {
    onClose: () => void;
    busy: boolean;
} & (
    | {kind: 'agent'; onDeploy: (deployment: AgentDeployment) => void}
    | {kind: 'provider'; onDeploy: (provider: ProviderDeployment, registry: Registry) => void}
);

function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
}

export function ArtifactDeploymentModal(props: Props) {
    const {kind, busy, onClose} = props;
    const {t} = useTranslation();
    const [sources, setSources] = useState<RegistrySource[]>([]);
    const [sourceId, setSourceId] = useState('');
    const [repository, setRepository] = useState('');
    const [providers, setProviders] = useState<ProviderRepository[]>([]);
    const [providerName, setProviderName] = useState('');
    const [tags, setTags] = useState<string[]>([]);
    const [tag, setTag] = useState('');
    const [userspace, setUserspace] = useState(false);
    const [loading, setLoading] = useState(false);
    const [loadError, setLoadError] = useState('');
    const availableSources = sources.filter(source => source.type === kind);
    const selectedSource = sources.find(source => source.id === sourceId);
    const localBinary = selectedSource && !selectedSource.url.startsWith('oci://') ? selectedSource.url : '';
    const registry: Registry = sourceId && !localBinary ? {sourceId} : {};

    const loadTags = useCallback(async (nextRepository: string, nextRegistry: Registry) => {
        if (!nextRepository) return;
        setLoading(true);
        setLoadError('');
        try {
            const list = (await listArtifactTags(nextRepository, nextRegistry)).slice(0, 10);
            setTags(list);
            setTag(list[0] ?? '');
            if (list.length === 0) setLoadError(t('servers.noArtifactTags'));
        } catch (error) {
            setTags([]);
            setTag('');
            setLoadError(message(error));
        } finally {
            setLoading(false);
        }
    }, [t]);

    const loadProviders = useCallback(async (nextPrefix: string, nextRegistry: Registry) => {
        setLoading(true);
        setLoadError('');
        try {
            const list = await listProviderRepositories(nextPrefix, nextRegistry);
            setProviders(list);
            const selected = list[0];
            setProviderName(selected?.name ?? '');
            setRepository(selected?.repository ?? '');
            if (selected) {
                await loadTags(selected.repository, nextRegistry);
            } else {
                setTags([]);
                setTag('');
                setLoadError(t('servers.noProvidersFound'));
            }
        } catch (error) {
            setProviders([]);
            setProviderName('');
            setRepository('');
            setLoadError(message(error));
        } finally {
            setLoading(false);
        }
    }, [loadTags, t]);

    useEffect(() => {
        void listRegistrySources().then(setSources).catch(error => setLoadError(message(error)));
    }, []);

    const selectSource = (id: string) => {
        setSourceId(id);
        setRepository('');
        setProviders([]);
        setProviderName('');
        setTags([]);
        setTag('');
        setLoadError('');
        const source = sources.find(item => item.id === id);
        if (!source || !source.url.startsWith('oci://')) return;
        const nextRegistry = {sourceId: id};
        if (kind === 'agent') {
            setRepository(source.url);
            void loadTags(source.url, nextRegistry);
        } else {
            void loadProviders(source.url, nextRegistry);
        }
    };

    const selectProvider = (name: string) => {
        setProviderName(name);
        const selected = providers.find(item => item.name === name);
        if (!selected) return;
        setRepository(selected.repository);
        setTags([]);
        setTag('');
        void loadTags(selected.repository, registry);
    };

    const submit = () => {
        if (props.kind === 'agent') {
            if (localBinary) {
                props.onDeploy({binaryPath: localBinary});
                return;
            }
            if (!repository || !tag) {
                toast.error(t('servers.artifactRequired'));
                return;
            }
            props.onDeploy({artifact: `${repository}:${tag}`, registry});
            return;
        }
        if (!providerName || !repository || !tag) {
            toast.error(t('servers.providerSelectionRequired'));
            return;
        }
        props.onDeploy({
            name: providerName,
            artifact: `${repository}:${tag}`,
            options: providerName === 'awg' ? {userspace} : {},
        }, registry);
    };

    const ready = !!sourceId && (!!localBinary || !!repository && !!tag && (kind === 'agent' || !!providerName));

    return (
        <Modal title={t(kind === 'agent' ? 'servers.deployAgent' : 'servers.addProviderTitle')} onClose={onClose} loading={busy}>
            <div className="space-y-4">
                <FormField label={t(kind === 'agent' ? 'servers.agentSource' : 'servers.ociSource')}>
                    <>
                        <select value={sourceId} onChange={event => selectSource(event.target.value)} disabled={busy || loading} className={inputs.primary}>
                            <option value="">{t('servers.selectRegistry')}</option>
                            {availableSources.map(source => <option key={source.id} value={source.id}>{source.url}</option>)}
                        </select>
                        <p className="text-xs text-muted-foreground">{t('servers.registrySettingsHint')}</p>
                    </>
                </FormField>
                {localBinary ? (
                    <p className="break-all rounded-lg bg-muted p-3 font-mono text-xs">{localBinary}</p>
                ) : sourceId && (
                    <>
                        {kind === 'provider' && (
                            <FormField label={t('servers.providerName')}>
                                <select value={providerName} onChange={event => selectProvider(event.target.value)} disabled={busy || loading || providers.length === 0} className={inputs.primary}>
                                    {providers.length === 0 && <option value="">{loading ? t('common.loading') : t('servers.noProvidersFound')}</option>}
                                    {providers.map(provider => <option key={provider.name} value={provider.name}>{provider.name}</option>)}
                                </select>
                            </FormField>
                        )}
                        <FormField label={t('servers.version')}>
                            <div className="flex gap-2">
                                <select value={tag} onChange={event => setTag(event.target.value)} disabled={busy || loading || tags.length === 0} className={inputs.primary}>
                                    {tags.length === 0 && <option value="">{loading ? t('common.loading') : t('servers.noArtifactTags')}</option>}
                                    {tags.map(item => <option key={item} value={item}>{item}</option>)}
                                </select>
                                <button type="button" onClick={() => void loadTags(repository, registry)} disabled={busy || loading || !repository} className={buttons.secondary} title={t('common.refresh')}><RefreshCw size={14} className={loading ? 'animate-spin' : ''} /></button>
                            </div>
                        </FormField>
                        {kind === 'provider' && providerName === 'awg' && (
                            <label className="flex items-center gap-3 text-sm">
                                <input type="checkbox" checked={userspace} onChange={event => setUserspace(event.target.checked)} disabled={busy} />
                                {t('servers.forceUserspace')}
                            </label>
                        )}
                    </>
                )}
                {loadError && <p className="whitespace-pre-wrap break-words rounded-lg bg-amber-500/10 p-3 text-xs text-amber-600 dark:text-amber-400">{loadError}</p>}
                <div className="flex justify-end gap-2 pt-2">
                    <button type="button" onClick={onClose} disabled={busy} className={buttons.secondary}>{t('common.cancel')}</button>
                    <button type="button" onClick={submit} disabled={busy || loading || !ready} className={buttons.primary}>{t(kind === 'agent' ? 'servers.deployAgent' : 'servers.deployProvider')}</button>
                </div>
            </div>
        </Modal>
    );
}
