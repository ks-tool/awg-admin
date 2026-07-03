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
import {ChevronDown, FolderOpen, Pencil, RefreshCw, X} from 'lucide-react';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs, Modal} from '@/components/common/Modal';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {cn} from '@/lib/utils';
import {createAgentSource, deleteAgentSource, listAgentSources, refreshAgentSourceCache, selectAgentFile, updateAgentSource} from '@/services/agentSources';
import {getCurrentApiMode} from '@/services/apiMode';
import type {AgentSource} from '@/types';

interface Props {
    /** Selected AgentSource ID, or '' if nothing is selected yet. */
    value: string;
    onChange: (id: string) => void;
    disabled?: boolean;
    /** Whether the target server has usable Docker (from its agent's HostInfo).
     *  false hides image (Docker) sources — both existing ones in the list and
     *  the "image" type in the add form — since the docker deploy can't run
     *  there. undefined = unknown (no agent yet) → everything is offered. */
    dockerAvailable?: boolean;
}

/**
 * Modal form for creating a new agent source. Broken out of the combobox into
 * its own dialog: the fields (name, url/path/image type + its input, the cache
 * toggle and its hint) don't fit legibly inline under the dropdown when the
 * combobox itself lives inside another modal (the agent dialog). onCreated hands
 * the saved source back so the opener can refresh its list and select it.
 */
