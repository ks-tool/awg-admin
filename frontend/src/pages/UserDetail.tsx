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
import {useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {toast} from 'sonner';
import {ArrowLeft, Download, Key, Plus, QrCode, Trash2, User as UserIcon, UserPen} from 'lucide-react';
import {PageHeader} from '@/components/layout/PageHeader';
import {useNavigation} from '@/contexts/NavigationContext';
import {useAppStore} from '@/store';
import {listInterfaces} from '@/services/interfaces';
import {getPeerConfig, getPeerQRCode, savePeerQRCode} from '@/services/peers';
import {CopyButton} from '@/components/common/CopyButton';
import {StatusBadge} from '@/components/common/StatusBadge';
import {Container} from '@/components/common/Container';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs, Modal} from '@/components/common/Modal';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {cn} from '@/lib/utils';
import type {Server} from '@/types';

// ---------------------------------------------------------------------------
// Peer QR code modal
// ---------------------------------------------------------------------------

function PeerQRModal({userId, publicKey, peerName, onClose}: {
    userId: string;
    publicKey: string;
    peerName: string;
    onClose: () => void;
}) {
    const {t} = useTranslation();
    const [config, setConfig] = useState('');
    const [qrDataUrl, setQrDataUrl] = useState('');
    const [loading, setLoading] = useState(true);

    // The QR image is rendered server-side (Service.GetPeerQRCode) from the
    // same config text fetched here for the "copy as text" box below — the
    // config (which embeds the peer's private key) never needs to be
    // handed to a client-side QR library, it only has to render the pixels
    // it already produced for the other use.
    useEffect(() => {
        let cancelled = false;
        (async () => {
            const [cfg, qrBase64] = await Promise.all([
                getPeerConfig(userId, publicKey),
                getPeerQRCode(userId, publicKey),
            ]);
            if (cancelled) return;
            if (!cfg || !qrBase64) {
                toast.error(t('peers.generateConfigError'));
                setLoading(false);
                return;
            }
            setConfig(cfg);
            setQrDataUrl(`data:image/png;base64,${qrBase64}`);
            setLoading(false);
        })();
        return () => {
            cancelled = true;
        };
    }, [userId, publicKey]);

    // Save the (already-rendered, server-produced) PNG to a file. The browser
    // and the Wails desktop webview need different mechanisms — savePeerQRCode
    // picks the right one (native save dialog on desktop, anchor download in a
    // plain browser); see its doc comment.
    const handleDownload = async () => {
        if (!qrDataUrl) return;
        // Strip only characters that are actually illegal in filenames (keep
        // Unicode letters, e.g. Cyrillic peer names, intact), then trim stray
        // separators so the result is never empty.
        const safeName = (peerName || 'peer')
            .replace(/[/\\:*?"<>|\x00-\x1f]+/g, '_')
            .replace(/^[.\s]+|[.\s]+$/g, '') || 'peer';
        const saved = await savePeerQRCode(userId, publicKey, safeName, qrDataUrl);
        if (saved) {
            toast.success(t('peers.qrCodeSaved'));
        }
    };

    return (
        <Modal title={t('peers.qrCodeTitle', {name: peerName})} onClose={onClose}>
            <div className="flex flex-col items-center gap-4">
                {loading ? (
                    <div className="py-12 text-sm text-muted-foreground dark:text-zinc-500">{t('common.generating')}</div>
                ) : qrDataUrl ? (
                    <>
                        <img src={qrDataUrl} alt="WireGuard peer QR code" className="rounded-lg bg-white p-2"/>
                        <div className="flex items-center gap-2 w-full">
                            <textarea
                                readOnly
                                value={config}
                                rows={6}
                                className={cn(inputs.primary, 'resize-none font-mono text-xs')}
                            />
                            <CopyButton value={config}/>
                        </div>
                        <button
                            onClick={handleDownload}
                            className={cn('w-full inline-flex items-center justify-center gap-2', buttons.primary)}
                        >
                            <Download size={16}/>
                            {t('peers.downloadQrCode')}
                        </button>
                    </>
                ) : (
                    <div className="py-12 text-sm text-red-500">{t('peers.configGenerateFailed')}</div>
                )}
                <button onClick={onClose} className={cn('w-full', buttons.secondary)}>
                    {t('common.close')}
                </button>
            </div>
        </Modal>
    );
}

// ---------------------------------------------------------------------------
// Add Peer modal (mirrors ServerDetail's InterfaceFormModal)
// ---------------------------------------------------------------------------

const INITIAL_PEER_FORM = {
    name: '',
    serverId: '',
    interfaceId: '',
    allowedIps: '',
    endpoint: '',
    dns: '',
    privateKey: '',
    presharedKey: '',
    withPresharedKey: true,
};
type PeerFormData = typeof INITIAL_PEER_FORM;

function AddPeerModal({
    servers,
    onSubmit,
    onClose,
    loading,
}: {
    servers: Server[];
    onSubmit: (data: PeerFormData) => Promise<void>;
    onClose: () => void;
    loading: boolean;
}) {
    const {t} = useTranslation();
    const [form, setForm] = useState<PeerFormData>(INITIAL_PEER_FORM);
    const [interfaces, setInterfaces] = useState<Array<{ id: string; name: string; addr: string }>>([]);

    useEffect(() => {
        if (!form.serverId) {
            setInterfaces([]);
            return;
        }
        let cancelled = false;
        (async () => {
            const list = await listInterfaces(form.serverId);
            if (!cancelled) {
                setInterfaces(
                    list ? list.map(iface => ({id: iface.id, name: iface.iface, addr: iface.addr || ''})) : [],
                );
            }
        })();
        return () => {
            cancelled = true;
        };
    }, [form.serverId]);

    const handleChange = (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement>) => {
        const {name, value, type} = e.target;
        const val = type === 'checkbox' ? (e.target as HTMLInputElement).checked : value;
        setForm(prev => ({
            ...prev,
            [name]: val,
            // changing server invalidates the previously selected interface
            ...(name === 'serverId' ? {interfaceId: ''} : {}),
        }));
    };

    const handleSubmit = async () => {
        if (!form.interfaceId) return toast.error(t('peers.interfaceRequired'));
        await onSubmit(form);
    };

    return (
        <Modal title={t('peers.addPeer')} onClose={onClose} loading={loading}>
            <div className="space-y-4">
                <FormField label={`${t('common.name')} (${t('common.optional')})`}>
                    <input
                        type="text"
                        name="name"
                        value={form.name}
                        onChange={handleChange}
                        placeholder={t('peers.namePlaceholder')}
                        disabled={loading}
                        className={inputs.primary}
                    />
                </FormField>

                <FormField label={`${t('peers.server')} (${t('common.required')})`}>
                    <select
                        name="serverId"
                        value={form.serverId}
                        onChange={handleChange}
                        disabled={loading}
                        className={inputs.primary}
                    >
                        <option value="">{t('peers.selectServer')}</option>
                        {servers.map(server => (
                            <option key={server.id} value={server.id}>{server.name}</option>
                        ))}
                    </select>
                </FormField>

                <FormField label={`${t('servers.interfaces.name')} (${t('common.required')})`}>
                    <select
                        name="interfaceId"
                        value={form.interfaceId}
                        onChange={handleChange}
                        disabled={loading || !form.serverId}
                        className={inputs.primary}
                    >
                        <option value="">
                            {form.serverId ? t('peers.selectInterface') : t('peers.selectServerFirst')}
                        </option>
                        {interfaces.map(iface => (
                            <option key={iface.id} value={iface.id}>{iface.name} ({iface.addr})</option>
                        ))}
                    </select>
                </FormField>

                <FormField label={`${t('peers.allowedIPs')} (${t('common.optional')})`}>
                    <input
                        type="text"
                        name="allowedIps"
                        value={form.allowedIps}
                        onChange={handleChange}
                        placeholder={t('peers.allowedIpsAutoPlaceholder')}
                        disabled={loading}
                        className={inputs.primary}
                    />
                </FormField>

                <FormField label={`${t('peers.endpoint')} (${t('common.optional')})`}>
                    <input
                        type="text"
                        name="endpoint"
                        value={form.endpoint}
                        onChange={handleChange}
                        placeholder="203.0.113.0:51820"
                        disabled={loading}
                        className={inputs.primary}
                    />
                </FormField>

                <FormField label={`${t('peers.dns')} (${t('common.optional')})`}>
                    <input
                        type="text"
                        name="dns"
                        value={form.dns}
                        onChange={handleChange}
                        placeholder={t('peers.dnsPlaceholder')}
                        disabled={loading}
                        className={inputs.primary}
                    />
                </FormField>

                <FormField label={`${t('peers.privateKey')} (${t('common.optional')})`}>
                    <input
                        type="text"
                        name="privateKey"
                        value={form.privateKey}
                        onChange={handleChange}
                        placeholder={t('peers.privateKeyPlaceholder')}
                        disabled={loading}
                        className={inputs.primary}
                    />
                </FormField>

                <FormField label="">
                    <label className="flex items-center cursor-pointer">
                        <input
                            type="checkbox"
                            name="withPresharedKey"
                            checked={form.withPresharedKey}
                            onChange={handleChange}
                            disabled={loading}
                            className="rounded border-input bg-background text-sky-500 focus:ring-sky-500 focus:ring-offset-0 disabled:opacity-50 dark:border-white/10 dark:bg-white/5"
                        />
                        <span className="ml-3 text-sm font-medium text-foreground dark:text-zinc-300">
                            {t('peers.generatePresharedKey')}
                        </span>
                    </label>
                </FormField>

                {!form.withPresharedKey && (
                    <FormField label={`${t('peers.presharedKey')} (${t('common.optional')})`}>
                        <input
                            type="text"
                            name="presharedKey"
                            value={form.presharedKey}
                            onChange={handleChange}
                            placeholder={t('peers.presharedKeyPlaceholder')}
                            disabled={loading}
                            className={inputs.primary}
                        />
                    </FormField>
                )}

                <div className="flex gap-3 pt-4">
                    <button
                        onClick={handleSubmit}
                        disabled={loading || !form.interfaceId}
                        className={cn('flex-1', buttons.primary)}
                    >
                        {loading ? `${t('common.saving')}...` : t('peers.addPeer')}
                    </button>
                    <button onClick={onClose} disabled={loading} className={cn('flex-1', buttons.secondary)}>
                        {t('common.cancel')}
                    </button>
                </div>
            </div>
        </Modal>
    );
}

// ---------------------------------------------------------------------------
// UserDetail
// ---------------------------------------------------------------------------

export default function UserDetail() {
    const {t} = useTranslation();
    const {navigate} = useNavigation();
    const {
        users,
        servers,
        selectedUserId,
        setSelectedUser,
        updateUser,
        deleteUser,
        addPeer,
        deletePeer,
        refreshData,
        getInterfaceName,
    } = useAppStore();

    const [isLoading, setIsLoading] = useState(false);
    const [isEditing, setIsEditing] = useState(false);
    const [showAddPeerModal, setShowAddPeerModal] = useState(false);
    const [qrPeer, setQrPeer] = useState<{publicKey: string; name: string} | null>(null);
    const [addPeerLoading, setAddPeerLoading] = useState(false);
    const [deletePeerKey, setDeletePeerKey] = useState<string | null>(null);
    const [deletePeerLoading, setDeletePeerLoading] = useState(false);
    const [showDeleteUserConfirm, setShowDeleteUserConfirm] = useState(false);
    const [deleteUserLoading, setDeleteUserLoading] = useState(false);
    const [formData, setFormData] = useState({
        name: '',
        description: '',
        disabled: false,
    });

    const user = users.find(u => u.id === selectedUserId);

    /* ---- Effects ---------------------------------------------------------- */

    useEffect(() => {
        if (user) {
            setFormData({
                name: user.name || '',
                description: user.description || '',
                disabled: user.disabled || false,
            });
        }
    }, [user]);

    /* ---- Handlers --------------------------------------------------------- */

    const handleInputChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
        const {name, value, type} = e.target;
        const val = type === 'checkbox' ? (e.target as HTMLInputElement).checked : value;
        setFormData(prev => ({...prev, [name]: val}));
    };

    const handleSave = async () => {
        if (!selectedUserId || !formData.name.trim()) {
            toast.error(t('users.nameRequired'));
            return;
        }
        setIsLoading(true);
        try {
            const updated = await updateUser(selectedUserId, {
                name: formData.name,
                description: formData.description || undefined,
                disabled: formData.disabled,
            });
            if (updated) {
                toast.success(t('users.userUpdated'));
                setIsEditing(false);
            } else {
                toast.error(t('users.updateUserError'));
            }
        } catch (error) {
            console.error('Failed to update user:', error);
            toast.error(t('users.updateUserError'));
        } finally {
            setIsLoading(false);
        }
    };

    const handleCancel = () => {
        if (user) {
            setFormData({
                name: user.name || '',
                description: user.description || '',
                disabled: user.disabled || false,
            });
        }
        setIsEditing(false);
    };

    const handleAddPeer = async (form: PeerFormData) => {
        if (!selectedUserId) {
            toast.error(t('users.userNotSelected'));
            return;
        }

        setAddPeerLoading(true);
        try {
            const allowedIpsArray = form.allowedIps
                .split(',')
                .map(ip => ip.trim())
                .filter(Boolean);

            const dnsArray = form.dns
                .split(',')
                .map(d => d.trim())
                .filter(Boolean);

            const success = await addPeer(selectedUserId, {
                name: form.name,
                interfaceId: form.interfaceId,
                allowedIps: allowedIpsArray,
                endpoint: form.endpoint || undefined,
                dns: dnsArray.length ? dnsArray : undefined,
                privateKey: form.privateKey.trim() || undefined,
                presharedKey: form.withPresharedKey ? undefined : form.presharedKey.trim() || undefined,
                withPresharedKey: form.withPresharedKey,
            });

            if (success) {
                toast.success(t('peers.peerAdded'));
                setShowAddPeerModal(false);
                await refreshData();
            } else {
                toast.error(t('peers.addPeerError'));
            }
        } catch (error) {
            console.error('Failed to add peer:', error);
            toast.error(t('peers.addPeerError'));
        } finally {
            setAddPeerLoading(false);
        }
    };

    const handleDeletePeer = async () => {
        if (!selectedUserId || !deletePeerKey) return;
        setDeletePeerLoading(true);
        try {
            const success = await deletePeer(selectedUserId, deletePeerKey);
            if (success) {
                toast.success(t('peers.deleted'));
                await refreshData();
            } else {
                toast.error(t('peers.deleteError'));
            }
        } catch (error) {
            console.error('Failed to delete peer:', error);
            toast.error(t('peers.deleteError'));
        } finally {
            setDeletePeerLoading(false);
            setDeletePeerKey(null);
        }
    };

    const handleBack = () => {
        setSelectedUser(null);
        navigate('users');
    };

    const handleDeleteUser = async () => {
        if (!selectedUserId) return;
        setDeleteUserLoading(true);
        try {
            const success = await deleteUser(selectedUserId);
            if (success) {
                toast.success(t('users.userDeleted'));
                handleBack();
            } else {
                toast.error(t('users.deleteUserError'));
            }
        } catch (error) {
            console.error('Failed to delete user:', error);
            toast.error(t('users.deleteUserError'));
        } finally {
            setDeleteUserLoading(false);
            setShowDeleteUserConfirm(false);
        }
    };

    const getServerForInterface = (interfaceId: string) =>
        servers.find(server => server.interfaces?.includes(interfaceId));

    /* ---- Render: empty state ---------------------------------------------- */

    if (!user) {
        return (
            <div className="flex flex-col">
                <PageHeader
                    title={t('users.title')}
                    icon={UserIcon}
                    description={t('users.noUser')}
                    actions={
                        <button onClick={handleBack} className={buttons.primary} title={t('users.backToUsers')}>
                            <ArrowLeft size={14}/>
                        </button>
                    }
                />
                <div className="p-8 text-center text-muted-foreground">{t('users.noUser')}</div>
            </div>
        );
    }

    /* ---- Render: main ----------------------------------------------------- */

    return (
        <div className="flex flex-col">
            <PageHeader
                title={user.name || t('users.noName')}
                icon={UserIcon}
                description={user.id}
                actions={
                    <div className="flex items-center gap-2">
                        <button
                            onClick={() => setShowDeleteUserConfirm(true)}
                            className={buttons.danger}
                            title={t('users.deleteUser')}
                        >
                            <Trash2 size={14}/>
                        </button>
                        <button onClick={handleBack} className={buttons.primary} title={t('users.backToUsers')}>
                            <ArrowLeft size={14}/>
                        </button>
                    </div>
                }
            />

            <div className="p-8 space-y-6">
                {/* User Information */}
                <Container
                    title={t('users.information')}
                    action={
                        !isEditing && (
                            <button onClick={() => setIsEditing(true)} className={buttons.primary}>
                                <UserPen size={14}/>
                            </button>
                        )
                    }
                >
                    {isEditing ? (
                        <form className="space-y-4">
                            <FormField label={t('common.name')}>
                                <input
                                    type="text"
                                    name="name"
                                    value={formData.name}
                                    onChange={handleInputChange}
                                    disabled={isLoading}
                                    className={inputs.primary}
                                />
                            </FormField>

                            <FormField label={t('common.description')}>
                <textarea
                    name="description"
                    value={formData.description}
                    onChange={handleInputChange}
                    disabled={isLoading}
                    rows={4}
                    className={cn(inputs.primary, "resize-none")}
                />
                            </FormField>

                            <FormField label="">
                                <label className="flex items-center cursor-pointer">
                                    <input
                                        type="checkbox"
                                        name="disabled"
                                        checked={formData.disabled}
                                        onChange={handleInputChange}
                                        disabled={isLoading}
                                        className="rounded border-input bg-background text-red-500 focus:ring-red-500 focus:ring-offset-0 disabled:opacity-50 dark:border-white/10 dark:bg-white/5"
                                    />
                                    <span className="ml-3 text-sm font-medium text-foreground dark:text-zinc-300">
                                        {t('common.disabled')}
                                    </span>
                                </label>
                            </FormField>

                            <div className="flex gap-3 pt-4">
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
                        <div className="flex items-center gap-4 text-sm">
              <span className="text-foreground font-semibold flex-1 dark:text-zinc-100">
                {user.description || '—'}
              </span>
                            <span className="text-muted-foreground dark:text-zinc-400">|</span>
                            <StatusBadge status={!user.disabled ? 'enabled' : 'disabled'} size="sm"/>
                        </div>
                    )}
                </Container>

                {/* Peers */}
                <Container
                    title={
                        <span
                            className="flex items-center gap-2 text-lg font-semibold text-foreground dark:text-zinc-100">
                            <Key size={20}/>
                            {t('users.peers')} ({user?.peers?.length || 0})
                        </span>
                    }
                    action={
                        <button onClick={() => setShowAddPeerModal(true)} className={buttons.primary} title={t('peers.addPeer')}>
                            <Plus size={14}/>
                        </button>
                    }
                >
                    {user?.peers && user.peers.length > 0 ? (
                        <div className="space-y-3">
                            {user.peers.map((peer, index) => (
                                <div
                                    key={index}
                                    className="rounded-lg border border-input bg-background p-4 dark:border-white/10 dark:bg-white/5"
                                >
                                    <div className="flex items-center justify-between mb-2">
                                        <div
                                            className="text-sm font-medium text-foreground dark:text-zinc-300">{peer.name}</div>
                                        <div className="flex items-center gap-2">
                                            {/*<CopyButton value={peer.pk}/>*/}
                                            <button
                                                onClick={() => setQrPeer({publicKey: peer.pk, name: peer.name})}
                                                className="p-1 text-muted-foreground hover:text-foreground dark:text-zinc-500 dark:hover:text-zinc-300 rounded transition-colors"
                                                title={t('peers.showQrCode')}
                                            >
                                                <QrCode size={16}/>
                                            </button>
                                            <button
                                                onClick={() => setDeletePeerKey(peer.pk)}
                                                className="p-1 text-muted-foreground hover:text-red-600 dark:hover:text-red-400 hover:bg-red-100 dark:hover:bg-red-500/10 rounded transition-colors"
                                                title={t('peers.deletePeerTooltip')}
                                            >
                                                <Trash2 size={16}/>
                                            </button>
                                        </div>
                                    </div>
                                    <div
                                        className="flex items-center justify-between text-xs text-muted-foreground dark:text-zinc-500">
                    <span className="text-foreground font-semibold dark:text-zinc-300">
                      {t('peers.server')}: {getServerForInterface(peer.interface)?.name}/
                        {getInterfaceName(peer.interface) || peer.interface}
                    </span>
                                        {peer.disabled &&
                                            <span className="text-orange-500">{t('common.disabled')}</span>}
                                    </div>
                                </div>
                            ))}
                        </div>
                    ) : (
                        <div
                            className="text-center text-muted-foreground dark:text-zinc-500 py-4">{t('users.noPeers')}</div>
                    )}
                </Container>
            </div>

            {showAddPeerModal && (
                <AddPeerModal
                    servers={servers}
                    onSubmit={handleAddPeer}
                    onClose={() => setShowAddPeerModal(false)}
                    loading={addPeerLoading}
                />
            )}

            {qrPeer && selectedUserId && (
                <PeerQRModal
                    userId={selectedUserId}
                    publicKey={qrPeer.publicKey}
                    peerName={qrPeer.name}
                    onClose={() => setQrPeer(null)}
                />
            )}

            {showDeleteUserConfirm && (
                <ConfirmModal
                    title={t('users.deleteUser')}
                    message={t('users.confirmDeleteUser')}
                    onConfirm={handleDeleteUser}
                    onCancel={() => setShowDeleteUserConfirm(false)}
                    loading={deleteUserLoading}
                />
            )}

            {deletePeerKey && (
                <ConfirmModal
                    title={t('peers.deletePeerTooltip')}
                    message={t('peers.confirmDelete')}
                    onConfirm={handleDeletePeer}
                    onCancel={() => setDeletePeerKey(null)}
                    loading={deletePeerLoading}
                />
            )}
        </div>
    );
}
