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

import * as React from 'react';
import {useCallback, useEffect, useRef, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {toast} from 'sonner';
import {
    Activity,
    AlertTriangle,
    ArrowLeft,
    CheckCircle2,
    Edit2,
    GitCompareArrows,
    Network,
    Plus,
    RefreshCw,
    Rocket,
    Server as ServerIcon,
    Settings,
    Trash2,
} from 'lucide-react';
import {PageHeader} from '@/components/layout/PageHeader';
import {useNavigation} from '@/contexts/NavigationContext';
import {useAppStore} from '@/store';
import {CopyButton} from '@/components/common/CopyButton';
import {getInterfaceDefaults, updateInterfaceConfig} from '@/services/interfaces';
import {
    deleteAgentInterface,
    deployAgent,
    getDeployStatus,
    importInterface,
    reconcileServer,
    setServerMonitoring,
    syncServer,
    unlockServerSSH,
} from '@/services/servers';
import type {ReconcileReport} from '@/services/servers';
import {SSHPassphraseRequiredError} from '@/services/sshErrors';
import {SSHPassphraseModal} from '@/components/server/SSHPassphraseModal';
import {formatRelativeTime} from '@/lib/utils';
import type {InterfaceConfig} from '@/types';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs, Modal} from '@/components/common/Modal';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {Container} from '@/components/common/Container';
import {cn} from '@/lib/utils';
import {ServerFormFields} from '@/components/server/ServerFormFields';
import {AgentSourceCombobox} from '@/components/server/AgentSourceCombobox';
import {formDataToServerInput, serverToFormData, useServerForm} from '@/hooks/useServerForm';
import type {AuthType} from '@/hooks/useServerForm';

// ---------------------------------------------------------------------------
// Deploy agent modal
// ---------------------------------------------------------------------------

function DeployAgentModal({onClose, onSubmit, loading, step}: {
    onClose: () => void;
    onSubmit: (agentSourceId: string) => Promise<void>;
    loading: boolean;
    step: string;
}) {
    const {t} = useTranslation();
    const [agentSourceId, setAgentSourceId] = useState('');

    const handleSubmit = async () => {
        if (!agentSourceId) return toast.error(t('servers.noSourceSelected'));
        await onSubmit(agentSourceId);
    };

    return (
        // The deploy now runs in the background once started (see
        // Service.DeployAgent) — closing this modal doesn't cancel it, so
        // unlike a blocking action there's no reason to keep the user from
        // closing it while one is in flight (loading={false} here).
        <Modal title={t('servers.deployAgentTitle')} onClose={onClose} loading={false}>
            <div className="space-y-4">
                <AgentSourceCombobox value={agentSourceId} onChange={setAgentSourceId} disabled={loading}/>

                {loading && step && (
                    <div className="flex items-center gap-2 text-sm text-muted-foreground dark:text-zinc-400">
                        <RefreshCw size={14} className="animate-spin"/>
                        <span>{t(`servers.deploySteps.${step}`, step)}</span>
                    </div>
                )}

                <div className="flex gap-3 pt-4">
                    <button onClick={handleSubmit} disabled={loading} className={cn('flex-1', buttons.primary)}>
                        {loading ? t('servers.deploying') : t('servers.deploy')}
                    </button>
                    <button onClick={onClose} className={cn('flex-1', buttons.secondary)}>
                        {t('common.cancel')}
                    </button>
                </div>
            </div>
        </Modal>
    );
}

// ---------------------------------------------------------------------------
// Interface form
// ---------------------------------------------------------------------------

const INITIAL_INTERFACE_FORM = {
    iface: '',
    addr: '',
    listen: '51820',
    pk: '',
    // AmneziaWG obfuscation. `amnezia` toggles whether these are sent at all;
    // when off the interface is plain WireGuard. The individual params are held
    // as strings for input binding and converted on submit.
    amnezia: true,
    jc: '', jmin: '', jmax: '',
    s1: '', s2: '', s3: '', s4: '',
    h1: '', h2: '', h3: '', h4: '',
    i1: '', i2: '', i3: '', i4: '', i5: '',
};
type InterfaceFormData = typeof INITIAL_INTERFACE_FORM;

// The AmneziaWG param field names, split by value type, so the form and the
// submit mapper stay in sync with a single source of truth.
const AMNEZIA_NUM_FIELDS = ['jc', 'jmin', 'jmax', 's1', 's2', 's3', 's4'] as const;
const AMNEZIA_STR_FIELDS = ['h1', 'h2', 'h3', 'h4', 'i1', 'i2', 'i3', 'i4', 'i5'] as const;

