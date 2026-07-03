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
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {toast} from 'sonner';
import {
    AlertTriangle,
    ArrowLeft,
    CheckCircle2,
    Edit2,
    Network,
    Plus,
    Server as ServerIcon,
    Settings,
    Trash2,
    Wrench,
} from 'lucide-react';
import {PageHeader} from '@/components/layout/PageHeader';
import {useNavigation} from '@/contexts/NavigationContext';
import {useAppStore} from '@/store';
import {CopyButton} from '@/components/common/CopyButton';
import {getInterfaceDefaults, updateInterfaceConfig} from '@/services/interfaces';
import {unlockServerSSH} from '@/services/servers';
import {SSHPassphraseRequiredError} from '@/services/sshErrors';
import {SSHPassphraseModal} from '@/components/server/SSHPassphraseModal';
import {formatRelativeTime} from '@/lib/utils';
import type {InterfaceConfig} from '@/types';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs, Modal} from '@/components/common/Modal';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {CollapsibleSection} from '@/components/common/CollapsibleSection';
import {Container} from '@/components/common/Container';
import {cn} from '@/lib/utils';
import {ServerFormFields} from '@/components/server/ServerFormFields';
import {AgentModal} from '@/components/server/AgentModal';
import {formDataToServerInput, serverToFormData, useServerForm} from '@/hooks/useServerForm';
import type {AuthType} from '@/hooks/useServerForm';

// ---------------------------------------------------------------------------
// Interface form
// ---------------------------------------------------------------------------

