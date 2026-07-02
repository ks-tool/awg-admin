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

import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { toast } from 'sonner';
import { Waypoints, Plus, Trash2, ArrowRight } from 'lucide-react';
import { PageHeader } from '@/components/layout/PageHeader';
import { Modal, buttons, inputs } from '@/components/common/Modal';
import { FormField } from '@/components/common/FormField';
import { ConfirmModal } from '@/components/common/ConfirmModal';
import { useAppStore } from '@/store';
import { listTunnels, buildTunnel, removeTunnel } from '@/services/tunnels';
import type { Interface, Server, Tunnel } from '@/types';

// An interface can anchor a tunnel entry only if it has a listen port and a
// CIDR address, and isn't already part of a tunnel.
const eligibleEntry = (iface: Interface) => !iface.tunnel && !!iface.listen && !!iface.addr && iface.addr.includes('/');
const eligibleExit = (iface: Interface) => !iface.tunnel;

// networkOf returns the CIDR network of an interface address, e.g.
// "172.23.24.2/24" -> "172.23.24.0/24" (IPv4). The tunnel always uses the entry
// interface's subnet, so this is shown read-only on the review step. Falls back
// to the raw input if it isn't a parseable IPv4 CIDR.
function networkOf(cidr: string): string {
    const [ip, bitsStr] = cidr.split('/');
    const bits = parseInt(bitsStr, 10);
    const octets = ip?.split('.').map(Number) ?? [];
    if (octets.length !== 4 || octets.some((o) => !Number.isInteger(o) || o < 0 || o > 255) || !(bits >= 0 && bits <= 32)) {
        return cidr;
    }
    const addr = ((octets[0] << 24) | (octets[1] << 16) | (octets[2] << 8) | octets[3]) >>> 0;
    const mask = bits === 0 ? 0 : (0xffffffff << (32 - bits)) >>> 0;
    const net = (addr & mask) >>> 0;
    return `${(net >>> 24) & 255}.${(net >>> 16) & 255}.${(net >>> 8) & 255}.${net & 255}/${bits}`;
}

