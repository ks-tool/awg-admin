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

import {type FormEvent, useCallback, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {Pencil, Play, Plus, RefreshCw, Trash2, X} from 'lucide-react';
import {toast} from 'sonner';
import {buttons, inputs} from '@/components/common/controls';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {FormField} from '@/components/common/FormField';
import {SchemaForm} from './SchemaForm';
import {
    applyVPNService,
    createNetworkInterface,
    createVPNServiceUser,
    deleteNetworkInterface,
    deleteTopologyServer,
    deleteVPNService,
    deleteVPNServiceUser,
    listVPNServiceUsers,
    refreshVPNServiceStatus,
    topologyCoordinates,
    updateNetworkInterface,
    updateTopologyServer,
    updateVPNService,
    updateVPNServiceUser,
} from '@/services/topology';
import type {NetworkInterface, PluginDescriptor, PluginUser, Topology, TopologyServer, VPNService} from '@/types';

export type TopologySelection = {kind: 'server' | 'interface' | 'service'; id: string};

interface InspectorProps {
    selection: TopologySelection;
    topology: Topology;
    plugins: PluginDescriptor[];
    onClose: () => void;
    onSelectService: (id: string) => void;
    refresh: () => Promise<void>;
}

function message(error: unknown, fallback: string): string {
    return error instanceof Error ? error.message : fallback;
}

function InspectorShell({title, subtitle, onClose, children}: {title: string; subtitle?: string; onClose: () => void; children: React.ReactNode}) {
    const {t} = useTranslation();
    return (
        <aside className="flex h-full w-96 shrink-0 flex-col border-l border-border bg-card">
            <div className="flex items-start justify-between border-b border-border p-4">
                <div className="min-w-0">
                    <h2 className="truncate font-semibold">{title}</h2>
                    {subtitle && <p className="mt-1 truncate text-xs text-muted-foreground">{subtitle}</p>}
                </div>
                <button type="button" onClick={onClose} aria-label={t('common.close')} className="rounded-md p-1 text-muted-foreground hover:bg-muted hover:text-foreground"><X size={18} /></button>
            </div>
            <div className="flex-1 overflow-y-auto p-4">{children}</div>
        </aside>
    );
}

function ServerInspector({server, topology, onClose, onSelectService, refresh}: {server: TopologyServer; topology: Topology; onClose: () => void; onSelectService: (id: string) => void; refresh: () => Promise<void>}) {
    const {t} = useTranslation();
    const [name, setName] = useState(server.name);
    const [address, setAddress] = useState(server.address ?? '');
    const [description, setDescription] = useState(server.description ?? '');
    const [location, setLocation] = useState(server.location ?? '');
    const [latitude, setLatitude] = useState(server.latitude?.toString() ?? '');
    const [longitude, setLongitude] = useState(server.longitude?.toString() ?? '');
    const [interfaceName, setInterfaceName] = useState('');
    const [interfaceAddress, setInterfaceAddress] = useState('');
    const [busy, setBusy] = useState(false);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const services = topology.services.filter(service => service.serverId === server.id);

    const save = async (event: FormEvent) => {
        event.preventDefault();
        setBusy(true);
        try {
            await updateTopologyServer(server.id, {name, address, description, location, ...topologyCoordinates(latitude, longitude), interfaces: []});
            await refresh();
            toast.success(t('topology.saved'));
        } catch (error) {
            toast.error(message(error, t('topology.saveError')));
        } finally {
            setBusy(false);
        }
    };
    const addInterface = async (event: FormEvent) => {
        event.preventDefault();
        setBusy(true);
        try {
            await createNetworkInterface(server.id, {name: interfaceName, address: interfaceAddress});
            setInterfaceName('');
            setInterfaceAddress('');
            await refresh();
            toast.success(t('topology.interfaceCreated'));
        } catch (error) {
            toast.error(message(error, t('topology.saveError')));
        } finally {
            setBusy(false);
        }
    };
    const remove = async () => {
        setBusy(true);
        try {
            await deleteTopologyServer(server.id);
            setConfirmDelete(false);
            onClose();
            await refresh();
            toast.success(t('topology.deleted'));
        } catch (error) {
            toast.error(message(error, t('topology.deleteError')));
        } finally {
            setBusy(false);
        }
    };

    return (
        <InspectorShell title={server.name} subtitle={t('topology.server')} onClose={onClose}>
            <form onSubmit={save} className="space-y-4">
                <FormField label={t('topology.serverName')}><input value={name} onChange={event => setName(event.target.value)} required className={inputs.primary} /></FormField>
                <FormField label={t('topology.serverAddress')}><input value={address} onChange={event => setAddress(event.target.value)} className={inputs.primary} /></FormField>
                <FormField label={t('topology.descriptionField')}><textarea value={description} onChange={event => setDescription(event.target.value)} rows={3} className={inputs.primary} /></FormField>
                <FormField label={t('servers.location')}><input aria-label={t('servers.location')} value={location} onChange={event => {setLocation(event.target.value); setLatitude(''); setLongitude('');}} placeholder={t('servers.locationPlaceholder')} className={inputs.primary} /></FormField>
                <p className="-mt-2 text-xs text-muted-foreground">{t('topology.locationHint')}</p>
                <div className="grid grid-cols-2 gap-3">
                    <FormField label={t('topology.latitude')}><input aria-label={t('topology.latitude')} type="number" min={-90} max={90} step="any" value={latitude} onChange={event => setLatitude(event.target.value)} placeholder="55.7558" disabled={Boolean(location.trim())} className={`${inputs.primary} disabled:cursor-not-allowed disabled:opacity-50`} /></FormField>
                    <FormField label={t('topology.longitude')}><input aria-label={t('topology.longitude')} type="number" min={-180} max={180} step="any" value={longitude} onChange={event => setLongitude(event.target.value)} placeholder="37.6173" disabled={Boolean(location.trim())} className={`${inputs.primary} disabled:cursor-not-allowed disabled:opacity-50`} /></FormField>
                </div>
                <button type="submit" disabled={busy} className={`${buttons.primary} w-full`}>{t('common.save')}</button>
            </form>
            <div className="mt-6 border-t border-border pt-4">
                <h3 className="mb-3 text-sm font-semibold">{t('topology.addInterface')}</h3>
                <form onSubmit={addInterface} className="space-y-3">
                    <input aria-label={t('topology.interfaceName')} placeholder="vpn0" value={interfaceName} onChange={event => setInterfaceName(event.target.value)} required className={inputs.primary} />
                    <input aria-label={t('topology.interfaceAddress')} placeholder="10.0.0.1/24" value={interfaceAddress} onChange={event => setInterfaceAddress(event.target.value)} className={inputs.primary} />
                    <button type="submit" disabled={busy} className={`${buttons.secondary} w-full`}><Plus size={14} />{t('topology.addInterface')}</button>
                </form>
            </div>
            {services.length > 0 && (
                <div className="mt-6 border-t border-border pt-4">
                    <h3 className="mb-2 text-sm font-semibold">{t('topology.services')}</h3>
                    {services.map(service => <button key={service.id} type="button" onClick={() => onSelectService(service.id)} className="block w-full rounded-md px-2 py-2 text-left text-sm hover:bg-muted">{service.name}</button>)}
                </div>
            )}
            <div className="mt-6 border-t border-border pt-4">
                <button type="button" onClick={() => setConfirmDelete(true)} className={`${buttons.danger} w-full`}><Trash2 size={14} />{t('topology.deleteServer')}</button>
            </div>
            {confirmDelete && <ConfirmModal title={t('topology.deleteServer')} message={t('topology.confirmDeleteServer')} onCancel={() => setConfirmDelete(false)} onConfirm={remove} loading={busy} />}
        </InspectorShell>
    );
}

function InterfaceInspector({iface, topology, onClose, onSelectService, refresh}: {iface: NetworkInterface; topology: Topology; onClose: () => void; onSelectService: (id: string) => void; refresh: () => Promise<void>}) {
    const {t} = useTranslation();
    const [name, setName] = useState(iface.name);
    const [address, setAddress] = useState(iface.address ?? '');
    const [busy, setBusy] = useState(false);
    const [confirmDelete, setConfirmDelete] = useState(false);
    const server = topology.servers.find(item => item.id === iface.serverId);
    const service = topology.services.find(item => item.id === iface.serviceId);

    const save = async (event: FormEvent) => {
        event.preventDefault();
        setBusy(true);
        try {
            await updateNetworkInterface(iface.id, {name, address});
            await refresh();
            toast.success(t('topology.saved'));
        } catch (error) {
            toast.error(message(error, t('topology.saveError')));
        } finally {
            setBusy(false);
        }
    };
    const remove = async () => {
        setBusy(true);
        try {
            await deleteNetworkInterface(iface.id);
            setConfirmDelete(false);
            onClose();
            await refresh();
            toast.success(t('topology.deleted'));
        } catch (error) {
            toast.error(message(error, t('topology.deleteError')));
        } finally {
            setBusy(false);
        }
    };

    return (
        <InspectorShell title={iface.name} subtitle={server?.name} onClose={onClose}>
            <form onSubmit={save} className="space-y-4">
                <FormField label={t('topology.interfaceName')}><input value={name} onChange={event => setName(event.target.value)} required className={inputs.primary} /></FormField>
                <FormField label={t('topology.interfaceAddress')}><input value={address} onChange={event => setAddress(event.target.value)} placeholder="10.0.0.1/24" className={inputs.primary} /></FormField>
                <button type="submit" disabled={busy} className={`${buttons.primary} w-full`}>{t('common.save')}</button>
            </form>
            {service && (
                <div className="mt-6 border-t border-border pt-4">
                    <h3 className="mb-2 text-sm font-semibold">{t('topology.attachedService')}</h3>
                    <button type="button" onClick={() => onSelectService(service.id)} className="w-full rounded-md bg-sky-500/10 px-3 py-2 text-left text-sm text-sky-600 dark:text-sky-400">{service.name}</button>
                </div>
            )}
            <div className="mt-6 border-t border-border pt-4">
                <button type="button" onClick={() => setConfirmDelete(true)} className={`${buttons.danger} w-full`}><Trash2 size={14} />{t('topology.deleteInterface')}</button>
            </div>
            {confirmDelete && <ConfirmModal title={t('topology.deleteInterface')} message={t('topology.confirmDeleteInterface')} onCancel={() => setConfirmDelete(false)} onConfirm={remove} loading={busy} />}
        </InspectorShell>
    );
}

function userLabel(user: PluginUser): string {
    const name = user.data.name ?? user.data.username ?? user.data.publicKey;
    return typeof name === 'string' && name ? name : user.id;
}

function ServiceInspector({service, topology, plugins, onClose, refresh}: {service: VPNService; topology: Topology; plugins: PluginDescriptor[]; onClose: () => void; refresh: () => Promise<void>}) {
    const {t} = useTranslation();
    const plugin = plugins.find(item => item.id === service.pluginId);
    const [tab, setTab] = useState<'config' | 'users'>('config');
    const [name, setName] = useState(service.name);
    const [interfaceId, setInterfaceId] = useState(service.interfaceId ?? '');
    const [config, setConfig] = useState<Record<string, unknown>>(service.config);
    const [users, setUsers] = useState<PluginUser[]>([]);
    const [usersLoading, setUsersLoading] = useState(false);
    const [userForm, setUserForm] = useState<{user?: PluginUser} | null>(null);
    const [userData, setUserData] = useState<Record<string, unknown>>({});
    const [confirmDelete, setConfirmDelete] = useState(false);
    const [deleteUser, setDeleteUser] = useState<PluginUser | null>(null);
    const [busy, setBusy] = useState(false);

    const loadUsers = useCallback(async () => {
        setUsersLoading(true);
        try {
            setUsers(await listVPNServiceUsers(service.id));
        } catch (error) {
            toast.error(message(error, t('topology.usersLoadError')));
        } finally {
            setUsersLoading(false);
        }
    }, [service.id, t]);

    const openUsers = () => {
        setTab('users');
        void loadUsers();
    };

    const save = async (value: Record<string, unknown>) => {
        setBusy(true);
        try {
            await updateVPNService(service.id, {serverId: service.serverId, interfaceId, pluginId: service.pluginId, name, config: value});
            await refresh();
            toast.success(t('topology.saved'));
        } catch (error) {
            toast.error(message(error, t('topology.saveError')));
        } finally {
            setBusy(false);
        }
    };
    const apply = async () => {
        setBusy(true);
        try {
            await applyVPNService(service.id);
            await refresh();
            toast.success(t('topology.applied'));
        } catch (error) {
            await refresh();
            toast.error(message(error, t('topology.applyError')));
        } finally {
            setBusy(false);
        }
    };
    const checkStatus = async () => {
        setBusy(true);
        try {
            await refreshVPNServiceStatus(service.id);
            await refresh();
        } catch (error) {
            toast.error(message(error, t('topology.statusError')));
        } finally {
            setBusy(false);
        }
    };
    const remove = async () => {
        setBusy(true);
        try {
            await deleteVPNService(service.id);
            setConfirmDelete(false);
            onClose();
            await refresh();
            toast.success(t('topology.deleted'));
        } catch (error) {
            toast.error(message(error, t('topology.deleteError')));
        } finally {
            setBusy(false);
        }
    };
    const openUserForm = (user?: PluginUser) => {
        setUserData(user?.data ?? {});
        setUserForm({user});
    };
    const submitUser = async (value: Record<string, unknown>) => {
        if (!userForm) return;
        setBusy(true);
        try {
            if (userForm.user) {
                await updateVPNServiceUser(service.id, userForm.user.id, value);
            } else {
                await createVPNServiceUser(service.id, value);
            }
            setUserForm(null);
            await loadUsers();
            toast.success(t('topology.userSaved'));
        } catch (error) {
            toast.error(message(error, t('topology.saveError')));
        } finally {
            setBusy(false);
        }
    };
    const removeUser = async () => {
        if (!deleteUser) return;
        setBusy(true);
        try {
            await deleteVPNServiceUser(service.id, deleteUser.id);
            setDeleteUser(null);
            await loadUsers();
            toast.success(t('topology.deleted'));
        } catch (error) {
            toast.error(message(error, t('topology.deleteError')));
        } finally {
            setBusy(false);
        }
    };

    return (
        <InspectorShell title={service.name} subtitle={plugin?.name ?? service.pluginId} onClose={onClose}>
            <div className="mb-4 flex items-center justify-between gap-2 rounded-lg border border-border p-3">
                <div>
                    <div className="text-xs text-muted-foreground">{t('topology.serviceStatus')}</div>
                    <div className="mt-1 text-sm font-medium">{t(`topology.status.${service.status}`)}</div>
                </div>
                <button type="button" title={t('topology.checkStatus')} onClick={checkStatus} disabled={busy} className="rounded-md p-2 text-muted-foreground hover:bg-muted"><RefreshCw size={16} /></button>
            </div>
            {service.statusMessage && <p className="mb-4 break-words text-xs text-muted-foreground">{service.statusMessage}</p>}
            <div className="mb-4 grid grid-cols-2 rounded-lg bg-muted p-1">
                <button type="button" onClick={() => setTab('config')} className={`rounded-md px-3 py-2 text-sm ${tab === 'config' ? 'bg-card shadow-sm' : 'text-muted-foreground'}`}>{t('topology.configuration')}</button>
                <button type="button" onClick={openUsers} className={`rounded-md px-3 py-2 text-sm ${tab === 'users' ? 'bg-card shadow-sm' : 'text-muted-foreground'}`}>{t('nav.users')}</button>
            </div>
            {tab === 'config' && (
                <div className="space-y-4">
                    <FormField label={t('topology.serviceName')}><input value={name} onChange={event => setName(event.target.value)} required className={inputs.primary} /></FormField>
                    <FormField label={t('topology.attachedInterface')}>
                        <select value={interfaceId} onChange={event => setInterfaceId(event.target.value)} className={inputs.primary}>
                            <option value="">{t('topology.noInterface')}</option>
                            {topology.interfaces.filter(iface => iface.serverId === service.serverId && (!iface.serviceId || iface.serviceId === service.id)).map(iface => <option key={iface.id} value={iface.id}>{iface.name}</option>)}
                        </select>
                    </FormField>
                    {plugin ? (
                        <SchemaForm key={service.id} schema={plugin.configSchema} uiSchema={plugin.uiSchema} formData={config} onChange={setConfig} onSubmit={save} submitLabel={t('common.save')} disabled={busy || !name.trim()} />
                    ) : <p className="text-sm text-red-500">{t('topology.pluginUnavailable')}</p>}
                    <button type="button" onClick={apply} disabled={busy || !plugin?.available} className={`${buttons.primary} flex w-full items-center justify-center gap-2`}><Play size={14} />{t('topology.apply')}</button>
                </div>
            )}
            {tab === 'users' && !userForm && (
                <div>
                    <button type="button" onClick={() => openUserForm()} disabled={!plugin} className={`${buttons.secondary} mb-4 w-full`}><Plus size={14} />{t('topology.addUser')}</button>
                    {usersLoading ? <p className="py-6 text-center text-sm text-muted-foreground">{t('common.loading')}</p> : (
                        <div className="space-y-2">
                            {users.map(user => (
                                <div key={user.id} className="rounded-lg border border-border p-3">
                                    <div className="flex items-center justify-between gap-2">
                                        <div className="min-w-0 truncate text-sm font-medium" title={userLabel(user)}>{userLabel(user)}</div>
                                        <div className="flex shrink-0 gap-1">
                                            <button type="button" title={t('common.edit')} onClick={() => openUserForm(user)} className="rounded p-1 text-muted-foreground hover:bg-muted"><Pencil size={14} /></button>
                                            <button type="button" title={t('common.delete')} onClick={() => setDeleteUser(user)} className="rounded p-1 text-muted-foreground hover:bg-red-500/10 hover:text-red-500"><Trash2 size={14} /></button>
                                        </div>
                                    </div>
                                    <pre className="mt-2 max-h-24 overflow-auto whitespace-pre-wrap break-all text-[10px] text-muted-foreground">{JSON.stringify(user.data, null, 2)}</pre>
                                </div>
                            ))}
                            {users.length === 0 && <p className="py-6 text-center text-sm text-muted-foreground">{t('topology.noUsers')}</p>}
                        </div>
                    )}
                </div>
            )}
            {tab === 'users' && userForm && plugin && (
                <div>
                    <button type="button" onClick={() => setUserForm(null)} className="mb-3 text-sm text-muted-foreground hover:text-foreground">← {t('topology.backToUsers')}</button>
                    <h3 className="mb-3 text-sm font-semibold">{userForm.user ? t('topology.editUser') : t('topology.addUser')}</h3>
                    <SchemaForm key={userForm.user?.id ?? 'new'} schema={plugin.userSchema} uiSchema={plugin.uiSchema} formData={userData} onChange={setUserData} onSubmit={submitUser} submitLabel={t('common.save')} disabled={busy} />
                </div>
            )}
            <div className="mt-6 border-t border-border pt-4">
                <button type="button" onClick={() => setConfirmDelete(true)} className={`${buttons.danger} w-full`}><Trash2 size={14} />{t('topology.deleteService')}</button>
            </div>
            {confirmDelete && <ConfirmModal title={t('topology.deleteService')} message={t('topology.confirmDeleteService')} onCancel={() => setConfirmDelete(false)} onConfirm={remove} loading={busy} />}
            {deleteUser && <ConfirmModal title={t('topology.deleteUser')} message={t('topology.confirmDeleteUser')} onCancel={() => setDeleteUser(null)} onConfirm={removeUser} loading={busy} />}
        </InspectorShell>
    );
}

export function TopologyInspector({selection, topology, plugins, onClose, onSelectService, refresh}: InspectorProps) {
    if (selection.kind === 'server') {
        const server = topology.servers.find(item => item.id === selection.id);
        if (server) return <ServerInspector key={`${server.id}:${server.latitude ?? ''}:${server.longitude ?? ''}`} server={server} topology={topology} onClose={onClose} onSelectService={onSelectService} refresh={refresh} />;
    }
    if (selection.kind === 'interface') {
        const iface = topology.interfaces.find(item => item.id === selection.id);
        if (iface) return <InterfaceInspector key={iface.id} iface={iface} topology={topology} onClose={onClose} onSelectService={onSelectService} refresh={refresh} />;
    }
    if (selection.kind === 'service') {
        const service = topology.services.find(item => item.id === selection.id);
        if (service) return <ServiceInspector key={service.id} service={service} topology={topology} plugins={plugins} onClose={onClose} refresh={refresh} />;
    }
    return null;
}