// amneziaToForm projects an interface config's Amnezia params onto the form's
// string fields (used both to seed the edit form and to apply generated
// defaults on add). Missing/nil params become empty strings.
function amneziaToForm(cfg: Partial<InterfaceConfig>) {
    const s = (v: number | string | undefined | null) =>
        v === undefined || v === null ? '' : String(v);
    return {
        jc: s(cfg.jc), jmin: s(cfg.jmin), jmax: s(cfg.jmax),
        s1: s(cfg.s1), s2: s(cfg.s2), s3: s(cfg.s3), s4: s(cfg.s4),
        h1: s(cfg.h1), h2: s(cfg.h2), h3: s(cfg.h3), h4: s(cfg.h4),
        i1: s(cfg.i1), i2: s(cfg.i2), i3: s(cfg.i3), i4: s(cfg.i4), i5: s(cfg.i5),
    };
}

function InterfaceFormModal({
    title,
    mode,
    initialValues,
    onSubmit,
    onClose,
    loading,
    submitLabel,
}: {
    title: string;
    mode: 'add' | 'edit';
    initialValues: InterfaceFormData;
    onSubmit: (data: InterfaceFormData) => Promise<void>;
    onClose: () => void;
    loading: boolean;
    submitLabel: string;
}) {
    const {t} = useTranslation();
    const [form, setForm] = useState(initialValues);
    const [tab, setTab] = useState<'general' | 'amnezia'>('general');

    useEffect(() => setForm(initialValues), [initialValues]);

    // On add, pre-fill the Amnezia tab with server-generated obfuscation params
    // (kept identical to what CreateInterface would apply). Edit keeps whatever
    // the existing interface already has (seeded via initialValues).
    useEffect(() => {
        if (mode !== 'add') return;
        let cancelled = false;
        (async () => {
            const defaults = await getInterfaceDefaults();
            if (cancelled || !defaults) return;
            setForm(prev => ({...prev, ...amneziaToForm(defaults)}));
        })();
        return () => {
            cancelled = true;
        };
    }, [mode]);

    const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const {name, value, type} = e.target;
        const val = type === 'checkbox' ? (e.target as HTMLInputElement).checked : value;
        setForm(prev => ({...prev, [name]: val}));
    };

    const handleSubmit = async () => {
        if (!form.iface.trim()) return toast.error(t('servers.interfaces.name') + ' ' + t('common.required'));
        await onSubmit(form);
    };

    const tabButton = (key: 'general' | 'amnezia', label: string) => (
        <button
            key={key}
            type="button"
            onClick={() => setTab(key)}
            className={cn(
                '-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors',
                tab === key
                    ? 'border-sky-500 text-foreground dark:text-zinc-100'
                    : 'border-transparent text-muted-foreground hover:text-foreground dark:text-zinc-500 dark:hover:text-zinc-300',
            )}
        >
            {label}
        </button>
    );

    const amneziaNum = (name: (typeof AMNEZIA_NUM_FIELDS)[number]) => (
        <FormField key={name} label={name.toUpperCase()}>
            <input
                type="number"
                name={name}
                value={form[name] as string}
                onChange={handleChange}
                disabled={loading || !form.amnezia}
                className={inputs.primary}
            />
        </FormField>
    );

    const amneziaStr = (name: (typeof AMNEZIA_STR_FIELDS)[number]) => (
        <FormField key={name} label={name.toUpperCase()}>
            <input
                type="text"
                name={name}
                value={form[name] as string}
                onChange={handleChange}
                disabled={loading || !form.amnezia}
                className={cn(inputs.primary, 'font-mono text-xs')}
            />
        </FormField>
    );

    const sectionTitle = 'mb-2 text-xs font-semibold uppercase tracking-wide text-muted-foreground dark:text-zinc-500';

    return (
        <Modal title={title} onClose={onClose} loading={loading}>
            <div className="mb-4 flex gap-1 border-b border-border dark:border-white/10">
                {tabButton('general', t('servers.interfaces.tabGeneral'))}
                {tabButton('amnezia', t('servers.interfaces.tabAmnezia'))}
            </div>

            <div className="space-y-4">
                <div className={tab === 'general' ? 'space-y-4' : 'hidden'}>
                    <FormField label={t('servers.interfaces.name')}>
                        <input type="text" name="iface" value={form.iface} onChange={handleChange}
                               placeholder="wg0" disabled={loading} className={inputs.primary}/>
                    </FormField>

                    <FormField label={t('servers.interfaces.addr')}>
                        <div>
                            <input type="text" name="addr" value={form.addr} onChange={handleChange}
                                   placeholder="10.0.0.1/24" disabled={loading} className={inputs.primary}/>
                            <p className="mt-1 text-xs text-muted-foreground">{t('servers.interfaces.addrHint')}</p>
                        </div>
                    </FormField>

                    <FormField label={t('servers.interfaces.port')}>
                        <input type="number" name="listen" value={form.listen} onChange={handleChange}
                               min="1" max="65535" disabled={loading} className={inputs.primary}/>
                    </FormField>

                    <FormField label={t('servers.interfaces.privateKey')}>
                        <input
                            type="text"
                            name="pk"
                            value={form.pk}
                            onChange={handleChange}
                            placeholder={t('servers.interfaces.privateKeyPlaceholder')} disabled={loading}
                            className={cn(inputs.primary, 'resize-none font-mono text-xs')}
                        />
                    </FormField>
                </div>

                <div className={tab === 'amnezia' ? 'space-y-4' : 'hidden'}>
                    <FormField label="">
                        <label className="flex items-center cursor-pointer">
                            <input
                                type="checkbox"
                                name="amnezia"
                                checked={form.amnezia}
                                onChange={handleChange}
                                disabled={loading}
                                className="rounded border-input bg-background text-sky-500 focus:ring-sky-500 focus:ring-offset-0 disabled:opacity-50 dark:border-white/10 dark:bg-white/5"
                            />
                            <span className="ml-3 text-sm font-medium text-foreground dark:text-zinc-300">
                                {t('servers.interfaces.amneziaInterface')}
                            </span>
                        </label>
                    </FormField>
                    <p className="text-xs text-muted-foreground">{t('servers.interfaces.amneziaHint')}</p>

                    <div>
                        <h4 className={sectionTitle}>{t('servers.interfaces.junkPackets')}</h4>
                        <div className="space-y-3">
                            {AMNEZIA_NUM_FIELDS.slice(0, 3).map(amneziaNum)}
                        </div>
                    </div>

                    <div>
                        <h4 className={sectionTitle}>{t('servers.interfaces.packetPadding')}</h4>
                        <div className="space-y-3">
                            {AMNEZIA_NUM_FIELDS.slice(3).map(amneziaNum)}
                        </div>
                    </div>

                    <div>
                        <h4 className={sectionTitle}>{t('servers.interfaces.magicHeaders')}</h4>
                        <div className="space-y-3">
                            {AMNEZIA_STR_FIELDS.slice(0, 4).map(amneziaStr)}
                        </div>
                    </div>

                    <div>
                        <h4 className={sectionTitle}>{t('servers.interfaces.signatures')}</h4>
                        <div className="space-y-3">
                            {AMNEZIA_STR_FIELDS.slice(4).map(amneziaStr)}
                        </div>
                    </div>
                </div>

                <div className="flex gap-3 pt-4">
                    <button onClick={handleSubmit} disabled={loading || !form.iface.trim()}
                            className={cn('flex-1', buttons.primary)}>
                        {loading ? `${t('common.saving')}...` : submitLabel}
                    </button>
                    <button onClick={onClose} disabled={loading}
                            className={cn('flex-1', buttons.secondary)}>
                        {t('common.cancel')}
                    </button>
                </div>
            </div>
        </Modal>
    );
}

