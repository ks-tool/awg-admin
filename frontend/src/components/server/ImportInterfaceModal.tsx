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

import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {toast} from 'sonner';
import {AlertTriangle, CheckCircle2, RefreshCw} from 'lucide-react';
import {buttons, inputs, Modal} from '@/components/common/Modal';
import {FormField} from '@/components/common/FormField';
import {importInterfaceFromServer, listUnmanagedInterfaces, previewInterfaceImport} from '@/services/servers';
import {cn} from '@/lib/utils';
import type {ImportPreview, ImportPreviewHooks, ImportPreviewPeer, UnmanagedInterface} from '@/types';

// Sentinel <select> value for "type a name yourself" — needed for the clean
// migration path (`wg-quick down` first), where the interface isn't live and
// so isn't in the unmanaged list, but its conf is still on disk.
const CUSTOM = '__custom__';

const PEER_MAP_PLACEHOLDER = 'alice:\n  <private-key>: laptop\n  <private-key>: phone\nbob:\n  <private-key>: pc';

function shortKey(key: string) {
    return key.length > 12 ? `${key.slice(0, 12)}…` : key;
}

function PeerLine({peer}: {peer: ImportPreviewPeer}) {
    const {t} = useTranslation();
    return (
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs">
            {peer.name && <span className="font-medium text-foreground dark:text-zinc-200">{peer.name}</span>}
            <span className="font-mono text-muted-foreground dark:text-zinc-500" title={peer.publicKey}>{shortKey(peer.publicKey)}</span>
            <span className="font-mono text-foreground dark:text-zinc-300">{(peer.allowedIPs ?? []).join(', ')}</span>
            {peer.endpoint && <span className="font-mono text-muted-foreground dark:text-zinc-500">{peer.endpoint}</span>}
            {peer.keepalive > 0 && <span className="text-muted-foreground dark:text-zinc-500">{t('servers.import.keepalive', {s: peer.keepalive})}</span>}
            {peer.presharedKey && <span className="rounded bg-muted px-1 text-[10px] font-medium text-muted-foreground dark:bg-white/10 dark:text-zinc-400">PSK</span>}
        </div>
    );
}

function HookBlock({hooks}: {hooks: ImportPreviewHooks}) {
    const {t} = useTranslation();
    const phases: (keyof ImportPreviewHooks)[] = ['preUp', 'postUp', 'preDown', 'postDown'];
    const present = phases.filter(p => (hooks[p]?.length ?? 0) > 0);
    if (present.length === 0) {
        return <p className="text-xs text-muted-foreground dark:text-zinc-500">{t('servers.import.noHooks')}</p>;
    }
    return (
        <div className="space-y-2">
            {present.map(p => (
                <div key={p}>
                    <div className="text-[11px] font-semibold uppercase tracking-wide text-muted-foreground dark:text-zinc-500">{p}</div>
                    <pre className="mt-0.5 overflow-x-auto rounded bg-muted p-2 font-mono text-xs text-foreground dark:bg-black/30 dark:text-zinc-300">{hooks[p]!.join('\n')}</pre>
                </div>
            ))}
        </div>
    );
}

function Section({title, hint, children}: {title: string; hint?: string; children: React.ReactNode}) {
    return (
        <div className="space-y-1.5">
            <h4 className="text-sm font-semibold text-foreground dark:text-zinc-100">{title}</h4>
            {hint && <p className="text-xs text-muted-foreground dark:text-zinc-500">{hint}</p>}
            {children}
        </div>
    );
}

