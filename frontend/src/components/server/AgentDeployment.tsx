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

import {type FormEvent, useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Plus, RefreshCw, Rocket} from 'lucide-react';
import {toast} from 'sonner';
import {FormField} from '@/components/common/FormField';
import {Modal} from '@/components/common/Modal';
import {buttons, inputs} from '@/components/common/controls';
import {SSHPassphraseModal} from '@/components/server/SSHPassphraseModal';
import {SudoPasswordModal} from '@/components/server/SudoPasswordModal';
import {createAgentSource, deployAgent, getDeployStatus, listAgentSources, unlockServerSudo, type AgentSourceInput} from '@/services/deployment';
import {unlockServerSSH} from '@/services/servers';
import {SSHPassphraseRequiredError, SudoPasswordRequiredError} from '@/services/sshErrors';
import {cn} from '@/lib/utils';
import type {AgentSource, Server} from '@/types';

function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
}

function AgentSourceForm({onClose, onCreated}: {onClose: () => void; onCreated: (source: AgentSource) => void}) {
    const {t} = useTranslation();
    const [type, setType] = useState<'url' | 'path' | 'image'>('url');
    const [name, setName] = useState('');
    const [value, setValue] = useState('');
    const [cacheLocally, setCacheLocally] = useState(false);
    const [userspace, setUserspace] = useState(false);
    const [saving, setSaving] = useState(false);

    const save = async (event: FormEvent) => {
        event.preventDefault();
        setSaving(true);
        const input: AgentSourceInput = {
            name: name.trim(),
            url: type === 'url' ? value.trim() : '',
            path: type === 'path' ? value.trim() : '',
            image: type === 'image' ? value.trim() : '',
            cacheLocally: type === 'url' && cacheLocally,
            userspace: type !== 'image' && userspace,
        };
        try {
            onCreated(await createAgentSource(input));
        } catch (error) {
            toast.error(message(error) || t('servers.sourceCreateError'));
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal title={t('servers.addSourceTitle')} onClose={onClose} loading={saving} dimmed={false}>
            <form onSubmit={save} className="space-y-4">
                <FormField label={t('servers.sourceName')}>
                    <input autoFocus required value={name} onChange={event => setName(event.target.value)} placeholder={t('servers.sourceNamePlaceholder')} disabled={saving} className={inputs.primary} />
                </FormField>
                <FormField label={t('servers.sourceType')}>
                    <select value={type} onChange={event => {setType(event.target.value as typeof type); setValue('');}} disabled={saving} className={inputs.primary}>
                        <option value="url">{t('servers.sourceTypeUrl')}</option>
                        <option value="path">{t('servers.sourceTypePath')}</option>
                        <option value="image">{t('servers.sourceTypeImage')}</option>
                    </select>
                </FormField>
                <FormField label={type === 'url' ? 'URL' : type === 'path' ? t('servers.sourcePath') : t('servers.sourceImage')}>
                    <input required value={value} onChange={event => setValue(event.target.value)} disabled={saving} className={cn(inputs.primary, 'font-mono text-xs')} />
                </FormField>
                {type === 'url' && (
                    <label className="flex items-center gap-3 text-sm">
                        <input type="checkbox" checked={cacheLocally} onChange={event => setCacheLocally(event.target.checked)} disabled={saving} />
                        {t('servers.cacheLocally')}
                    </label>
                )}
                {type !== 'image' && (
                    <label className="flex items-center gap-3 text-sm">
                        <input type="checkbox" checked={userspace} onChange={event => setUserspace(event.target.checked)} disabled={saving} />
                        {t('servers.sourceUserspace')}
                    </label>
                )}
                <div className="flex justify-end gap-2">
                    <button type="button" onClick={onClose} disabled={saving} className={buttons.secondary}>{t('common.cancel')}</button>
                    <button type="submit" disabled={saving || !name.trim() || !value.trim()} className={buttons.primary}>{saving ? t('common.saving') : t('servers.addSource')}</button>
                </div>
            </form>
        </Modal>
    );
}

export function AgentDeployment({server, onChanged, onBusyChange}: {
    server: Server;
    onChanged: () => void;
    onBusyChange: (busy: boolean) => void;
}) {
    const {t} = useTranslation();
    const [sources, setSources] = useState<AgentSource[]>([]);
    const [sourceId, setSourceId] = useState('');
    const [addingSource, setAddingSource] = useState(false);
    const [starting, setStarting] = useState(false);
    const [deploying, setDeploying] = useState(false);
    const [step, setStep] = useState('');
    const [deployError, setDeployError] = useState('');
    const [unlock, setUnlock] = useState<'ssh' | 'sudo' | null>(null);
    const [unlocking, setUnlocking] = useState(false);
    const [unlockError, setUnlockError] = useState('');
    const busy = starting || deploying;

    useEffect(() => {
        onBusyChange(busy);
        return () => onBusyChange(false);
    }, [busy, onBusyChange]);

    useEffect(() => {
        let cancelled = false;
        void listAgentSources().then(items => {
            if (cancelled) return;
            setSources(items);
            setSourceId(current => items.some(item => item.id === current) ? current : (items[0]?.id ?? ''));
        }).catch(error => toast.error(message(error)));
        return () => {cancelled = true;};
    }, []);

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

    const start = useCallback(async () => {
        if (!sourceId) {
            toast.error(t('servers.noSourceSelected'));
            return;
        }
        setStarting(true);
        setDeployError('');
        setStep('connect');
        try {
            await deployAgent(server.id, sourceId);
            setDeploying(true);
        } catch (error) {
            handleError(error);
        } finally {
            setStarting(false);
        }
    }, [handleError, server.id, sourceId, t]);

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
                toast.success(t('servers.deployedSuccess'));
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
    }, [deploying, handleError, onChanged, server.id, t]);

    const submitUnlock = async (secret: string, applyToAll = false) => {
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
            await start();
        } catch (error) {
            setUnlockError(message(error));
        } finally {
            setUnlocking(false);
        }
    };

    return (
        <div className="rounded-xl border border-border bg-card p-6 dark:border-white/5 dark:bg-white/3">
            <div className="mb-1 flex items-center gap-2 text-lg font-semibold"><Rocket size={20} />{t('servers.deployAgentTitle')}</div>
            <p className="mb-5 text-sm text-muted-foreground">{t('servers.agentModal.deployHint')}</p>
            <div className="flex items-end gap-2">
                <div className="min-w-0 flex-1">
                    <FormField label={t('servers.agentSource')}>
                        <select value={sourceId} onChange={event => setSourceId(event.target.value)} disabled={busy} className={inputs.primary}>
                            {!sources.length && <option value="">{t('servers.selectSource')}</option>}
                            {sources.map(source => <option key={source.id} value={source.id}>{source.name}</option>)}
                        </select>
                    </FormField>
                </div>
                <button type="button" onClick={() => setAddingSource(true)} disabled={busy} title={t('servers.addSourceTitle')} className={cn(buttons.secondary, 'px-3')}><Plus size={16} /></button>
                <button type="button" onClick={() => void start()} disabled={busy || !sourceId} className={cn(buttons.primary, 'inline-flex items-center gap-2')}>
                    <Rocket size={14} />{busy ? t('servers.deploying') : t('servers.deploy')}
                </button>
            </div>
            {busy && step && <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground"><RefreshCw size={12} className="animate-spin" />{t(`servers.deploySteps.${step}`, step)}</div>}
            {deployError && !busy && <p className="mt-3 whitespace-pre-wrap break-words rounded-lg bg-red-500/10 p-3 text-xs text-red-600 dark:text-red-400">{deployError}</p>}
            {addingSource && <AgentSourceForm onClose={() => setAddingSource(false)} onCreated={source => {
                setSources(current => [...current, source]);
                setSourceId(source.id);
                setAddingSource(false);
            }} />}
            {unlock === 'ssh' && <SSHPassphraseModal onSubmit={(secret, applyToAll) => submitUnlock(secret, applyToAll)} onClose={() => setUnlock(null)} loading={unlocking} error={unlockError} />}
            {unlock === 'sudo' && <SudoPasswordModal onSubmit={secret => submitUnlock(secret)} onClose={() => setUnlock(null)} loading={unlocking} error={unlockError} />}
        </div>
    );
}
