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
import {RegistrySourceModal} from '@/components/server/RegistrySourceModal';
import {listArtifactTags, listProviderRepositories, listRegistrySources} from '@/services/registrySources';
import {cn} from '@/lib/utils';
import type {ProviderDeployment, ProviderRepository, Registry, RegistrySource} from '@/types';

function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
}

export function ProviderDeploymentModal({onClose, onDeploy, busy}: {
    onClose: () => void;
    onDeploy: (provider: ProviderDeployment, registry: Registry) => void;
    busy: boolean;
}) {
    const {t} = useTranslation();
    const [sources, setSources] = useState<RegistrySource[]>([]);
    const [sourceId, setSourceId] = useState('');
    const [prefix, setPrefix] = useState('');
    const [providers, setProviders] = useState<ProviderRepository[]>([]);
    const [providerName, setProviderName] = useState('');
    const [repository, setRepository] = useState('');
    const [tags, setTags] = useState<string[]>([]);
    const [tag, setTag] = useState('');
    const [userspace, setUserspace] = useState(false);
    const [loading, setLoading] = useState(false);
    const [loadError, setLoadError] = useState('');
    const [addingRegistry, setAddingRegistry] = useState(false);

    const registry: Registry = sourceId ? {sourceId} : {};

    const loadTags = useCallback(async (nextRepository: string, nextRegistry: Registry) => {
        if (!nextRepository.trim()) return;
        setLoading(true);
        setLoadError('');
        try {
            const list = await listArtifactTags(nextRepository.trim(), nextRegistry);
            setTags(list);
            setTag(current => list.includes(current) ? current : (list[0] ?? ''));
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
            if (selected) await loadTags(selected.repository, nextRegistry);
        } catch (error) {
            setProviders([]);
            setLoadError(message(error));
        } finally {
            setLoading(false);
        }
    }, [loadTags]);

    useEffect(() => {
        void listRegistrySources().then(items => {
            setSources(items);
            const source = items[0];
            if (source) {
                const nextPrefix = `${source.url.replace(/\/$/, '')}/vpn-providers`;
                setSourceId(source.id);
                setPrefix(nextPrefix);
                void loadProviders(nextPrefix, {sourceId: source.id});
            }
        }).catch(error => toast.error(message(error)));
    }, [loadProviders]);

    const selectSource = (id: string) => {
        setSourceId(id);
        setProviders([]);
        setRepository('');
        setTags([]);
        const source = sources.find(item => item.id === id);
        if (!source) {
            setPrefix('');
            return;
        }
        const nextPrefix = `${source.url.replace(/\/$/, '')}/vpn-providers`;
        setPrefix(nextPrefix);
        void loadProviders(nextPrefix, {sourceId: id});
    };

    const selectProvider = (name: string) => {
        setProviderName(name);
        const selected = providers.find(item => item.name === name);
        const nextRepository = selected?.repository ?? `${prefix.replace(/\/$/, '')}/${name}`;
        setRepository(nextRepository);
        void loadTags(nextRepository, registry);
    };

    const updateRepository = (value: string) => {
        setRepository(value);
        setProviderName(value.trim().replace(/\/$/, '').split('/').pop() ?? '');
    };

    const submit = () => {
        if (!providerName.trim() || !repository.trim() || !tag) {
            toast.error(t('servers.providerSelectionRequired'));
            return;
        }
        onDeploy({
            name: providerName.trim(),
            artifact: `${repository.trim().replace(/\/$/, '')}:${tag}`,
            options: providerName === 'awg' ? {userspace} : {},
        }, registry);
    };

    return (
        <Modal title={t('servers.addProviderTitle')} onClose={onClose} loading={busy} dimmed={!addingRegistry}>
            <div className="space-y-4">
                <FormField label={t('servers.ociSource')}>
                    <select value={sourceId} onChange={event => {
                        if (event.target.value === '__add__') {
                            setAddingRegistry(true);
                            return;
                        }
                        selectSource(event.target.value);
                    }} disabled={busy || loading} className={inputs.primary}>
                        <option value="">{t('servers.selectRegistry')}</option>
                        {sources.map(source => <option key={source.id} value={source.id}>{source.name} — {source.url}</option>)}
                        <option value="__add__">{t('servers.addRegistry')}</option>
                    </select>
                </FormField>
                {sourceId && (
                    <FormField label={t('servers.providerCatalog')}>
                        <div className="flex gap-2">
                            <input value={prefix} onChange={event => setPrefix(event.target.value)} disabled={busy || loading} className={cn(inputs.primary, 'font-mono text-xs')} />
                            <button type="button" onClick={() => void loadProviders(prefix, registry)} disabled={busy || loading || !prefix.trim()} className={buttons.secondary} title={t('common.refresh')}><RefreshCw size={14} className={loading ? 'animate-spin' : ''} /></button>
                        </div>
                    </FormField>
                )}
                <FormField label={t('servers.providerName')}>
                    {providers.length > 0 ? (
                        <select value={providerName} onChange={event => selectProvider(event.target.value)} disabled={busy || loading} className={inputs.primary}>
                            {providers.map(provider => <option key={provider.name} value={provider.name}>{provider.name}</option>)}
                        </select>
                    ) : (
                        <input value={providerName} onChange={event => {
                            const name = event.target.value;
                            setProviderName(name);
                            setRepository(`${prefix.replace(/\/$/, '')}/${name}`);
                        }} disabled={busy || loading} className={inputs.primary} placeholder="awg" />
                    )}
                </FormField>
                <FormField label={t('servers.providerRepository')}>
                    <input value={repository} onChange={event => updateRepository(event.target.value)} disabled={busy || loading} className={cn(inputs.primary, 'font-mono text-xs')} />
                </FormField>
                <FormField label={t('servers.version')}>
                    <div className="flex gap-2">
                        <select value={tag} onChange={event => setTag(event.target.value)} disabled={busy || loading || tags.length === 0} className={inputs.primary}>
                            {tags.length === 0 && <option value="">{t('servers.noArtifactTags')}</option>}
                            {tags.map(item => <option key={item} value={item}>{item}</option>)}
                        </select>
                        <button type="button" onClick={() => void loadTags(repository, registry)} disabled={busy || loading || !repository.trim()} className={buttons.secondary} title={t('common.refresh')}><RefreshCw size={14} className={loading ? 'animate-spin' : ''} /></button>
                    </div>
                </FormField>
                {providerName === 'awg' && (
                    <label className="flex items-center gap-3 text-sm">
                        <input type="checkbox" checked={userspace} onChange={event => setUserspace(event.target.checked)} disabled={busy} />
                        {t('servers.forceUserspace')}
                    </label>
                )}
                {loadError && <p className="whitespace-pre-wrap break-words rounded-lg bg-amber-500/10 p-3 text-xs text-amber-600 dark:text-amber-400">{loadError}</p>}
                <div className="flex justify-end gap-2 pt-2">
                    <button type="button" onClick={onClose} disabled={busy} className={buttons.secondary}>{t('common.cancel')}</button>
                    <button type="button" onClick={submit} disabled={busy || loading || !providerName || !repository || !tag} className={buttons.primary}>{t('servers.deployProvider')}</button>
                </div>
            </div>
            {addingRegistry && (
                <RegistrySourceModal onClose={() => setAddingRegistry(false)} onSaved={source => {
                    setAddingRegistry(false);
                    setSources(current => [...current, source]);
                    const nextPrefix = `${source.url.replace(/\/$/, '')}/vpn-providers`;
                    setSourceId(source.id);
                    setPrefix(nextPrefix);
                    setProviders([]);
                    setRepository('');
                    setTags([]);
                    void loadProviders(nextPrefix, {sourceId: source.id});
                }} />
            )}
        </Modal>
    );
}
