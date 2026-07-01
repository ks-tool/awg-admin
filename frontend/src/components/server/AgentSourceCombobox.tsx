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

import {useCallback, useEffect, useRef, useState} from 'react';
import {createPortal} from 'react-dom';
import {useTranslation} from 'react-i18next';
import {toast} from 'sonner';
import {ChevronDown, FolderOpen, RefreshCw, X} from 'lucide-react';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs} from '@/components/common/Modal';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {cn} from '@/lib/utils';
import {createAgentSource, deleteAgentSource, listAgentSources, refreshAgentSourceCache, selectAgentFile} from '@/services/agentSources';
import {getCurrentApiMode} from '@/services/apiMode';
import type {AgentSource} from '@/types';

interface Props {
    /** Selected AgentSource ID, or '' if nothing is selected yet. */
    value: string;
    onChange: (id: string) => void;
    disabled?: boolean;
}

/**
 * Dropdown of saved agent-binary deploy presets (see models.AgentSource),
 * replacing a plain "Local directory"/"URL" choice: each preset is either a
 * named URL — optionally cached locally by awg-admin (prefixed with "*" in
 * the list) instead of having the managed server download it itself — or a
 * named local file path on awg-admin's own filesystem (prefixed with "↳").
 * Picking "Добавить новый…"/"Add new…" reveals inline fields to save a new
 * one; each existing entry has a remove button on the right, like clearing
 * single entries from Chrome's address-bar history.
 */