function TunnelWizard({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
    const { t } = useTranslation();
    const { servers, interfaces } = useAppStore();
    const [step, setStep] = useState(1);
    const [entryServerId, setEntryServerId] = useState('');
    const [entryIfaceId, setEntryIfaceId] = useState('');
    const [exitServerId, setExitServerId] = useState('');
    const [exitIfaceId, setExitIfaceId] = useState('');
    const [building, setBuilding] = useState(false);

    const ifacesOf = (serverId: string): Interface[] => {
        const server = servers.find((s) => s.id === serverId);
        return (server?.interfaces ?? [])
            .map((id) => interfaces.get(id))
            .filter((i): i is Interface => !!i);
    };

    const entryIface = interfaces.get(entryIfaceId);
    const exitIface = interfaces.get(exitIfaceId);

    const handleBuild = async () => {
        setBuilding(true);
        try {
            const { tunnel, error } = await buildTunnel(
                [
                    { serverId: entryServerId, ifaceId: entryIfaceId },
                    { serverId: exitServerId, ifaceId: exitIfaceId },
                ],
                // The tunnel always uses the entry interface's subnet — the
                // backend derives it; nothing arbitrary is sent.
                '',
            );
            if (error || !tunnel) {
                toast.error(error || t('tunnels.buildError'));
                return;
            }
            toast.success(t('tunnels.built'));
            onCreated();
        } finally {
            setBuilding(false);
        }
    };

    const serverSelect = (value: string, onChange: (v: string) => void, exclude?: string) => (
        <select value={value} onChange={(e) => onChange(e.target.value)} className={inputs.primary}>
            <option value="">{t('tunnels.selectServer')}</option>
            {servers.filter((s) => s.id !== exclude).map((s: Server) => (
                <option key={s.id} value={s.id}>{s.name}</option>
            ))}
        </select>
    );

    const ifaceSelect = (
        serverId: string,
        value: string,
        onChange: (v: string) => void,
        eligible: (i: Interface) => boolean,
    ) => {
        const options = ifacesOf(serverId);
        return (
            <select value={value} onChange={(e) => onChange(e.target.value)} disabled={!serverId} className={inputs.primary}>
                <option value="">{t('tunnels.selectInterface')}</option>
                {options.map((i) => (
                    <option key={i.id} value={i.id} disabled={!eligible(i)}>
                        {i.iface}{i.tunnel ? ` (${t('tunnels.inTunnel')})` : ''}
                    </option>
                ))}
            </select>
        );
    };

    return (
        <Modal title={t('tunnels.wizardTitle')} onClose={onClose} loading={building}>
            <div className="space-y-4">
                {step === 1 && (
                    <>
                        <h4 className="text-sm font-semibold text-foreground">{t('tunnels.stepEntry')}</h4>
                        <p className="text-xs text-muted-foreground">{t('tunnels.entryHint')}</p>
                        <FormField label={t('tunnels.server')}>
                            {serverSelect(entryServerId, (v) => { setEntryServerId(v); setEntryIfaceId(''); })}
                        </FormField>
                        <FormField label={t('tunnels.interface')}>
                            {ifaceSelect(entryServerId, entryIfaceId, setEntryIfaceId, eligibleEntry)}
                        </FormField>
                    </>
                )}

                {step === 2 && (
                    <>
                        <h4 className="text-sm font-semibold text-foreground">{t('tunnels.stepExit')}</h4>
                        <p className="text-xs text-amber-600 dark:text-amber-400">{t('tunnels.exitWarning')}</p>
                        <FormField label={t('tunnels.server')}>
                            {serverSelect(exitServerId, (v) => { setExitServerId(v); setExitIfaceId(''); }, entryServerId)}
                        </FormField>
                        <FormField label={t('tunnels.interface')}>
                            {ifaceSelect(exitServerId, exitIfaceId, setExitIfaceId, eligibleExit)}
                        </FormField>
                    </>
                )}

                {step === 3 && (
                    <>
                        <h4 className="text-sm font-semibold text-foreground">{t('tunnels.stepReview')}</h4>
                        <FormField label={t('tunnels.subnet')}>
                            <div>
                                <div className="font-mono text-xs text-foreground dark:text-zinc-200">
                                    {entryIface?.addr ? networkOf(entryIface.addr) : '—'}
                                </div>
                                <p className="text-xs text-muted-foreground mt-2">{t('tunnels.subnetHint')}</p>
                            </div>
                        </FormField>
                        <div className="rounded-lg border border-border bg-muted/40 p-3 text-sm dark:border-white/10 dark:bg-white/5">
                            <div className="flex items-center gap-2 font-mono text-xs">
                                <span>{servers.find((s) => s.id === entryServerId)?.name}/{entryIface?.iface}</span>
                                <ArrowRight size={14} className="text-muted-foreground" />
                                <span>{servers.find((s) => s.id === exitServerId)?.name}/{exitIface?.iface}</span>
                            </div>
                        </div>
                    </>
                )}

                <div className="flex justify-between gap-3 pt-2">
                    <button
                        type="button"
                        onClick={() => (step === 1 ? onClose() : setStep(step - 1))}
                        disabled={building}
                        className={buttons.secondary}
                    >
                        {step === 1 ? t('common.cancel') : t('tunnels.back')}
                    </button>
                    {step < 3 ? (
                        <button
                            type="button"
                            onClick={() => setStep(step + 1)}
                            disabled={(step === 1 && !entryIfaceId) || (step === 2 && !exitIfaceId)}
                            className={buttons.primary}
                        >
                            {t('tunnels.next')}
                        </button>
                    ) : (
                        <button type="button" onClick={handleBuild} disabled={building} className={buttons.primary}>
                            {building ? t('tunnels.building') : t('tunnels.build')}
                        </button>
                    )}
                </div>
            </div>
        </Modal>
    );
}

export default function Tunnels() {
    const { t } = useTranslation();
    const { refreshData } = useAppStore();
    const [tunnels, setTunnels] = useState<Tunnel[]>([]);
    const [wizardOpen, setWizardOpen] = useState(false);
    const [removeId, setRemoveId] = useState<string | null>(null);
    const [removing, setRemoving] = useState(false);

    const load = async () => {
        const list = await listTunnels();
        if (list) setTunnels(list);
    };

    useEffect(() => {
        void load();
    }, []);

    const memberOf = (tun: Tunnel, role: string) => tun.members?.find((m) => m.role === role);

    const confirmRemove = async () => {
        if (!removeId) return;
        setRemoving(true);
        try {
            const { ok, error } = await removeTunnel(removeId);
            if (!ok) {
                toast.error(error || t('tunnels.removeError'));
                return;
            }
            toast.success(t('tunnels.removed'));
            await Promise.all([load(), refreshData()]);
        } finally {
            setRemoving(false);
            setRemoveId(null);
        }
    };

    return (
        <div className="flex flex-col">
            <PageHeader
                title={t('nav.tunnels')}
                icon={Waypoints}
                actions={
                    <button onClick={() => setWizardOpen(true)} className={buttons.primary}>
                        <Plus size={14} />
                    </button>
                }
            />

            <div className="px-8 py-6">
                <div className="rounded-xl border border-border bg-card dark:border-white/5 dark:bg-white/3 overflow-hidden">
                    <table className="w-full text-sm">
                        <thead>
                            <tr className="border-b border-white/5 text-xs uppercase tracking-wider text-zinc-600">
                                <th className="px-5 py-3 text-left font-medium">{t('tunnels.entry')}</th>
                                <th className="px-5 py-3 text-left font-medium">{t('tunnels.exit')}</th>
                                <th className="px-5 py-3 text-right font-medium"></th>
                            </tr>
                        </thead>
                        <tbody>
                            {tunnels.map((tun) => {
                                const entry = memberOf(tun, 'entry');
                                const exit = memberOf(tun, 'exit');
                                return (
                                    <tr key={tun.id} className="border-b border-white/3 last:border-0">
                                        <td className="px-5 py-3 font-mono text-xs dark:text-zinc-300">
                                            {entry ? `${entry.serverName}/${entry.interface}` : '—'}
                                        </td>
                                        <td className="px-5 py-3 font-mono text-xs dark:text-zinc-300">
                                            {exit ? `${exit.serverName}/${exit.interface}` : '—'}
                                        </td>
                                        <td className="px-5 py-3 text-right">
                                            <button
                                                onClick={() => setRemoveId(tun.id)}
                                                title={t('tunnels.remove')}
                                                className="text-muted-foreground hover:text-red-600 dark:hover:text-red-400"
                                            >
                                                <Trash2 size={16} />
                                            </button>
                                        </td>
                                    </tr>
                                );
                            })}
                        </tbody>
                    </table>
                    {tunnels.length === 0 && (
                        <div className="px-5 py-10 text-center text-sm text-zinc-600">{t('tunnels.noTunnels')}</div>
                    )}
                </div>
            </div>

            {wizardOpen && (
                <TunnelWizard
                    onClose={() => setWizardOpen(false)}
                    onCreated={async () => {
                        setWizardOpen(false);
                        await Promise.all([load(), refreshData()]);
                    }}
                />
            )}

            {removeId && (
                <ConfirmModal
                    title={t('tunnels.remove')}
                    message={t('tunnels.confirmRemove')}
                    confirmLabel={t('tunnels.remove')}
                    onConfirm={confirmRemove}
                    onCancel={() => setRemoveId(null)}
                    loading={removing}
                />
            )}
        </div>
    );
}
