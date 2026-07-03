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
import {Activity, Download, GitCompareArrows, RefreshCw, Rocket} from 'lucide-react';
import {buttons, inputs, Modal} from '@/components/common/Modal';
import {
    deployAgent,
    getDeployStatus,
    getServerHostInfo,
    saveServerProfile,
    setServerMonitoring,
    setServerProfiling,
    syncServer,
    unlockServerSSH,
} from '@/services/servers';
import {SSHPassphraseRequiredError} from '@/services/sshErrors';
import {SSHPassphraseModal} from '@/components/server/SSHPassphraseModal';
import {AgentSourceCombobox} from '@/components/server/AgentSourceCombobox';
import {HostInfoBadges} from '@/components/server/HostInfoBadges';
import {ReconcileModal} from '@/components/server/ReconcileModal';
import {useAppStore} from '@/store';
import {cn} from '@/lib/utils';
import type {HostInfo, Server} from '@/types';

// pprof profile kinds the agent exposes. "profile" (CPU) and "trace" sample
// over a window (seconds); the rest are instantaneous. Names are the raw pprof
// terms — deliberately untranslated, like other technical identifiers.
const PROFILE_KINDS = ['goroutine', 'heap', 'allocs', 'profile', 'trace', 'block', 'mutex', 'threadcreate'] as const;
const TIMED_KINDS = new Set(['profile', 'trace']);

function Row({title, hint, children}: {title: string; hint: string; children: React.ReactNode}) {
    return (
        <div className="flex items-start justify-between gap-4 py-3 border-b border-border last:border-0 dark:border-white/5">
            <div className="min-w-0">
                <div className="text-sm font-medium text-foreground dark:text-zinc-200">{title}</div>
                <div className="mt-0.5 text-xs text-muted-foreground dark:text-zinc-500">{hint}</div>
            </div>
            <div className="shrink-0 flex items-center gap-2">{children}</div>
        </div>
    );
}

