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
import {toast} from 'sonner';
import {CheckCircle2, RefreshCw} from 'lucide-react';
import {useAppStore} from '@/store';
import {deleteAgentInterface, importInterface, reconcileServer, syncServer} from '@/services/servers';
import type {ReconcileReport} from '@/services/servers';
import {buttons, Modal} from '@/components/common/Modal';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {cn} from '@/lib/utils';

// ReconcileModal shows the drift between an agent's actual interfaces and the
// admin DB (Service.ReconcileServer), letting the admin resolve each side —
// import/delete an agent-only interface, re-push/delete a DB-only one. onChanged
// fires after any change so the opener can refresh its own view.
export function ReconcileModal({serverId, onClose, onChanged}: {
    serverId: string;
    onClose: () => void;
    onChanged: () => void;
}) {
    const {t} = useTranslation();
    const {deleteInterface} = useAppStore();
    const [report, setReport] = useState<ReconcileReport | null>(null);
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState<string | null>(null);
    // Deleting a DB-only interface cascades its peers too (same as the normal
    // delete), so it goes through the same peer-aware confirmation rather than
    // firing immediately. dbDeleteConfirmed tracks the second stage when there
    // are peers.
    const [pendingDbDelete, setPendingDbDelete] = useState<{id: string; name: string; peerCount: number} | null>(null);
    const [dbDeleteConfirmed, setDbDeleteConfirmed] = useState(false);

    const load = useCallback(async () => {
        setLoading(true);
        const result = await reconcileServer(serverId);
        setReport(result);
        setLoading(false);
    }, [serverId]);

    useEffect(() => {
        void load();
    }, [load]);

    const handleImport = async (iface: string) => {
        setBusy(`import-${iface}`);
        try {
            const ok = await importInterface(serverId, iface);
            if (ok) {
                toast.success(t('servers.reconcile.imported', {iface}));
                onChanged();
                await load();
            } else {
                toast.error(t('servers.reconcile.importError'));
            }
        } finally {
            setBusy(null);
        }
    };

    const handleDeleteFromAgent = async (iface: string) => {
        setBusy(`delete-agent-${iface}`);
        try {
            const ok = await deleteAgentInterface(serverId, iface);
            if (ok) {
                toast.success(t('servers.reconcile.deletedFromAgent', {iface}));
                await load();
            } else {
                toast.error(t('servers.reconcile.deleteFromAgentError'));
            }
        } finally {
            setBusy(null);
        }
    };

    const handleRepush = async (iface: string) => {
        setBusy(`repush-${iface}`);
        try {
            const ok = await syncServer(serverId);
            if (ok) {
                toast.success(t('servers.reconcile.repushed', {iface}));
                onChanged();
                await load();
            } else {
                toast.error(t('servers.reconcile.repushError'));
            }
        } finally {
            setBusy(null);
        }
    };

    const handleDeleteFromDB = async (ifaceId: string, ifaceName: string) => {
        setBusy(`delete-db-${ifaceId}`);
        try {
            const ok = await deleteInterface(serverId, ifaceId);
            if (ok) {
                toast.success(t('servers.reconcile.deletedFromDb', {iface: ifaceName}));
                onChanged();
                await load();
            } else {
                toast.error(t('servers.reconcile.deleteFromDbError'));
            }
        } finally {
            setBusy(null);
        }
    };

    // Go marshals nil slices as JSON `null`, so agentOnly/dbOnly arrive as
    // null (not []) whenever that side has no mismatching interfaces — guard
    // against it here rather than assuming an array.
    const agentOnly = report?.agentOnly ?? [];
    const dbOnly = report?.dbOnly ?? [];
    const hasMismatch = report && (agentOnly.length > 0 || dbOnly.length > 0);

    return (
        <>
        <Modal title={t('servers.reconcile.title')} onClose={onClose} size="lg">
            {loading ? (
                <div className="flex items-center justify-center py-12 text-muted-foreground dark:text-zinc-400">
                    <RefreshCw className="w-5 h-5 mr-2 animate-spin"/>
                    {t('common.loading')}
                </div>
            ) : !report ? (
                <div className="py-12 text-center text-sm text-red-500">{t('servers.reconcile.loadError')}</div>
            ) : !hasMismatch ? (
                <div className="flex items-center gap-2 py-12 justify-center text-sm text-muted-foreground dark:text-zinc-400">
                    <CheckCircle2 size={16} className="text-emerald-500"/>
                    {t('servers.reconcile.noMismatch')}
                </div>
            ) : (
                <div className="space-y-6">
                    {agentOnly.length > 0 && (
                        <div className="space-y-2">
                            <h3 className="text-sm font-semibold text-foreground dark:text-zinc-100">
                                {t('servers.reconcile.agentOnlyTitle')}
                            </h3>
                            <p className="text-xs text-muted-foreground dark:text-zinc-500">
                                {t('servers.reconcile.agentOnlyHint')}
                            </p>
                            <div className="space-y-2">
                                {agentOnly.map(cfg => (
                                    <div key={cfg.iface} className="flex items-center justify-between rounded-lg border border-input bg-background p-3 dark:border-white/10 dark:bg-white/5">
                                        <span className="font-mono text-sm dark:text-zinc-300">{cfg.iface}</span>
                                        <div className="flex gap-2">
                                            <button
                                                onClick={() => handleImport(cfg.iface)}
                                                disabled={busy === `import-${cfg.iface}`}
                                                className={buttons.primary}
                                            >
                                                {t('servers.reconcile.import')}
                                            </button>
                                            <button
                                                onClick={() => handleDeleteFromAgent(cfg.iface)}
                                                disabled={busy === `delete-agent-${cfg.iface}`}
                                                className={buttons.danger}
                                            >
                                                {t('servers.reconcile.deleteFromAgent')}
                                            </button>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}

                    {dbOnly.length > 0 && (
                        <div className="space-y-2">
                            <h3 className="text-sm font-semibold text-foreground dark:text-zinc-100">
                                {t('servers.reconcile.dbOnlyTitle')}
                            </h3>
                            <p className="text-xs text-muted-foreground dark:text-zinc-500">
                                {t('servers.reconcile.dbOnlyHint')}
                            </p>
                            <div className="space-y-2">
                                {dbOnly.map(iface => (
                                    <div key={iface.id} className="flex items-center justify-between rounded-lg border border-input bg-background p-3 dark:border-white/10 dark:bg-white/5">
                                        <span className="font-mono text-sm dark:text-zinc-300">{iface.iface}</span>
                                        <div className="flex gap-2">
                                            <button
                                                onClick={() => handleRepush(iface.iface)}
                                                disabled={busy === `repush-${iface.iface}`}
                                                className={buttons.primary}
                                            >
                                                {t('servers.reconcile.repush')}
                                            </button>
                                            <button
                                                onClick={() => {
                                                    setDbDeleteConfirmed(false);
                                                    setPendingDbDelete({id: iface.id, name: iface.iface, peerCount: iface.peers?.length ?? 0});
                                                }}
                                                disabled={busy === `delete-db-${iface.id}`}
                                                className={buttons.danger}
                                            >
                                                {t('servers.reconcile.deleteFromDb')}
                                            </button>
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </div>
                    )}
                </div>
            )}

            <div className="pt-4">
                <button onClick={onClose} className={cn('w-full', buttons.secondary)}>
                    {t('common.close')}
                </button>
            </div>
        </Modal>

        {pendingDbDelete && (() => {
            const closeConfirm = () => {
                setPendingDbDelete(null);
                setDbDeleteConfirmed(false);
            };
            // Peers present → confirm twice (first spells out the cascade), else once.
            if (pendingDbDelete.peerCount > 0 && !dbDeleteConfirmed) {
                return (
                    <ConfirmModal
                        title={t('servers.reconcile.deleteFromDb')}
                        message={t('servers.interfaces.confirmDeleteWithPeers', {
                            count: pendingDbDelete.peerCount,
                            name: pendingDbDelete.name,
                        })}
                        confirmLabel={t('common.continue')}
                        onConfirm={() => setDbDeleteConfirmed(true)}
                        onCancel={closeConfirm}
                    />
                );
            }
            return (
                <ConfirmModal
                    title={t('servers.reconcile.deleteFromDb')}
                    message={pendingDbDelete.peerCount > 0
                        ? t('servers.interfaces.confirmDeleteWithPeersFinal', {count: pendingDbDelete.peerCount})
                        : t('servers.interfaces.confirmDelete')}
                    onConfirm={async () => {
                        const {id, name} = pendingDbDelete;
                        closeConfirm();
                        await handleDeleteFromDB(id, name);
                    }}
                    onCancel={closeConfirm}
                    loading={busy === `delete-db-${pendingDbDelete.id}`}
                />
            );
        })()}
        </>
    );
}