// ImportInterfaceModal adopts a wg-quick / awg-quick interface that's up on
// the server but unknown to awg-admin (Service.PreviewImport /
// ImportInterfaceFromServer). Two steps: pick the interface + paste the
// user→peer map, Preview (a dry run that reports what will be created and
// warns about what changed meaning), then Import. The preview is invalidated
// whenever the inputs change, so what's imported is always what was reviewed.
// unmanaged, if given, seeds the interface picker (else it's fetched here).
export function ImportInterfaceModal({serverId, initialInterface, unmanaged: unmanagedProp, onClose, onImported}: {
    serverId: string;
    initialInterface?: string;
    unmanaged?: UnmanagedInterface[] | null;
    onClose: () => void;
    onImported?: () => void;
}) {
    const {t} = useTranslation();
    const [unmanaged, setUnmanaged] = useState<UnmanagedInterface[]>(unmanagedProp ?? []);
    const [loadingList, setLoadingList] = useState(unmanagedProp == null);
    // A preselected name that isn't in the (already known) list goes to the
    // custom input so it stays editable; when the list is fetched here, the
    // loader below makes the same call once it knows.
    const preselectCustom = !!initialInterface && unmanagedProp != null && !unmanagedProp.some(u => u.name === initialInterface);
    const [selection, setSelection] = useState<string>(preselectCustom ? CUSTOM : (initialInterface ?? ''));
    const [customName, setCustomName] = useState(preselectCustom ? initialInterface! : '');
    const [peerMap, setPeerMap] = useState('');
    const [preview, setPreview] = useState<ImportPreview | null>(null);
    // The inputs the current preview was computed from; a mismatch means the
    // preview is stale and Import is withheld until it's re-run.
    const [previewedFor, setPreviewedFor] = useState<{iface: string; peerMap: string} | null>(null);
    const [error, setError] = useState<string | null>(null);
    const [previewBusy, setPreviewBusy] = useState(false);
    const [importBusy, setImportBusy] = useState(false);

    useEffect(() => {
        if (unmanagedProp != null) return;
        let cancelled = false;
        (async () => {
            const list = await listUnmanagedInterfaces(serverId);
            if (cancelled) return;
            setUnmanaged(list ?? []);
            setLoadingList(false);
            if (!initialInterface) {
                // Preselect the only candidate; otherwise leave the choice to the user.
                if (list && list.length === 1) setSelection(list[0].name);
            } else if (!list?.some(u => u.name === initialInterface)) {
                setCustomName(initialInterface);
                setSelection(CUSTOM);
            }
        })();
        return () => { cancelled = true; };
    }, [serverId, unmanagedProp, initialInterface]);

    const ifaceName = (selection === CUSTOM ? customName : selection).trim();
    const stale = !preview || !previewedFor || previewedFor.iface !== ifaceName || previewedFor.peerMap !== peerMap;
    const busy = previewBusy || importBusy;

    const runPreview = async () => {
        if (!ifaceName) return;
        setPreviewBusy(true);
        setError(null);
        try {
            const result = await previewInterfaceImport(serverId, ifaceName, peerMap);
            setPreview(result);
            setPreviewedFor({iface: ifaceName, peerMap});
        } catch (err) {
            setPreview(null);
            setPreviewedFor(null);
            setError(err instanceof Error && err.message ? err.message : t('servers.import.previewError'));
        } finally {
            setPreviewBusy(false);
        }
    };

    const runImport = async () => {
        if (!ifaceName || stale) return;
        setImportBusy(true);
        setError(null);
        try {
            await importInterfaceFromServer(serverId, ifaceName, peerMap);
            toast.success(t('servers.import.imported', {iface: ifaceName}));
            onImported?.();
            onClose();
        } catch (err) {
            setError(err instanceof Error && err.message ? err.message : t('servers.import.importError'));
        } finally {
            setImportBusy(false);
        }
    };

    const kindLabel = preview ? (preview.amnezia ? 'amneziawg' : 'wireguard') : '';

    return (
        <Modal title={t('servers.import.title')} onClose={onClose} size="lg" loading={busy}>
            <div className="space-y-4">
                <FormField label={t('servers.import.interfaceLabel')}>
                    <div className="space-y-2">
                        <select
                            value={selection}
                            onChange={e => setSelection(e.target.value)}
                            disabled={busy || loadingList}
                            className={inputs.primary}
                        >
                            <option value="">{loadingList ? t('common.loading') : '—'}</option>
                            {unmanaged.map(u => (
                                <option key={u.name} value={u.name}>{u.name} ({u.kind})</option>
                            ))}
                            <option value={CUSTOM}>{t('servers.import.customOption')}</option>
                        </select>
                        {selection === CUSTOM && (
                            <input
                                type="text"
                                value={customName}
                                onChange={e => setCustomName(e.target.value)}
                                placeholder="wg0"
                                disabled={busy}
                                className={cn(inputs.primary, 'font-mono')}
                            />
                        )}
                        {!loadingList && unmanaged.length === 0 && (
                            <p className="text-xs text-muted-foreground dark:text-zinc-500">{t('servers.import.noUnmanaged')}</p>
                        )}
                    </div>
                </FormField>

                <FormField label={t('servers.import.peerMapLabel')}>
                    <div>
                        <textarea
                            value={peerMap}
                            onChange={e => setPeerMap(e.target.value)}
                            placeholder={PEER_MAP_PLACEHOLDER}
                            rows={6}
                            disabled={busy}
                            spellCheck={false}
                            className={cn(inputs.primary, 'font-mono text-xs')}
                        />
                        <p className="mt-1 text-xs text-muted-foreground dark:text-zinc-500">{t('servers.import.peerMapHint')}</p>
                    </div>
                </FormField>

                {error && (
                    <p className="rounded-lg border border-red-200 bg-red-50 p-3 text-xs text-red-700 whitespace-pre-wrap break-words dark:border-red-500/25 dark:bg-red-500/10 dark:text-red-400">
                        {error}
                    </p>
                )}

                {preview && !stale && (
                    <div className="space-y-4 rounded-lg border border-input bg-background p-4 dark:border-white/10 dark:bg-white/5">
                        <Section title={t('servers.import.sectionInterface')}>
                            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-xs">
                                <dt className="text-muted-foreground dark:text-zinc-500">{t('servers.import.source')}</dt>
                                <dd className="font-mono text-foreground dark:text-zinc-300">{preview.source}</dd>
                                <dt className="text-muted-foreground dark:text-zinc-500">{t('servers.import.address')}</dt>
                                <dd className="font-mono text-foreground dark:text-zinc-300">{preview.address}</dd>
                                <dt className="text-muted-foreground dark:text-zinc-500">{t('servers.import.listenPort')}</dt>
                                <dd className="font-mono text-foreground dark:text-zinc-300">{preview.listenPort}</dd>
                                <dt className="text-muted-foreground dark:text-zinc-500">{t('servers.import.kind')}</dt>
                                <dd className="font-mono text-foreground dark:text-zinc-300">
                                    {kindLabel}
                                    {preview.mtu ? ` · MTU ${preview.mtu}` : ''}
                                    {preview.table ? ` · Table ${preview.table}` : ''}
                                    {preview.dns && preview.dns.length > 0 ? ` · DNS ${preview.dns.join(', ')}` : ''}
                                </dd>
                            </dl>
                            <p className={cn('flex items-center gap-1.5 text-xs', preview.live ? 'text-emerald-600 dark:text-emerald-400' : 'text-amber-600 dark:text-amber-400')}>
                                {preview.live ? <CheckCircle2 size={14}/> : <AlertTriangle size={14}/>}
                                {preview.live
                                    ? t('servers.import.live', {kind: preview.liveKind})
                                    : t('servers.import.down')}
                            </p>
                        </Section>

                        <Section title={t('servers.import.sectionUsers')}>
                            {preview.users.length === 0 ? (
                                <p className="text-xs text-muted-foreground dark:text-zinc-500">{t('servers.import.noUsers')}</p>
                            ) : (
                                <div className="space-y-2">
                                    {preview.users.map(u => (
                                        <div key={u.name} className="rounded border border-input p-2 dark:border-white/10">
                                            <div className="mb-1 flex items-center gap-2 text-sm font-medium text-foreground dark:text-zinc-200">
                                                {u.name}
                                                <span className={cn('rounded px-1.5 py-0.5 text-[10px] font-medium', u.exists
                                                    ? 'bg-muted text-muted-foreground dark:bg-white/10 dark:text-zinc-400'
                                                    : 'bg-emerald-100 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-400')}>
                                                    {u.exists ? t('servers.import.userExists') : t('servers.import.userNew')}
                                                </span>
                                            </div>
                                            <div className="space-y-1 pl-2">
                                                {u.peers.map(p => <PeerLine key={p.publicKey} peer={p}/>)}
                                            </div>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </Section>

                        {preview.embeddedPeers.length > 0 && (
                            <Section title={t('servers.import.sectionEmbedded')} hint={t('servers.import.embeddedHint')}>
                                <div className="space-y-1 pl-2">
                                    {preview.embeddedPeers.map(p => <PeerLine key={p.publicKey} peer={p}/>)}
                                </div>
                            </Section>
                        )}

                        <Section title={t('servers.import.sectionHooks')}>
                            <HookBlock hooks={preview.hooks}/>
                        </Section>

                        <Section title={t('servers.import.sectionGenerated')} hint={t('servers.import.generatedHint')}>
                            <HookBlock hooks={preview.generatedHooks}/>
                        </Section>

                        <Section title={t('servers.import.sectionWarnings')}>
                            {preview.warnings.length === 0 ? (
                                <p className="flex items-center gap-1.5 text-xs text-emerald-600 dark:text-emerald-400">
                                    <CheckCircle2 size={14}/>
                                    {t('servers.import.noWarnings')}
                                </p>
                            ) : (
                                <ul className="space-y-1">
                                    {preview.warnings.map((w, i) => (
                                        <li key={i} className="flex gap-1.5 text-xs text-amber-700 dark:text-amber-400">
                                            <AlertTriangle size={14} className="mt-0.5 shrink-0"/>
                                            <span className="break-words">{w}</span>
                                        </li>
                                    ))}
                                </ul>
                            )}
                        </Section>
                    </div>
                )}

                {preview && stale && (
                    <p className="text-xs text-muted-foreground dark:text-zinc-500">{t('servers.import.reviewFirst')}</p>
                )}

                <div className="flex gap-3 pt-2">
                    <button
                        onClick={runPreview}
                        disabled={busy || !ifaceName}
                        className={cn('flex-1 inline-flex items-center justify-center gap-1.5', stale ? buttons.primary : buttons.secondary)}
                    >
                        <RefreshCw size={14} className={cn(previewBusy && 'animate-spin')}/>
                        {previewBusy ? t('servers.import.previewing') : t('servers.import.preview')}
                    </button>
                    <button
                        onClick={runImport}
                        disabled={busy || stale}
                        className={cn('flex-1', buttons.primary)}
                    >
                        {importBusy ? t('servers.import.importing') : t('servers.import.confirm')}
                    </button>
                    <button onClick={onClose} disabled={busy} className={cn('flex-1', buttons.secondary)}>
                        {t('common.cancel')}
                    </button>
                </div>
            </div>
        </Modal>
    );
}