// AgentModal is the single place to view and operate a server's agent: its
// version/capabilities, deploy (install/update over SSH), metrics on/off, sync,
// reconcile, and Go runtime profiling (toggle + dump). Opened from the dashboard
// (per-row gear) and the server detail page. onChanged, if given, is called
// after any action that may alter the server's interfaces so the opener can
// refresh its own view.
export function AgentModal({server, onClose, onChanged}: {server: Server; onClose: () => void; onChanged?: () => void}) {
    const {t} = useTranslation();
    const refreshData = useAppStore((s) => s.refreshData);

    const [hostInfo, setHostInfo] = useState<HostInfo | null>(null);

    // Seeded once from the server record; the dashboard icon reads the store
    // (refreshed after each toggle), this drives the modal's own controls.
    const [metricsEnabled, setMetricsEnabled] = useState(!server.agent?.monitoringDisabled);
    const [profilingEnabled, setProfilingEnabled] = useState(!!server.agent?.profilingEnabled);

    const [metricsBusy, setMetricsBusy] = useState(false);
    const [syncBusy, setSyncBusy] = useState(false);
    const [profilingBusy, setProfilingBusy] = useState(false);
    const [dumpBusy, setDumpBusy] = useState(false);

    const [profileKind, setProfileKind] = useState<string>('goroutine');
    const [profileSeconds, setProfileSeconds] = useState(30);

    const [showReconcile, setShowReconcile] = useState(false);

    // Deploy state (mirrors the old ServerDetail flow): DeployAgent starts a
    // background deploy and returns immediately; pollDeployStatus watches step
    // progress + the outcome. A passphrase-protected SSH key surfaces as an
    // SSHPassphraseRequiredError, handled via the nested SSHPassphraseModal.
    const [agentSourceId, setAgentSourceId] = useState('');
    const [deployLoading, setDeployLoading] = useState(false);
    const [deployStep, setDeployStep] = useState('');
    const deployPollRef = useRef<ReturnType<typeof setInterval> | null>(null);
    const handleDeployAgentRef = useRef<(sourceId: string) => void>(() => {});
    const [sshUnlock, setSshUnlock] = useState<{retry: () => void} | null>(null);
    const [unlockLoading, setUnlockLoading] = useState(false);
    const [unlockError, setUnlockError] = useState<string | undefined>();

    const timed = TIMED_KINDS.has(profileKind);

    // Agent version/capabilities (GET /info via the agent). Best-effort — null
    // when the agent is unreachable or not yet deployed. Fetched on open and
    // again after a deploy; a monotonic seq makes the latest fetch win so a
    // slow/late one (e.g. a mount fetch that hung until its ~15s timeout) can't
    // clobber a fresher value written by the post-deploy fetch.
    const hostSeq = useRef(0);
    const refreshHostInfo = useCallback(() => {
        const seq = ++hostSeq.current;
        void getServerHostInfo(server.id).then((info) => {
            if (seq === hostSeq.current) setHostInfo(info);
        });
    }, [server.id]);
    useEffect(() => { refreshHostInfo(); }, [refreshHostInfo]);

    const stopDeployPolling = useCallback(() => {
        if (deployPollRef.current !== null) {
            clearInterval(deployPollRef.current);
            deployPollRef.current = null;
        }
    }, []);
    useEffect(() => stopDeployPolling, [stopDeployPolling]);

    const afterChange = useCallback(async () => {
        await refreshData();
        onChanged?.();
    }, [refreshData, onChanged]);

    const pollDeployStatus = useCallback(async (sourceId: string) => {
        try {
            const status = await getDeployStatus(server.id);
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
                refreshHostInfo();
                await afterChange();
            }
        } catch (err) {
            stopDeployPolling();
            setDeployLoading(false);
            if (err instanceof SSHPassphraseRequiredError) {
                setSshUnlock({retry: () => handleDeployAgentRef.current(sourceId)});
                return;
            }
            console.error('Failed to poll deploy status:', err);
            toast.error(t('servers.deployError'));
        }
    }, [server.id, stopDeployPolling, afterChange, refreshHostInfo, t]);

    const handleDeployAgent = useCallback(async (sourceId: string) => {
        stopDeployPolling();
        setDeployLoading(true);
        setDeployStep('connect');
        try {
            const ok = await deployAgent(server.id, sourceId);
            if (!ok) {
                setDeployLoading(false);
                toast.error(t('servers.deployError'));
                return;
            }
            void pollDeployStatus(sourceId);
            deployPollRef.current = setInterval(() => void pollDeployStatus(sourceId), 1000);
        } catch (err) {
            setDeployLoading(false);
            if (err instanceof SSHPassphraseRequiredError) {
                setSshUnlock({retry: () => handleDeployAgentRef.current(sourceId)});
                return;
            }
            console.error('Failed to deploy agent:', err);
            toast.error(t('servers.deployError'));
        }
    }, [server.id, stopDeployPolling, pollDeployStatus, t]);

    useEffect(() => {
        handleDeployAgentRef.current = (sourceId: string) => void handleDeployAgent(sourceId);
    }, [handleDeployAgent]);

    const onDeployClick = () => {
        if (!agentSourceId) return toast.error(t('servers.noSourceSelected'));
        void handleDeployAgent(agentSourceId);
    };

    const handleUnlockSubmit = async (passphrase: string, applyToAll: boolean) => {
        if (!sshUnlock) return;
        setUnlockLoading(true);
        setUnlockError(undefined);
        try {
            const ok = await unlockServerSSH(server.id, passphrase, applyToAll);
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

    const toggleMetrics = async () => {
        setMetricsBusy(true);
        try {
            const updated = await setServerMonitoring(server.id, !metricsEnabled);
            if (!updated) return toast.error(t('servers.monitoringError'));
            setMetricsEnabled(!metricsEnabled);
            toast.success(!metricsEnabled ? t('servers.monitoringEnabled') : t('servers.monitoringDisabled'));
            await afterChange();
        } finally {
            setMetricsBusy(false);
        }
    };

    const doSync = async () => {
        setSyncBusy(true);
        try {
            const ok = await syncServer(server.id);
            toast[ok ? 'success' : 'error'](ok ? t('servers.syncSuccess') : t('servers.syncError'));
            if (ok) await afterChange();
        } finally {
            setSyncBusy(false);
        }
    };

    const toggleProfiling = async () => {
        setProfilingBusy(true);
        try {
            const updated = await setServerProfiling(server.id, !profilingEnabled);
            if (!updated) return toast.error(t('servers.agentModal.profilingErrorMsg'));
            setProfilingEnabled(!profilingEnabled);
            toast.success(!profilingEnabled ? t('servers.agentModal.profilingOn') : t('servers.agentModal.profilingOff'));
            await afterChange();
        } finally {
            setProfilingBusy(false);
        }
    };

    const downloadDump = async () => {
        setDumpBusy(true);
        try {
            const ok = await saveServerProfile(server.id, profileKind, timed ? profileSeconds : 0);
            if (ok) toast.success(t('servers.agentModal.dumpDownloaded'));
        } catch (error) {
            console.error('Failed to download profile:', error);
            toast.error(error instanceof Error && error.message ? error.message : t('servers.agentModal.dumpError'));
        } finally {
            setDumpBusy(false);
        }
    };

    return (
        // Block closing while a deploy is in flight (loading), so the modal stays
        // mounted and the status poll survives to fire the completion toast +
        // refresh — closing would unmount it and tear the poll down. dimmed is
        // dropped while a nested modal (reconcile / passphrase) is up so the two
        // backdrops don't compound into a darker overlay.
        <Modal
            title={t('servers.agentModal.title', {name: server.name})}
            onClose={onClose}
            size="md"
            loading={deployLoading}
            dimmed={!(showReconcile || sshUnlock)}
        >
            <div className="space-y-1">
                {/* Agent version / capabilities */}
                <div className="flex items-center justify-between gap-4 pb-3 border-b border-border dark:border-white/5">
                    <div className="text-sm">
                        <span className="text-muted-foreground dark:text-zinc-500">{t('servers.hostInfo.version')}: </span>
                        <span className="font-mono text-foreground dark:text-zinc-200">{hostInfo?.version || '—'}</span>
                    </div>
                    <HostInfoBadges info={hostInfo}/>
                </div>

                {/* Deploy agent (install/update over SSH) */}
                <div className="py-3 border-b border-border dark:border-white/5">
                    <div className="text-sm font-medium text-foreground dark:text-zinc-200">{t('servers.deployAgentTitle')}</div>
                    <div className="mt-0.5 mb-2 text-xs text-muted-foreground dark:text-zinc-500">{t('servers.agentModal.deployHint')}</div>
                    <div className="flex items-end gap-2">
                        <div className="flex-1 min-w-0">
                            <AgentSourceCombobox value={agentSourceId} onChange={setAgentSourceId} disabled={deployLoading} dockerAvailable={hostInfo?.docker}/>
                        </div>
                        <button onClick={onDeployClick} disabled={deployLoading} className={cn(buttons.primary, 'inline-flex items-center gap-1.5')}>
                            <Rocket size={14}/>
                            {deployLoading ? t('servers.deploying') : t('servers.deploy')}
                        </button>
                    </div>
                    {deployLoading && deployStep && (
                        <div className="mt-2 flex items-center gap-2 text-xs text-muted-foreground dark:text-zinc-400">
                            <RefreshCw size={12} className="animate-spin"/>
                            <span>{t(`servers.deploySteps.${deployStep}`, deployStep)}</span>
                        </div>
                    )}
                </div>

                {/* Metrics */}
                <Row title={t('servers.agentModal.metrics')} hint={t('servers.agentModal.metricsHint')}>
                    <button
                        onClick={toggleMetrics}
                        disabled={metricsBusy}
                        className={cn(buttons.secondary, metricsEnabled && 'text-emerald-600 dark:text-emerald-400')}
                    >
                        {metricsEnabled ? t('servers.disableMonitoring') : t('servers.enableMonitoring')}
                    </button>
                </Row>

                {/* Sync */}
                <Row title={t('servers.agentModal.sync')} hint={t('servers.agentModal.syncHint')}>
                    <button onClick={doSync} disabled={syncBusy} className={cn(buttons.secondary, 'inline-flex items-center gap-1.5')}>
                        <RefreshCw size={14} className={cn(syncBusy && 'animate-spin')}/>
                        {t('servers.agentModal.sync')}
                    </button>
                </Row>

                {/* Check (reconcile) */}
                <Row title={t('servers.agentModal.check')} hint={t('servers.agentModal.checkHint')}>
                    <button onClick={() => setShowReconcile(true)} className={cn(buttons.secondary, 'inline-flex items-center gap-1.5')}>
                        <GitCompareArrows size={14}/>
                        {t('servers.agentModal.check')}
                    </button>
                </Row>

                {/* Profiling */}
                <Row title={t('servers.agentModal.profiling')} hint={t('servers.agentModal.profilingHint')}>
                    <button
                        onClick={toggleProfiling}
                        disabled={profilingBusy}
                        className={cn(
                            buttons.secondary,
                            'inline-flex items-center gap-1.5',
                            profilingEnabled && 'text-amber-600 dark:text-amber-400',
                        )}
                    >
                        <Activity size={14}/>
                        {profilingEnabled ? t('servers.agentModal.disableProfiling') : t('servers.agentModal.enableProfiling')}
                    </button>
                </Row>

                {/* Profile dump — only meaningful while profiling is enabled. */}
                <div className="pt-3">
                    {profilingEnabled ? (
                        <div className="flex flex-wrap items-end gap-2">
                            <label className="flex flex-col gap-1 text-xs text-muted-foreground dark:text-zinc-400">
                                {t('servers.agentModal.profileKind')}
                                <select
                                    value={profileKind}
                                    onChange={(e) => setProfileKind(e.target.value)}
                                    className={cn(inputs.primary, 'py-1.5')}
                                >
                                    {PROFILE_KINDS.map((k) => (
                                        <option key={k} value={k}>{k === 'profile' ? 'profile (CPU)' : k}</option>
                                    ))}
                                </select>
                            </label>
                            {timed && (
                                <label className="flex flex-col gap-1 text-xs text-muted-foreground dark:text-zinc-400">
                                    {t('servers.agentModal.seconds')}
                                    <input
                                        type="number"
                                        min={1}
                                        max={60}
                                        value={profileSeconds}
                                        onChange={(e) => setProfileSeconds(Math.max(1, Math.min(60, Number(e.target.value) || 1)))}
                                        className={cn(inputs.primary, 'w-20 py-1.5')}
                                    />
                                </label>
                            )}
                            <button
                                onClick={downloadDump}
                                disabled={dumpBusy}
                                className={cn(buttons.primary, 'inline-flex items-center gap-1.5')}
                            >
                                <Download size={14}/>
                                {t('servers.agentModal.downloadDump')}
                            </button>
                        </div>
                    ) : (
                        <p className="text-xs text-muted-foreground dark:text-zinc-500">{t('servers.agentModal.dumpNeedsProfiling')}</p>
                    )}
                </div>
            </div>

            {showReconcile && (
                <ReconcileModal
                    serverId={server.id}
                    onClose={() => setShowReconcile(false)}
                    onChanged={() => { void afterChange(); }}
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
        </Modal>
    );
}