function AgentSourceModal({onClose, onSaved, dockerAvailable, source}: {
    onClose: () => void;
    onSaved: (src: AgentSource) => void;
    /** false => the target server has no usable Docker, so the Docker-image
     *  source type (which only the docker deploy uses) isn't offered. undefined
     *  = unknown (agent not deployed yet) → offer it; the deploy pre-checks. */
    dockerAvailable?: boolean;
    /** When set, the modal edits this existing source (pre-filled) instead of
     *  creating a new one. */
    source?: AgentSource;
}) {
    const {t} = useTranslation();
    const editing = !!source;
    // Hide the Docker-image option when Docker is known-unavailable — deploying
    // from an image would just fail the docker pre-check on that host. Keep it
    // shown, though, when editing a source that already is an image.
    const types = dockerAvailable === false && !source?.image
        ? (['url', 'path'] as const)
        : (['url', 'path', 'image'] as const);
    const [sourceType, setSourceType] = useState<'url' | 'path' | 'image'>(
        source?.image ? 'image' : source?.path ? 'path' : 'url',
    );
    const [name, setName] = useState(source?.name ?? '');
    const [url, setUrl] = useState(source?.url ?? '');
    const [path, setPath] = useState(source?.path ?? '');
    const [image, setImage] = useState(source?.image ?? '');
    const [cacheLocally, setCacheLocally] = useState(source?.cacheLocally ?? false);
    // Marks a URL/Path source as the userspace agent binary — the systemd deploy
    // then skips its AmneziaWG-kernel-module pre-check (see deploy.deploySystemd).
    const [userspace, setUserspace] = useState(source?.userspace ?? false);
    const [saving, setSaving] = useState(false);

    // The native file picker only exists in the Wails desktop app; a browser
    // can't resolve a real filesystem path, so the Browse button is desktop-only.
    const isDesktop = getCurrentApiMode() === 'bindings';

    const handleBrowse = async () => {
        const picked = await selectAgentFile(t('servers.selectAgentBinaryDialog'));
        if (picked) setPath(picked);
    };

    const handleSave = async () => {
        if (!name.trim()) return toast.error(t('servers.sourceNameRequired'));
        if (sourceType === 'url' && !url.trim()) return toast.error(t('servers.sourceUrlRequired'));
        if (sourceType === 'path' && !path.trim()) return toast.error(t('servers.sourcePathRequired'));
        if (sourceType === 'image' && !image.trim()) return toast.error(t('servers.sourceImageRequired'));

        const args = [
            name.trim(),
            sourceType === 'url' ? url.trim() : '',
            sourceType === 'path' ? path.trim() : '',
            sourceType === 'image' ? image.trim() : '',
            cacheLocally,
            sourceType !== 'image' && userspace,
        ] as const;

        setSaving(true);
        try {
            const src = source
                ? await updateAgentSource(source.id, ...args)
                : await createAgentSource(...args);
            if (src) {
                onSaved(src);
            } else {
                toast.error(editing ? t('servers.sourceUpdateError') : t('servers.sourceCreateError'));
            }
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal title={editing ? t('servers.editSourceTitle') : t('servers.addSourceTitle')} onClose={onClose} loading={saving}>
            <div className="space-y-4">
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

                <div className="flex flex-wrap gap-4">
                    {types.map(type => (
                        <label key={type} className="flex items-center cursor-pointer">
                            <input
                                type="radio"
                                checked={sourceType === type}
                                onChange={() => setSourceType(type)}
                                disabled={saving}
                                className="rounded border-input bg-background dark:border-white/10 dark:bg-white/5 text-sky-500"
                            />
                            <span className="ml-2 text-sm text-foreground dark:text-zinc-300">
                                {type === 'url'
                                    ? t('servers.sourceTypeUrl')
                                    : type === 'path'
                                        ? t('servers.sourceTypePath')
                                        : t('servers.sourceTypeImage')}
                            </span>
                        </label>
                    ))}
                </div>

                {sourceType === 'url' && (
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

                        <label className="flex items-center cursor-pointer">
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
                )}

                {sourceType === 'path' && (
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

                {sourceType === 'image' && (
                    <FormField label={t('servers.sourceImage')}>
                        <div>
                            <input
                                type="text"
                                value={image}
                                onChange={e => setImage(e.target.value)}
                                placeholder="ghcr.io/ks-tool/awg-agent-userspace:latest"
                                disabled={saving}
                                className={cn(inputs.primary, 'font-mono text-xs')}
                            />
                            <p className="text-xs text-muted-foreground dark:text-zinc-500 mt-2">
                                {t('servers.sourceImageHint')}
                            </p>
                        </div>
                    </FormField>
                )}

                {/* Userspace agent (systemd) — the binary can be awg-agent (kernel)
                    or awg-agent-userspace; ticking this skips the kernel-module
                    pre-check on deploy. Not applicable to a Docker image source. */}
                {sourceType !== 'image' && (
                    <>
                        <label className="flex items-center cursor-pointer">
                            <input
                                type="checkbox"
                                checked={userspace}
                                onChange={e => setUserspace(e.target.checked)}
                                disabled={saving}
                                className="rounded border-input bg-background dark:border-white/10 dark:bg-white/5 text-sky-500"
                            />
                            <span className="ml-3 text-sm font-medium text-foreground dark:text-zinc-300">
                                {t('servers.sourceUserspace')}
                            </span>
                        </label>
                        <p className="text-xs text-muted-foreground dark:text-zinc-500">
                            {t('servers.sourceUserspaceHint')}
                        </p>
                    </>
                )}

                <div className="flex gap-3 pt-2">
                    <button type="button" onClick={handleSave} disabled={saving} className={cn('flex-1', buttons.primary)}>
                        {saving ? `${t('common.saving')}...` : editing ? t('common.save') : t('servers.addSource')}
                    </button>
                    <button type="button" onClick={onClose} disabled={saving} className={cn('flex-1', buttons.secondary)}>
                        {t('common.cancel')}
                    </button>
                </div>
            </div>
        </Modal>
    );
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
export function AgentSourceCombobox({value, onChange, disabled = false, dockerAvailable}: Props) {
    const {t} = useTranslation();
    const [sources, setSources] = useState<AgentSource[]>([]);
    const [open, setOpen] = useState(false);
    const [adding, setAdding] = useState(false);
    const [editingSource, setEditingSource] = useState<AgentSource | null>(null);
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

    // Hide image (Docker) sources when Docker is known-unavailable on the target
    // server — the docker deploy can't run there. Unknown/available (undefined or
    // true) shows everything. A currently-selected image source is kept so the
    // trigger doesn't blank, but it won't be offered for re-selection.
    const visibleSources = dockerAvailable === false
        ? sources.filter(s => !s.image || s.id === value)
        : sources;

    const handleSelect = (id: string) => {
        onChange(id);
        setOpen(false);
    };

    const handleRemove = (id: string, e: React.MouseEvent) => {
        e.stopPropagation();
        // Close the (portaled, z-60) dropdown so it doesn't overlay the confirm
        // modal's buttons.
        setOpen(false);
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
        setOpen(false);
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
                                ? `${selected.image ? '🐳 ' : selected.path ? '↳ ' : selected.cacheLocally ? '*' : ''}${selected.name}`
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
                            {visibleSources.map(s => (
                                <div
                                    key={s.id}
                                    onClick={() => handleSelect(s.id)}
                                    className="flex items-center justify-between gap-2 px-3 py-2 text-sm cursor-pointer hover:bg-muted dark:hover:bg-white/5"
                                >
                                    <span className="truncate">{s.image ? '🐳 ' : s.path ? '↳ ' : s.cacheLocally ? '*' : ''}{s.name}</span>
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
                                            onClick={(e) => { e.stopPropagation(); setOpen(false); setEditingSource(s); }}
                                            className="text-muted-foreground hover:text-sky-600 dark:hover:text-sky-400"
                                            title={t('servers.editSource')}
                                        >
                                            <Pencil size={14}/>
                                        </button>
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
                <AgentSourceModal
                    dockerAvailable={dockerAvailable}
                    onClose={() => setAdding(false)}
                    onSaved={(src) => {
                        setAdding(false);
                        void load();
                        onChange(src.id);
                    }}
                />
            )}

            {editingSource && (
                <AgentSourceModal
                    source={editingSource}
                    dockerAvailable={dockerAvailable}
                    onClose={() => setEditingSource(null)}
                    onSaved={() => {
                        setEditingSource(null);
                        void load();
                    }}
                />
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
