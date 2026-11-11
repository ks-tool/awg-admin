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
import {Pencil, Plus, Trash2} from 'lucide-react';
import {toast} from 'sonner';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {buttons} from '@/components/common/controls';
import {RegistrySourceModal} from '@/components/server/RegistrySourceModal';
import {deleteRegistrySource, listRegistrySources} from '@/services/registrySources';
import {cn} from '@/lib/utils';
import type {RegistrySource} from '@/types';

function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
}

export function RegistrySourcesSection() {
    const {t} = useTranslation();
    const [sources, setSources] = useState<RegistrySource[]>([]);
    const [loaded, setLoaded] = useState(false);
    const [editing, setEditing] = useState<RegistrySource | undefined>();
    const [modalOpen, setModalOpen] = useState(false);
    const [deleting, setDeleting] = useState<RegistrySource | null>(null);

    const load = useCallback(async () => {
        try {
            setSources(await listRegistrySources());
        } catch (error) {
            toast.error(message(error));
        } finally {
            setLoaded(true);
        }
    }, []);

    useEffect(() => {
        const timeout = window.setTimeout(() => void load(), 0);
        return () => window.clearTimeout(timeout);
    }, [load]);

    const remove = async () => {
        if (!deleting) return;
        try {
            await deleteRegistrySource(deleting.id);
            setSources(current => current.filter(source => source.id !== deleting.id));
            setDeleting(null);
        } catch (error) {
            toast.error(message(error));
        }
    };

    return (
        <div className="space-y-4 rounded-lg border border-border bg-card p-4">
            <div className="flex items-center justify-between gap-4">
                <div>
                    <h2 className="text-sm font-semibold text-foreground">{t('settings.registrySources')}</h2>
                    <p className="mt-1 text-xs text-muted-foreground">{t('settings.registrySourcesHint')}</p>
                </div>
                <button type="button" onClick={() => {
                    setEditing(undefined);
                    setModalOpen(true);
                }} className={cn(buttons.primary, 'inline-flex shrink-0 items-center gap-2')}><Plus size={14} />{t('settings.addRegistrySource')}</button>
            </div>
            {loaded && sources.length === 0 && <p className="text-sm text-muted-foreground">{t('settings.noRegistrySources')}</p>}
            <div className="space-y-2">
                {sources.map(source => (
                    <div key={source.id} className="flex items-start justify-between gap-3 rounded-lg border border-border bg-background p-3 dark:border-white/10 dark:bg-white/5">
                        <div className="min-w-0 space-y-1">
                            <div className="text-sm font-medium">{source.name}<span className="ml-2 font-mono text-xs font-normal text-muted-foreground">{source.url}</span></div>
                            <div className="break-all font-mono text-xs text-muted-foreground">{t('settings.agentArtifacts')}: {source.agentRepository}</div>
                            <div className="break-all font-mono text-xs text-muted-foreground">{t('settings.providerArtifacts')}: {source.providerRepository}</div>
                        </div>
                        <div className="flex shrink-0 gap-2">
                            <button type="button" onClick={() => {
                                setEditing(source);
                                setModalOpen(true);
                            }} className={cn(buttons.secondary, 'px-3')} title={t('servers.editSource')}><Pencil size={14} /></button>
                            <button type="button" onClick={() => setDeleting(source)} className={cn(buttons.secondary, 'px-3')} title={t('common.delete')}><Trash2 size={14} /></button>
                        </div>
                    </div>
                ))}
            </div>
            {modalOpen && <RegistrySourceModal source={editing} onClose={() => setModalOpen(false)} onSaved={source => {
                setModalOpen(false);
                setSources(current => [...current.filter(item => item.id !== source.id), source].sort((a, b) => a.name.localeCompare(b.name)));
            }} />}
            {deleting && <ConfirmModal title={t('servers.registrySources')} message={t('servers.confirmDeleteRegistry')} onConfirm={() => void remove()} onCancel={() => setDeleting(null)} />}
        </div>
    );
}