const INITIAL_INTERFACE_FORM = {
    iface: '',
    addr: '',
    listen: '51820',
    pk: '',
    // Whether the interface is active on the agent (default on). Maps to the
    // inverse of InterfaceConfig.disabled; the agent brings up only active
    // interfaces and tears down inactive ones.
    enabled: true,
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
            <div className="space-y-4">
                <FormField label={t('servers.interfaces.name')}>
                    <input type="text" name="iface" value={form.iface} onChange={handleChange}
                           placeholder="wg0" disabled={loading} className={inputs.primary}/>
                </FormField>

                <CollapsibleSection label={t('common.advancedSettings')} defaultOpen={mode === 'edit'}>
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

                    <FormField label="">
                        <label className="flex items-center cursor-pointer">
                            <input
                                type="checkbox"
                                name="enabled"
                                checked={form.enabled}
                                onChange={handleChange}
                                disabled={loading}
                                className="rounded border-input bg-background text-sky-500 focus:ring-sky-500 focus:ring-offset-0 disabled:opacity-50 dark:border-white/10 dark:bg-white/5"
                            />
                            <span className="ml-3 text-sm font-medium text-foreground dark:text-zinc-300">
                                {t('servers.interfaces.enabled')}
                            </span>
                        </label>
                    </FormField>
                    <p className="text-xs text-muted-foreground">{t('servers.interfaces.enabledHint')}</p>

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
                </CollapsibleSection>

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
// ServerDetail
// ---------------------------------------------------------------------------

export default function ServerDetail() {
    const {t} = useTranslation();
    const {navigate} = useNavigation();
    const {
        servers,
        users,
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
    // All agent operations (deploy, sync, metrics, reconcile, profiling) live in
    // the AgentModal now — this page just opens it.
    const [showAgentModal, setShowAgentModal] = useState(false);
    const [serverInterfaces, setServerInterfaces] = useState<any[]>([]);
    // sshUnlock is still used here for the server-save passphrase retry (handleSave);
    // the deploy passphrase flow moved into the AgentModal with the deploy action.
    const [sshUnlock, setSshUnlock] = useState<{retry: () => void} | null>(null);
    const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
    const [deleteLoading, setDeleteLoading] = useState(false);
    const [deleteInterfaceId, setDeleteInterfaceId] = useState<string | null>(null);
    // Second-stage flag: an interface that has peers requires confirming twice
    // (the cascade deletes all of them), so this tracks that the first warning
    // was acknowledged.
    const [deleteInterfaceConfirmed, setDeleteInterfaceConfirmed] = useState(false);
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
            setDeleteInterfaceConfirmed(false);
        }
    };

    const handleInterfaceSubmit = async (form: InterfaceFormData) => {
        const config: InterfaceConfig = {
            iface: form.iface,
            addr: form.addr,
            listen: parseInt(form.listen) || 51820,
            pk: form.pk,
            disabled: !form.enabled,
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

        const isAdd = interfaceModal?.mode === 'add';
        try {
            if (isAdd && selectedServerId) {
                await createInterface(selectedServerId, config);
            } else if (interfaceModal?.mode === 'edit' && interfaceModal.interfaceId) {
                await updateInterfaceConfig(selectedServerId!, interfaceModal.interfaceId, config);
            } else {
                return;
            }
            toast.success(isAdd ? t('servers.interfaces.added') : t('servers.interfaces.updated'));
            setInterfaceModal(null);
            await loadInterfaces();
        } catch (err) {
            // Surface the specific backend reason (e.g. a name/port/subnet
            // conflict) instead of a generic failure.
            const fallback = isAdd ? t('servers.interfaces.addError') : t('servers.interfaces.updateError');
            toast.error(err instanceof Error && err.message ? err.message : fallback);
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
                        <button
                            onClick={() => setShowAgentModal(true)}
                            className={cn(buttons.secondary, server.agent?.profilingEnabled && 'text-amber-600 dark:text-amber-400')}
                            title={t('servers.agentModal.open')}
                        >
                            <Wrench size={14}/>
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
                                                {iface.disabled && (
                                                    <span className="rounded bg-muted px-1.5 py-0.5 text-xs font-medium text-muted-foreground dark:bg-white/10 dark:text-zinc-400">
                                                        {t('servers.interfaces.disabledBadge')}
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
                                                            enabled: iface.disabled !== true,
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
                                                    onClick={() => { setDeleteInterfaceConfirmed(false); setDeleteInterfaceId(iface.id); }}
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

            {showAgentModal && server && (
                <AgentModal
                    server={server}
                    onClose={() => setShowAgentModal(false)}
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

            {deleteInterfaceId && (() => {
                const iface = serverInterfaces.find((i) => i.id === deleteInterfaceId);
                // Count from the reactive store's users (peers live under users,
                // tagged with their interface id — the same thing the backend
                // cascade-deletes), so a peer added on the Users page is seen
                // even though serverInterfaces was loaded on mount. iface.peers is
                // a fallback for when the users list hasn't loaded yet.
                const usersPeerCount = users.reduce(
                    (n, u) => n + (u.peers?.filter((p) => p.interface === deleteInterfaceId).length ?? 0),
                    0,
                );
                const peerCount = Math.max(usersPeerCount, iface?.peers?.length ?? 0);
                const closeDelete = () => {
                    setDeleteInterfaceId(null);
                    setDeleteInterfaceConfirmed(false);
                };
                // An interface with peers needs a double confirmation: the first
                // dialog spells out that all its peers will be deleted too, the
                // second is the final go-ahead. No peers → a single confirmation.
                if (peerCount > 0 && !deleteInterfaceConfirmed) {
                    return (
                        <ConfirmModal
                            title={t('servers.interfaces.deleteTooltip')}
                            message={t('servers.interfaces.confirmDeleteWithPeers', {
                                count: peerCount,
                                name: iface?.iface ?? '',
                            })}
                            confirmLabel={t('common.continue')}
                            onConfirm={() => setDeleteInterfaceConfirmed(true)}
                            onCancel={closeDelete}
                        />
                    );
                }
                return (
                    <ConfirmModal
                        title={t('servers.interfaces.deleteTooltip')}
                        message={peerCount > 0
                            ? t('servers.interfaces.confirmDeleteWithPeersFinal', {count: peerCount})
                            : t('servers.interfaces.confirmDelete')}
                        onConfirm={handleDeleteInterface}
                        onCancel={closeDelete}
                        loading={deleteInterfaceLoading}
                    />
                );
            })()}
        </div>
    );
}