// ---------------------------------------------------------------------------
// Reconcile modal — agent ↔ admin DB interface mismatch
// ---------------------------------------------------------------------------

function ReconcileModal({serverId, onClose, onChanged}: {
    serverId: string;
    onClose: () => void;
    onChanged: () => void;
}) {
    const {t} = useTranslation();
    const {deleteInterface} = useAppStore();
    const [report, setReport] = useState<ReconcileReport | null>(null);
    const [loading, setLoading] = useState(true);
    const [busy, setBusy] = useState<string | null>(null);

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
                                                onClick={() => handleDeleteFromDB(iface.id, iface.iface)}
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
    );
}

// ---------------------------------------------------------------------------
// ServerDetail
// ---------------------------------------------------------------------------

export default function ServerDetail() {
    const {t} = useTranslation();
    const {navigate} = useNavigation();
    const {
        servers,
        selectedServerId,
        setSelectedServer,
        updateServer,
        deleteServer,
        createInterface,
        deleteInterface,
        listInterfacesForServer,
        refreshData,
    } = useAppStore();

    const [isLoading, setIsLoading] = useState(false);
    const [isEditing, setIsEditing] = useState(false);
    const [authType, setAuthType] = useState<AuthType>('key');
    const [showDeployModal, setShowDeployModal] = useState(false);
    const [deployLoading, setDeployLoading] = useState(false);
    const [deployStep, setDeployStep] = useState('');
    const deployPollRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const [syncLoading, setSyncLoading] = useState(false);
    const [showReconcileModal, setShowReconcileModal] = useState(false);
    const [monitoringLoading, setMonitoringLoading] = useState(false);
    const [serverInterfaces, setServerInterfaces] = useState<any[]>([]);
    const [sshUnlock, setSshUnlock] = useState<{retry: () => void} | null>(null);
    const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
    const [deleteLoading, setDeleteLoading] = useState(false);
    const [deleteInterfaceId, setDeleteInterfaceId] = useState<string | null>(null);
    const [deleteInterfaceLoading, setDeleteInterfaceLoading] = useState(false);
    const [unlockLoading, setUnlockLoading] = useState(false);
    const [unlockError, setUnlockError] = useState<string | undefined>();
    const [interfaceModal, setInterfaceModal] = useState<{
        mode: 'add' | 'edit';
        initialValues: InterfaceFormData;
        interfaceId?: string;
    } | null>(null);

    const server = servers.find(s => s.id === selectedServerId);
    const {formData, updateField, resetForm} = useServerForm(
        server ? serverToFormData(server) : undefined,
    );

    // ---- Effects ------------------------------------------------------------

    useEffect(() => {
        if (server) {
            resetForm(serverToFormData(server));
            loadInterfaces();
        }
    }, [server]);

    const loadInterfaces = useCallback(async () => {
        if (!selectedServerId) return;
        const list = await listInterfacesForServer(selectedServerId);
        if (list) setServerInterfaces(list);
    }, [selectedServerId, listInterfacesForServer]);

    // ---- Server handlers ----------------------------------------------------

    const handleSave = async () => {
        if (!selectedServerId || !formData.name.trim()) {
            return toast.error(t('servers.nameRequired'));
        }
        setIsLoading(true);
        try {
            const updated = await updateServer(
                selectedServerId,
                formDataToServerInput(formData, authType, server),
            );
            if (updated) {
                toast.success(t('servers.serverUpdated'));
                setIsEditing(false);
                await refreshData();
            } else {
                toast.error(t('servers.updateServerError'));
            }
        } catch (err) {
            if (err instanceof SSHPassphraseRequiredError) {
                // The server record itself was already saved by the backend
                // before the dial attempt failed — only the live tunnel is
                // pending, so on success we just refresh and finish editing.
                setSshUnlock({
                    retry: () => {
                        toast.success(t('servers.serverUpdated'));
                        setIsEditing(false);
                        void refreshData();
                    },
                });
                return;
            }
            console.error('Failed to update server:', err);
            toast.error(t('servers.updateServerError'));
        } finally {
            setIsLoading(false);
        }
    };

    const handleCancel = () => {
        if (server) resetForm(serverToFormData(server));
        setIsEditing(false);
    };

    const goBack = () => {
        setSelectedServer(null);
        navigate('servers');
    };

    const handleDeleteServer = async () => {
        if (!selectedServerId) return;
        setDeleteLoading(true);
        try {
            const ok = await deleteServer(selectedServerId);
            if (ok) {
                toast.success(t('servers.serverDeleted'));
                goBack();
            } else {
                toast.error(t('servers.deleteServerError'));
            }
        } catch (err) {
            console.error('Failed to delete server:', err);
            toast.error(t('servers.deleteServerError'));
        } finally {
            setDeleteLoading(false);
            setShowDeleteConfirm(false);
        }
    };

    const stopDeployPolling = useCallback(() => {
        if (deployPollRef.current !== null) {
            clearInterval(deployPollRef.current);
            deployPollRef.current = null;
        }
    }, []);

    // pollDeployStatus's passphrase-retry path needs to re-invoke
    // handleDeployAgent, which itself calls pollDeployStatus to start
    // watching the retry — a genuine cycle between the two. A ref breaks
    // it without forward-referencing a not-yet-declared const (which the
    // React Compiler may not preserve memoization for, even though it's
    // safe at plain-JS runtime via closures).
    const handleDeployAgentRef = useRef<(agentSourceId: string) => void>(() => {});

    // DeployAgent only starts the deploy and returns immediately (it can
    // take a while — downloading a binary, SSH round-trips); this polls
    // GetDeployStatus for step-by-step progress and the eventual outcome.
    const pollDeployStatus = useCallback(async (agentSourceId: string) => {
        if (!selectedServerId) return;
        try {
            const status = await getDeployStatus(selectedServerId);
            if (!status) return; // not started yet from the backend's point of view — keep polling
            setDeployStep(status.step);
            if (!status.done) return;

            stopDeployPolling();
            setDeployLoading(false);
            if (status.error) {
                console.error('Failed to deploy agent:', status.error);
                toast.error(t('servers.deployError'));
            } else {
                toast.success(t('servers.deployedSuccess'));
                setShowDeployModal(false);
            }
        } catch (err) {
            stopDeployPolling();
            setDeployLoading(false);
            if (err instanceof SSHPassphraseRequiredError) {
                setSshUnlock({retry: () => handleDeployAgentRef.current(agentSourceId)});
                return;
            }
            console.error('Failed to poll deploy status:', err);
            toast.error(t('servers.deployError'));
        }
    }, [selectedServerId, stopDeployPolling, t]);

    const handleDeployAgent = useCallback(async (agentSourceId: string) => {
        if (!selectedServerId) return;
        stopDeployPolling();
        setDeployLoading(true);
        setDeployStep('connect');
        try {
            const ok = await deployAgent(selectedServerId, agentSourceId);
            if (!ok) {
                setDeployLoading(false);
                toast.error(t('servers.deployError'));
                return;
            }
            void pollDeployStatus(agentSourceId);
            deployPollRef.current = setInterval(() => void pollDeployStatus(agentSourceId), 1000);
        } catch (err) {
            setDeployLoading(false);
            if (err instanceof SSHPassphraseRequiredError) {
                setSshUnlock({retry: () => handleDeployAgentRef.current(agentSourceId)});
                return;
            }
            console.error('Failed to deploy agent:', err);
            toast.error(t('servers.deployError'));
        }
    }, [selectedServerId, stopDeployPolling, pollDeployStatus, t]);

    useEffect(() => {
        handleDeployAgentRef.current = (agentSourceId: string) => void handleDeployAgent(agentSourceId);
    }, [handleDeployAgent]);

    useEffect(() => stopDeployPolling, [stopDeployPolling]);

    const handleUnlockSubmit = async (passphrase: string, applyToAll: boolean) => {
        if (!selectedServerId || !sshUnlock) return;
        setUnlockLoading(true);
        setUnlockError(undefined);
        try {
            const ok = await unlockServerSSH(selectedServerId, passphrase, applyToAll);
            if (ok) {
                const {retry} = sshUnlock;
                setSshUnlock(null);
                retry();
            } else {
                setUnlockError(t('auth.unlockError'));
            }
        } finally {
            setUnlockLoading(false);
        }
    };

    const handleSyncServer = async () => {
        if (!selectedServerId) return;
        setSyncLoading(true);
        try {
            const ok = await syncServer(selectedServerId);
            if (ok) {
                toast.success(t('servers.syncSuccess'));
                await loadInterfaces();
            } else {
                toast.error(t('servers.syncError'));
            }
        } finally {
            setSyncLoading(false);
        }
    };

    const handleToggleMonitoring = async () => {
        if (!selectedServerId || !server) return;
        const enable = Boolean(server.agent?.monitoringDisabled);
        setMonitoringLoading(true);
        try {
            const updated = await setServerMonitoring(selectedServerId, enable);
            if (updated) {
                toast.success(enable ? t('servers.monitoringEnabled') : t('servers.monitoringDisabled'));
                await refreshData();
            } else {
                toast.error(t('servers.monitoringError'));
            }
        } finally {
            setMonitoringLoading(false);
        }
    };

    // ---- Interface handlers -------------------------------------------------

    const handleDeleteInterface = async () => {
        if (!selectedServerId || !deleteInterfaceId) return;
        setDeleteInterfaceLoading(true);
        try {
            const ok = await deleteInterface(selectedServerId, deleteInterfaceId);
            if (ok) {
                toast.success(t('servers.interfaces.deleted'));
                await loadInterfaces();
            } else {
                toast.error(t('servers.interfaces.deleteError'));
            }
        } catch (err) {
            console.error('Failed to delete interface:', err);
            toast.error(t('servers.interfaces.deleteError'));
        } finally {
            setDeleteInterfaceLoading(false);
            setDeleteInterfaceId(null);
        }
    };

    const handleInterfaceSubmit = async (form: InterfaceFormData) => {
        const config: InterfaceConfig = {
            iface: form.iface,
            addr: form.addr,
            listen: parseInt(form.listen) || 51820,
            pk: form.pk,
        };

        // Only carry the AmneziaWG obfuscation params when the interface is an
        // Amnezia one; otherwise they're omitted, so the backend stores a plain
        // WireGuard interface (and, on edit, strips any previously-set params).
        if (form.amnezia) {
            const num = (v: string) => {
                const n = parseInt(v, 10);
                return Number.isFinite(n) ? n : undefined;
            };
            const str = (v: string) => {
                const trimmed = v.trim();
                return trimmed === '' ? undefined : trimmed;
            };
            for (const f of AMNEZIA_NUM_FIELDS) config[f] = num(form[f]);
            for (const f of AMNEZIA_STR_FIELDS) config[f] = str(form[f]);
        }

        let ok = false;
        if (interfaceModal?.mode === 'add' && selectedServerId) {
            ok = !!(await createInterface(selectedServerId, config));
        } else if (interfaceModal?.mode === 'edit' && interfaceModal.interfaceId) {
            ok = !!(await updateInterfaceConfig(selectedServerId!, interfaceModal.interfaceId, config));
        }

        if (ok) {
            toast.success(interfaceModal?.mode === 'add' ? t('servers.interfaces.added') : t('servers.interfaces.updated'));
            setInterfaceModal(null);
            await loadInterfaces();
        } else {
            toast.error(interfaceModal?.mode === 'add' ? t('servers.interfaces.addError') : t('servers.interfaces.updateError'));
        }
    };

    // ---- Render: not found --------------------------------------------------

    if (!server) {
        return (
            <div className="flex flex-col">
                <PageHeader
                    title={t('servers.title')}
                    icon={ServerIcon}
                    description={t('servers.noServers')}
                    actions={
                        <button onClick={goBack} className={buttons.primary} title={t('servers.backToServers')}>
                            <ArrowLeft size={14}/>
                        </button>
                    }
                />
                <div className="p-8 text-center text-muted-foreground">{t('servers.noServers')}</div>
            </div>
        );
    }

    // ---- Render: main -------------------------------------------------------

    return (
        <div className="flex flex-col">
            <PageHeader
                title={server.name || t('servers.noName') || 'Unnamed Server'}
                icon={ServerIcon}
                description={server.id}
                actions={
                    <div className="flex items-center gap-2">
                        <button
                            onClick={() => setShowDeleteConfirm(true)}
                            className={buttons.danger}
                            title={t('servers.deleteServer')}
                        >
                            <Trash2 size={14}/>
                        </button>
                        <button onClick={() => setShowDeployModal(true)} className={buttons.secondary} title={t('servers.deployAgentTooltip')}>
                            <Rocket size={14}/>
                        </button>
                        <button
                            onClick={handleToggleMonitoring}
                            disabled={monitoringLoading}
                            className={cn(buttons.secondary, server.agent?.monitoringDisabled && 'opacity-50')}
                            title={server.agent?.monitoringDisabled ? t('servers.enableMonitoring') : t('servers.disableMonitoring')}
                        >
                            <Activity size={14}/>
                        </button>
                        <button
                            onClick={handleSyncServer}
                            disabled={syncLoading}
                            className={buttons.secondary}
                            title={t('servers.syncTooltip')}
                        >
                            <RefreshCw size={14} className={syncLoading ? 'animate-spin' : undefined}/>
                        </button>
                        <button
                            onClick={() => setShowReconcileModal(true)}
                            className={buttons.secondary}
                            title={t('servers.reconcile.tooltip')}
                        >
                            <GitCompareArrows size={14}/>
                        </button>
                        <button onClick={goBack} className={buttons.primary} title={t('servers.backToServers')}>
                            <ArrowLeft size={14}/>
                        </button>
                    </div>
                }
            />

            <div className="p-8 space-y-6">
                {/* Server information */}
                <Container
                    title={t('servers.information')}
                    action={!isEditing && (
                        <button onClick={() => setIsEditing(true)} className={buttons.primary}>
                            <Settings size={14}/>
                        </button>
                    )}
                >
                    {isEditing ? (
                        <form onSubmit={e => e.preventDefault()}>
                            <ServerFormFields
                                formData={formData}
                                onChange={updateField}
                                disabled={isLoading}
                                showAuth
                                authType={authType}
                                onAuthTypeChange={setAuthType}
                                serverId={server.id}
                                hasStoredKey={Boolean(server.ssh.keyData)}
                                hasTLSCertificates={!!server.agent.tls?.ca?.cert}
                                onCertificatesGenerated={async () => {
                                    toast.success(t('servers.certificatesGenerated'));
                                    await refreshData();
                                }}
                            />
                            <div className="flex gap-3 pt-6">
                                <button type="button" onClick={handleSave} disabled={isLoading}
                                        className={buttons.primary}>
                                    {isLoading ? `${t('common.saving')}...` : t('common.save')}
                                </button>
                                <button type="button" onClick={handleCancel} disabled={isLoading}
                                        className={buttons.secondary}>
                                    {t('common.cancel')}
                                </button>
                            </div>
                        </form>
                    ) : (
                        <div className="space-y-3">
                            <div className="flex items-center gap-3 text-sm">
                                <div className="flex items-center gap-1 font-mono text-xs">
                                    <span className="text-foreground font-semibold dark:text-zinc-100">
                                        SSH: {server.ssh.host}:{server.ssh.port ?? 22}
                                    </span>
                                    <CopyButton value={`${server.ssh.host}:${server.ssh.port ?? 22}`}/>
                                </div>
                                <span className="text-muted-foreground dark:text-zinc-400">|</span>
                                <div className="flex items-center gap-1 font-mono text-xs">
                                    <span className="text-foreground font-semibold dark:text-zinc-100">
                                        {t('servers.agentLabel')}: {server.agent.addr}
                                    </span>
                                    <CopyButton value={server.agent.addr}/>
                                </div>
                                {server.agent.tls && <span className="text-emerald-600 text-xs">🔒</span>}
                            </div>

                            {(server.info?.location || server.info?.description) && (
                                <div className="text-xs text-muted-foreground dark:text-zinc-500 space-x-4">
                                    {server.info.location && (
                                        <span>
                                            <span className="font-medium dark:text-zinc-400">{t('servers.location')}:</span>{' '}
                                            <span className="text-foreground font-semibold dark:text-zinc-100">
                                                {server.info.location}
                                            </span>
                                        </span>
                                    )}
                                    {server.info.location && server.info.description && (
                                        <span className="mx-2">|</span>
                                    )}
                                    {server.info.description && (
                                        <span>
                                            <span className="font-medium dark:text-zinc-400">{t('common.description')}:</span>{' '}
                                            <span className="text-foreground font-semibold dark:text-zinc-100">
                                                {server.info.description}
                                            </span>
                                        </span>
                                    )}
                                </div>
                            )}
                        </div>
                    )}
                </Container>

                {/* Interfaces */}
                <div className="rounded-xl border border-border bg-card p-6 dark:border-white/5 dark:bg-white/3">
                    <div className="flex items-center justify-between mb-4">
                        <h2 className="text-lg font-semibold text-foreground dark:text-zinc-100 flex items-center gap-2">
                            <Network size={20}/>
                            {t('servers.interface')} ({serverInterfaces.length})
                        </h2>
                        <button
                            onClick={() => setInterfaceModal({mode: 'add', initialValues: INITIAL_INTERFACE_FORM})}
                            className={buttons.primary}
                            title={t('servers.addInterfaceTooltip')}
                        >
                            <Plus size={14}/>
                        </button>
                    </div>

                    {serverInterfaces.length > 0 ? (
                        <div className="space-y-3">
                            {serverInterfaces.map(iface => (
                                <div
                                    key={iface.id ?? iface.iface}
                                    className="rounded-lg border border-input bg-background p-4 dark:border-white/10 dark:bg-white/5"
                                >
                                    <div className="flex items-center justify-between mb-3">
                                        {iface.iface && (
                                            <div className="text-sm font-semibold text-foreground dark:text-zinc-100 flex items-center gap-2">
                                                <Network size={16}/>
                                                {iface.iface}
                                                {iface.inSync ? (
                                                    <span
                                                        className="text-emerald-600 dark:text-emerald-400"
                                                        title={iface.lastSyncedAt
                                                            ? t('servers.interfaces.syncedAt', {time: formatRelativeTime(iface.lastSyncedAt)})
                                                            : t('servers.interfaces.synced')}
                                                    >
                                                        <CheckCircle2 size={14}/>
                                                    </span>
                                                ) : (
                                                    <span
                                                        className="text-amber-600 dark:text-amber-400"
                                                        title={iface.lastSyncError || t('servers.interfaces.notSynced')}
                                                    >
                                                        <AlertTriangle size={14}/>
                                                    </span>
                                                )}
                                            </div>
                                        )}
                                        {iface.id && (
                                            <div className="flex gap-2">
                                                <button
                                                    onClick={() => setInterfaceModal({
                                                        mode: 'edit',
                                                        initialValues: {
                                                            iface: iface.iface ?? '',
                                                            addr: iface.addr ?? '',
                                                            listen: String(iface.listen ?? 51820),
                                                            pk: iface.pk ?? '',
                                                            amnezia: iface.jc != null || iface.jmin != null || iface.jmax != null,
                                                            ...amneziaToForm(iface),
                                                        },
                                                        interfaceId: iface.id,
                                                    })}
                                                    disabled={!!iface.tunnel}
                                                    className="p-1 text-muted-foreground hover:text-sky-600 dark:hover:text-sky-400 hover:bg-sky-100 dark:hover:bg-sky-500/10 rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-transparent"
                                                    title={iface.tunnel ? t('servers.interfaces.inTunnelTooltip') : t('servers.interfaces.editTooltip')}
                                                >
                                                    <Edit2 size={16}/>
                                                </button>
                                                <button
                                                    onClick={() => setDeleteInterfaceId(iface.id)}
                                                    disabled={!!iface.tunnel}
                                                    className="p-1 text-muted-foreground hover:text-red-600 dark:hover:text-red-400 hover:bg-red-100 dark:hover:bg-red-500/10 rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed disabled:hover:bg-transparent"
                                                    title={iface.tunnel ? t('servers.interfaces.inTunnelTooltip') : t('servers.interfaces.deleteTooltip')}
                                                >
                                                    <Trash2 size={16}/>
                                                </button>
                                            </div>
                                        )}
                                    </div>
                                    {iface.addr && (
                                        <div className="text-xs text-muted-foreground dark:text-zinc-500 mb-2">
                                            {t('servers.interfaces.addressLabel')}: <span className="font-mono text-foreground dark:text-zinc-300">{iface.addr}</span>
                                        </div>
                                    )}
                                    {iface.listen && (
                                        <div className="text-xs text-muted-foreground dark:text-zinc-500">
                                            {t('servers.interfaces.listenPortLabel')}: <span className="font-mono text-foreground dark:text-zinc-300">{iface.listen}</span>
                                        </div>
                                    )}
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div className="text-center text-muted-foreground dark:text-zinc-500 py-4">
                            {t('servers.interfaces.noInterfaces')}
                        </div>
                    )}
                </div>
            </div>

            {interfaceModal && (
                <InterfaceFormModal
                    title={interfaceModal.mode === 'add'
                        ? t('servers.interfaces.addTitle')
                        : t('servers.interfaces.editTitle')}
                    mode={interfaceModal.mode}
                    initialValues={interfaceModal.initialValues}
                    onSubmit={handleInterfaceSubmit}
                    onClose={() => setInterfaceModal(null)}
                    loading={false}
                    submitLabel={interfaceModal.mode === 'add' ? t('servers.interfaces.addTitle') : t('common.save')}
                />
            )}

            {showDeployModal && (
                <DeployAgentModal
                    onSubmit={handleDeployAgent}
                    onClose={() => setShowDeployModal(false)}
                    loading={deployLoading}
                    step={deployStep}
                />
            )}

            {showReconcileModal && selectedServerId && (
                <ReconcileModal
                    serverId={selectedServerId}
                    onClose={() => setShowReconcileModal(false)}
                    onChanged={loadInterfaces}
                />
            )}

            {sshUnlock && (
                <SSHPassphraseModal
                    onSubmit={handleUnlockSubmit}
                    onClose={() => setSshUnlock(null)}
                    loading={unlockLoading}
                    error={unlockError}
                />
            )}

            {showDeleteConfirm && (
                <ConfirmModal
                    title={t('servers.deleteServer')}
                    message={t('servers.confirmDeleteServer')}
                    onConfirm={handleDeleteServer}
                    onCancel={() => setShowDeleteConfirm(false)}
                    loading={deleteLoading}
                />
            )}

            {deleteInterfaceId && (
                <ConfirmModal
                    title={t('servers.interfaces.deleteTooltip')}
                    message={t('servers.interfaces.confirmDelete')}
                    onConfirm={handleDeleteInterface}
                    onCancel={() => setDeleteInterfaceId(null)}
                    loading={deleteInterfaceLoading}
                />
            )}
        </div>
    );
}