export function AgentSourceCombobox({value, onChange, disabled = false}: Props) {
    const {t} = useTranslation();
    const [sources, setSources] = useState<AgentSource[]>([]);
    const [open, setOpen] = useState(false);
    const [adding, setAdding] = useState(false);
    const [sourceType, setSourceType] = useState<'url' | 'path'>('url');
    const [name, setName] = useState('');
    const [url, setUrl] = useState('');
    const [path, setPath] = useState('');
    const [cacheLocally, setCacheLocally] = useState(false);
    const [saving, setSaving] = useState(false);
    const [refreshingId, setRefreshingId] = useState<string | null>(null);
    const [removeConfirmId, setRemoveConfirmId] = useState<string | null>(null);
    const [removeLoading, setRemoveLoading] = useState(false);
    const [refreshConfirmId, setRefreshConfirmId] = useState<string | null>(null);
    const rootRef = useRef<HTMLDivElement>(null);
    const triggerRef = useRef<HTMLButtonElement>(null);
    const menuRef = useRef<HTMLDivElement>(null);
    // Fixed-viewport position of the open list. The list is portaled to
    // document.body so it floats above the modal instead of being clipped by
    // its overflow-y-auto — and, unlike an in-flow list, it doesn't change the
    // modal's size when it opens.
    const [menuPos, setMenuPos] = useState<{top: number; left: number; width: number; maxHeight: number} | null>(null);

    const updateMenuPos = useCallback(() => {
        const el = triggerRef.current;
        if (!el) return;
        const r = el.getBoundingClientRect();
        const gap = 4;
        const margin = 8;
        const desired = 256; // matches the old max-h-64
        const spaceBelow = window.innerHeight - r.bottom - margin;
        const spaceAbove = r.top - margin;
        // Prefer opening downward; flip up only when there's clearly more room.
        const openUp = spaceBelow < desired && spaceAbove > spaceBelow;
        const maxHeight = Math.max(0, Math.min(desired, openUp ? spaceAbove : spaceBelow));
        setMenuPos({
            top: openUp ? r.top - gap - maxHeight : r.bottom + gap,
            left: r.left,
            width: r.width,
            maxHeight,
        });
    }, []);

    // The native file picker only exists in the Wails desktop app; a browser
    // can't resolve a real filesystem path, so the Browse button is desktop-only.
    const isDesktop = getCurrentApiMode() === 'bindings';

    const handleBrowse = async () => {
        const picked = await selectAgentFile(t('servers.selectAgentBinaryDialog'));
        if (picked) setPath(picked);
    };

    const load = useCallback(async () => {
        const list = await listAgentSources();
        if (list) setSources(list);
    }, []);

    useEffect(() => {
        void load();
    }, [load]);

    useEffect(() => {
        if (!open) return;
        // The list is portaled outside rootRef, so a click inside it must not
        // count as "outside" — check the menu element too.
        const handleClickOutside = (e: MouseEvent) => {
            const target = e.target as Node;
            if (rootRef.current?.contains(target) || menuRef.current?.contains(target)) return;
            setOpen(false);
        };
        document.addEventListener('mousedown', handleClickOutside);
        return () => document.removeEventListener('mousedown', handleClickOutside);
    }, [open]);

    // Keep the portaled list anchored to the trigger while it's open: recompute
    // on open and whenever the viewport or any scroll container moves it.
    useEffect(() => {
        if (!open) return;
        updateMenuPos();
        const onReflow = () => updateMenuPos();
        window.addEventListener('scroll', onReflow, true); // capture: catch scrolling in the modal body too
        window.addEventListener('resize', onReflow);
        return () => {
            window.removeEventListener('scroll', onReflow, true);
            window.removeEventListener('resize', onReflow);
        };
    }, [open, updateMenuPos]);

    const selected = sources.find(s => s.id === value);

    const handleSelect = (id: string) => {
        onChange(id);
        setOpen(false);
    };

    const handleAdd = async () => {
        if (!name.trim()) return toast.error(t('servers.sourceNameRequired'));
        if (sourceType === 'url' && !url.trim()) return toast.error(t('servers.sourceUrlRequired'));
        if (sourceType === 'path' && !path.trim()) return toast.error(t('servers.sourcePathRequired'));

        setSaving(true);
        try {
            const src = await createAgentSource(
                name.trim(),
                sourceType === 'url' ? url.trim() : '',
                sourceType === 'path' ? path.trim() : '',
                cacheLocally,
            );
            if (src) {
                await load();
                onChange(src.id);
                setAdding(false);
                setName('');
                setUrl('');
                setPath('');
                setCacheLocally(false);
                setSourceType('url');
            } else {
                toast.error(t('servers.sourceCreateError'));
            }
        } finally {
            setSaving(false);
        }
    };

    const handleRemove = (id: string, e: React.MouseEvent) => {
        e.stopPropagation();
        setRemoveConfirmId(id);
    };

    const confirmRemove = async () => {
        if (!removeConfirmId) return;
        const id = removeConfirmId;
        setRemoveLoading(true);
        try {
            const ok = await deleteAgentSource(id);
            if (ok) {
                await load();
                if (value === id) onChange('');
            } else {
                toast.error(t('servers.deleteSourceError'));
            }
        } finally {
            setRemoveLoading(false);
            setRemoveConfirmId(null);
        }
    };

    const handleRefresh = (id: string, e: React.MouseEvent) => {
        e.stopPropagation();
        setRefreshConfirmId(id);
    };

    const confirmRefresh = async () => {
        if (!refreshConfirmId) return;
        const id = refreshConfirmId;
        setRefreshConfirmId(null);
        setRefreshingId(id);
        try {
            const ok = await refreshAgentSourceCache(id);
            if (ok) {
                toast.success(t('servers.refreshCacheSuccess'));
            } else {
                toast.error(t('servers.refreshCacheError'));
            }
        } finally {
            setRefreshingId(null);
        }
    };

    return (
        <div ref={rootRef} className="space-y-3">
            <FormField label={t('servers.agentSource')}>
                <div className="relative">
                    <button
                        ref={triggerRef}
                        type="button"
                        onClick={() => setOpen(o => !o)}
                        disabled={disabled}
                        className={cn(inputs.primary, 'flex items-center justify-between text-left')}
                    >
                        <span className="truncate">
                            {selected
                                ? `${selected.path ? '↳ ' : selected.cacheLocally ? '*' : ''}${selected.name}`
                                : t('servers.selectSource')}
                        </span>
                        <ChevronDown size={14} className="shrink-0 ml-2"/>
                    </button>

                    {open && menuPos && createPortal(
                        // Portaled to document.body and fixed-positioned so the list floats
                        // above the modal — not clipped by its overflow-y-auto — without
                        // resizing it; kept anchored to the trigger by updateMenuPos.
                        <div
                            ref={menuRef}
                            style={{
                                position: 'fixed',
                                top: menuPos.top,
                                left: menuPos.left,
                                width: menuPos.width,
                                maxHeight: menuPos.maxHeight,
                                zIndex: 60,
                            }}
                            className="overflow-auto rounded-lg border border-border bg-card shadow-lg dark:border-white/10 dark:bg-zinc-900"
                        >
                            {sources.map(s => (
                                <div
                                    key={s.id}
                                    onClick={() => handleSelect(s.id)}
                                    className="flex items-center justify-between gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-muted dark:hover:bg-white/5"
                                >
                                    <span className="truncate">{s.path ? '↳ ' : s.cacheLocally ? '*' : ''}{s.name}</span>
                                    <div className="flex items-center gap-1 shrink-0">
                                        {s.cacheLocally && (
                                            <button
                                                type="button"
                                                onClick={(e) => handleRefresh(s.id, e)}
                                                disabled={refreshingId === s.id}
                                                className="text-muted-foreground hover:text-sky-600 dark:hover:text-sky-400"
                                                title={t('servers.refreshCache')}
                                            >
                                                <RefreshCw size={14} className={refreshingId === s.id ? 'animate-spin' : undefined}/>
                                            </button>
                                        )}
                                        <button
                                            type="button"
                                            onClick={(e) => handleRemove(s.id, e)}
                                            className="text-muted-foreground hover:text-red-600 dark:hover:text-red-400"
                                            title={t('common.close')}
                                        >
                                            <X size={14}/>
                                        </button>
                                    </div>
                                </div>
                            ))}
                            <div
                                onClick={() => {
                                    setAdding(true);
                                    setOpen(false);
                                }}
                                className="px-3 py-2 text-sm cursor-pointer text-sky-600 dark:text-sky-400 hover:bg-muted dark:hover:bg-white/5 border-t border-border dark:border-white/10"
                            >
                                {t('servers.addNewSource')}
                            </div>
                        </div>,
                        document.body,
                    )}
                </div>
            </FormField>

            {adding && (
                <div className="space-y-3 rounded-lg border border-input bg-background p-3 dark:border-white/10 dark:bg-white/5">
                    <FormField label={t('servers.sourceName')}>
                        <input
                            type="text"
                            value={name}
                            onChange={e => setName(e.target.value)}
                            placeholder={t('servers.sourceNamePlaceholder')}
                            disabled={saving}
                            className={inputs.primary}
                        />
                    </FormField>

                    <div className="flex gap-4">
                        {(['url', 'path'] as const).map(type => (
                            <label key={type} className="flex items-center">
                                <input
                                    type="radio"
                                    checked={sourceType === type}
                                    onChange={() => setSourceType(type)}
                                    disabled={saving}
                                    className="rounded border-input bg-background dark:border-white/10 dark:bg-white/5 text-sky-500"
                                />
                                <span className="ml-2 text-sm text-foreground dark:text-zinc-300">
                                    {type === 'url' ? t('servers.sourceTypeUrl') : t('servers.sourceTypePath')}
                                </span>
                            </label>
                        ))}
                    </div>

                    {sourceType === 'url' ? (
                        <>
                            <FormField label="URL">
                                <input
                                    type="text"
                                    value={url}
                                    onChange={e => setUrl(e.target.value)}
                                    placeholder="https://example.com/awg-agent"
                                    disabled={saving}
                                    className={cn(inputs.primary, 'font-mono text-xs')}
                                />
                            </FormField>

                            <label className="flex items-center">
                                <input
                                    type="checkbox"
                                    checked={cacheLocally}
                                    onChange={e => setCacheLocally(e.target.checked)}
                                    disabled={saving}
                                    className="rounded border-input bg-background dark:border-white/10 dark:bg-white/5 text-sky-500"
                                />
                                <span className="ml-3 text-sm font-medium text-foreground dark:text-zinc-300">
                                    {t('servers.cacheLocally')}
                                </span>
                            </label>
                            <p className="text-xs text-muted-foreground dark:text-zinc-500">
                                {t('servers.cacheLocallyHint')}
                            </p>
                        </>
                    ) : (
                        <FormField label={t('servers.sourcePath')}>
                            <div>
                                <div className="flex gap-2">
                                    <input
                                        type="text"
                                        value={path}
                                        onChange={e => setPath(e.target.value)}
                                        placeholder="/usr/local/bin/awg-agent"
                                        disabled={saving}
                                        className={cn(inputs.primary, 'font-mono text-xs min-w-0 flex-1')}
                                    />
                                    {isDesktop && (
                                        <button
                                            type="button"
                                            onClick={handleBrowse}
                                            disabled={saving}
                                            className={cn('shrink-0 inline-flex items-center gap-2', buttons.secondary)}
                                        >
                                            <FolderOpen size={16}/>
                                            {t('common.browse')}
                                        </button>
                                    )}
                                </div>
                                <p className="text-xs text-muted-foreground dark:text-zinc-500 mt-2">
                                    {t('servers.sourcePathHint')}
                                </p>
                            </div>
                        </FormField>
                    )}

                    <div className="flex gap-3">
                        <button type="button" onClick={handleAdd} disabled={saving} className={cn('flex-1', buttons.primary)}>
                            {saving ? `${t('common.saving')}...` : t('servers.addSource')}
                        </button>
                        <button
                            type="button"
                            onClick={() => setAdding(false)}
                            disabled={saving}
                            className={cn('flex-1', buttons.secondary)}
                        >
                            {t('common.cancel')}
                        </button>
                    </div>
                </div>
            )}

            {removeConfirmId && (
                <ConfirmModal
                    title={t('servers.agentSource')}
                    message={t('servers.confirmDeleteSource')}
                    onConfirm={confirmRemove}
                    onCancel={() => setRemoveConfirmId(null)}
                    loading={removeLoading}
                />
            )}

            {refreshConfirmId && (
                <ConfirmModal
                    title={t('servers.refreshCache')}
                    message={t('servers.confirmRefreshCache')}
                    confirmLabel={t('servers.refreshCache')}
                    onConfirm={confirmRefresh}
                    onCancel={() => setRefreshConfirmId(null)}
                />
            )}
        </div>
    );
}
