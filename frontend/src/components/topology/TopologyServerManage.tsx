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

import {type FormEvent, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Edit2, Network, Plus, Trash2} from 'lucide-react';
import {toast} from 'sonner';
import {Container} from '@/components/common/Container';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {FormField} from '@/components/common/FormField';
import {Modal} from '@/components/common/Modal';
import {buttons, inputs} from '@/components/common/controls';
import {
    createNetworkInterface,
    deleteNetworkInterface,
    topologyCoordinates,
    updateNetworkInterface,
    updateTopologyServer,
} from '@/services/topology';
import type {NetworkInterface, Topology, TopologyServer} from '@/types';

interface Props {
    server: TopologyServer;
    topology: Topology;
    section: 'information' | 'interfaces';
    onChanged: () => Promise<void>;
}

function errorMessage(error: unknown, fallback: string): string {
    return error instanceof Error ? error.message : fallback;
}

function InterfaceEditor({serverId, iface, onClose, onChanged}: {
    serverId: string;
    iface?: NetworkInterface;
    onClose: () => void;
    onChanged: () => Promise<void>;
}) {
    const {t} = useTranslation();
    const [name, setName] = useState(iface?.name ?? '');
    const [address, setAddress] = useState(iface?.address ?? '');
    const [saving, setSaving] = useState(false);

    const save = async (event: FormEvent) => {
        event.preventDefault();
        setSaving(true);
        try {
            if (iface) {
                await updateNetworkInterface(iface.id, {name, address});
            } else {
                await createNetworkInterface(serverId, {name, address});
            }
            await onChanged();
            toast.success(t('topology.saved'));
            onClose();
        } catch (error) {
            toast.error(errorMessage(error, t('topology.saveError')));
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal title={iface ? t('servers.interfaces.editTitle') : t('servers.interfaces.addTitle')} onClose={onClose} loading={saving}>
            <form onSubmit={save} className="space-y-4">
                <FormField label={t('topology.interfaceName')}>
                    <input autoFocus required value={name} onChange={event => setName(event.target.value)} placeholder="vpn0" disabled={saving} className={inputs.primary} />
                </FormField>
                <FormField label={t('topology.interfaceAddress')}>
                    <input value={address} onChange={event => setAddress(event.target.value)} placeholder="10.0.0.1/24" disabled={saving} className={inputs.primary} />
                </FormField>
                <div className="flex justify-end gap-2 pt-2">
                    <button type="button" onClick={onClose} disabled={saving} className={buttons.secondary}>{t('common.cancel')}</button>
                    <button type="submit" disabled={saving || !name.trim()} className={buttons.primary}>{saving ? t('common.saving') : t('common.save')}</button>
                </div>
            </form>
        </Modal>
    );
}

function ServerInformation({server, onChanged}: {server: TopologyServer; onChanged: () => Promise<void>}) {
    const {t} = useTranslation();
    const [name, setName] = useState(server.name);
    const [address, setAddress] = useState(server.address ?? '');
    const [description, setDescription] = useState(server.description ?? '');
    const [location, setLocation] = useState(server.location ?? '');
    const [latitude, setLatitude] = useState(server.latitude?.toString() ?? '');
    const [longitude, setLongitude] = useState(server.longitude?.toString() ?? '');
    const [saving, setSaving] = useState(false);

    const save = async (event: FormEvent) => {
        event.preventDefault();
        setSaving(true);
        try {
            await updateTopologyServer(server.id, {
                name,
                address,
                description,
                location,
                ...topologyCoordinates(latitude, longitude),
                interfaces: [],
            });
            await onChanged();
            toast.success(t('topology.saved'));
        } catch (error) {
            toast.error(errorMessage(error, t('topology.saveError')));
        } finally {
            setSaving(false);
        }
    };

    return (
        <Container title={t('servers.information')}>
            <form onSubmit={save} className="space-y-4">
                <div className="grid gap-4 md:grid-cols-2">
                    <FormField label={t('topology.serverName')}><input required value={name} onChange={event => setName(event.target.value)} disabled={saving} className={inputs.primary} /></FormField>
                    <FormField label={t('topology.serverAddress')}><input value={address} onChange={event => setAddress(event.target.value)} disabled={saving} className={inputs.primary} /></FormField>
                </div>
                <FormField label={t('topology.descriptionField')}><textarea rows={3} value={description} onChange={event => setDescription(event.target.value)} disabled={saving} className={inputs.primary} /></FormField>
                <FormField label={t('servers.location')}><input value={location} onChange={event => {setLocation(event.target.value); setLatitude(''); setLongitude('');}} placeholder={t('servers.locationPlaceholder')} disabled={saving} className={inputs.primary} /></FormField>
                <p className="-mt-2 text-xs text-muted-foreground">{t('topology.locationHint')}</p>
                <div className="grid grid-cols-2 gap-3">
                    <FormField label={t('topology.latitude')}><input type="number" min={-90} max={90} step="any" value={latitude} onChange={event => setLatitude(event.target.value)} placeholder="55.7558" disabled={saving || Boolean(location.trim())} className={`${inputs.primary} disabled:cursor-not-allowed disabled:opacity-50`} /></FormField>
                    <FormField label={t('topology.longitude')}><input type="number" min={-180} max={180} step="any" value={longitude} onChange={event => setLongitude(event.target.value)} placeholder="37.6173" disabled={saving || Boolean(location.trim())} className={`${inputs.primary} disabled:cursor-not-allowed disabled:opacity-50`} /></FormField>
                </div>
                <div className="flex justify-end"><button type="submit" disabled={saving || !name.trim()} className={buttons.primary}>{saving ? t('common.saving') : t('common.save')}</button></div>
            </form>
        </Container>
    );
}

function ServerInterfaces({server, topology, onChanged}: {server: TopologyServer; topology: Topology; onChanged: () => Promise<void>}) {
    const {t} = useTranslation();
    const [editor, setEditor] = useState<{iface?: NetworkInterface} | null>(null);
    const [deleting, setDeleting] = useState<NetworkInterface | null>(null);
    const [busy, setBusy] = useState(false);
    const interfaces = topology.interfaces.filter(iface => iface.serverId === server.id);

    const remove = async () => {
        if (!deleting) return;
        setBusy(true);
        try {
            await deleteNetworkInterface(deleting.id);
            await onChanged();
            setDeleting(null);
            toast.success(t('topology.deleted'));
        } catch (error) {
            toast.error(errorMessage(error, t('topology.deleteError')));
        } finally {
            setBusy(false);
        }
    };

    return (
        <div className="rounded-xl border border-border bg-card p-6 dark:border-white/5 dark:bg-white/3">
            <div className="mb-4 flex items-center justify-between">
                <h2 className="flex items-center gap-2 text-lg font-semibold text-foreground dark:text-zinc-100"><Network size={20} />{t('topology.interfaces')} ({interfaces.length})</h2>
                <button type="button" onClick={() => setEditor({})} className={buttons.primary} title={t('servers.addInterfaceTooltip')}><Plus size={14} /></button>
            </div>
            {interfaces.length > 0 ? (
                <div className="space-y-3">
                    {interfaces.map(iface => (
                        <div key={iface.id} className="flex items-center justify-between gap-4 rounded-lg border border-input bg-background p-4 dark:border-white/10 dark:bg-white/5">
                            <div className="min-w-0">
                                <div className="flex items-center gap-2 text-sm font-semibold"><Network size={16} />{iface.name}</div>
                                {iface.address && <div className="mt-1 font-mono text-xs text-muted-foreground">{iface.address}</div>}
                            </div>
                            <div className="flex shrink-0 gap-2">
                                <button type="button" onClick={() => setEditor({iface})} title={t('servers.interfaces.editTooltip')} className="rounded p-1 text-muted-foreground hover:bg-sky-100 hover:text-sky-600 dark:hover:bg-sky-500/10 dark:hover:text-sky-400"><Edit2 size={16} /></button>
                                <button type="button" onClick={() => setDeleting(iface)} title={t('servers.interfaces.deleteTooltip')} className="rounded p-1 text-muted-foreground hover:bg-red-100 hover:text-red-600 dark:hover:bg-red-500/10 dark:hover:text-red-400"><Trash2 size={16} /></button>
                            </div>
                        </div>
                    ))}
                </div>
            ) : <div className="py-4 text-center text-muted-foreground dark:text-zinc-500">{t('servers.interfaces.noInterfaces')}</div>}
            {editor && <InterfaceEditor serverId={server.id} iface={editor.iface} onClose={() => setEditor(null)} onChanged={onChanged} />}
            {deleting && <ConfirmModal title={t('topology.deleteInterface')} message={t('topology.confirmDeleteInterface')} onCancel={() => setDeleting(null)} onConfirm={remove} loading={busy} />}
        </div>
    );
}

export function TopologyServerManage({server, topology, section, onChanged}: Props) {
    return section === 'information'
        ? <ServerInformation key={`${server.id}:${server.location ?? ''}:${server.latitude ?? ''}:${server.longitude ?? ''}`} server={server} onChanged={onChanged} />
        : <ServerInterfaces server={server} topology={topology} onChanged={onChanged} />;
}
