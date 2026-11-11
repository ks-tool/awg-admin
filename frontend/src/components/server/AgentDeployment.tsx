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
import {RefreshCw, Rocket} from 'lucide-react';
import {toast} from 'sonner';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs} from '@/components/common/controls';
import {SSHPassphraseModal} from '@/components/server/SSHPassphraseModal';
import {SudoPasswordModal} from '@/components/server/SudoPasswordModal';
import {deployAgentPack, deployProvider, getDeployStatus, unlockServerSudo} from '@/services/deployment';
import {unlockServerSSH} from '@/services/servers';
import {SSHPassphraseRequiredError, SudoPasswordRequiredError} from '@/services/sshErrors';
import {cn} from '@/lib/utils';
import type {Server} from '@/types';

const DEFAULT_AGENT_ARTIFACT = 'oci://ghcr.io/ks-tool/vpn-agent:v2.0.0-alpha.1';
const DEFAULT_AWG_ARTIFACT = 'oci://ghcr.io/ks-tool/vpn-providers/awg:v1.0.0-alpha.1';

function message(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
}

export function AgentDeployment({server, onChanged, onBusyChange}: {
    server: Server;
    onChanged: () => void;
    onBusyChange: (busy: boolean) => void;
}) {
    const {t} = useTranslation();
    const [agentArtifact, setAgentArtifact] = useState(DEFAULT_AGENT_ARTIFACT);
    const [awgArtifact, setAwgArtifact] = useState(DEFAULT_AWG_ARTIFACT);
    const [userspace, setUserspace] = useState(false);
    const [deploymentKind, setDeploymentKind] = useState<'agent' | 'provider'>('agent');
    const [starting, setStarting] = useState(false);
    const [deploying, setDeploying] = useState(false);
    const [step, setStep] = useState('');
    const [deployError, setDeployError] = useState('');
    const [unlock, setUnlock] = useState<'ssh' | 'sudo' | null>(null);
    const [unlocking, setUnlocking] = useState(false);
    const [unlockError, setUnlockError] = useState('');
    const busy = starting || deploying;

    useEffect(() => {
        onBusyChange(busy);
        return () => onBusyChange(false);
    }, [busy, onBusyChange]);

    const handleError = useCallback((error: unknown) => {
        setStarting(false);
        setDeploying(false);
        if (error instanceof SSHPassphraseRequiredError) {
            setUnlock('ssh');
            return;
        }
        if (error instanceof SudoPasswordRequiredError) {
            setUnlock('sudo');
            return;
        }
        const text = message(error);
        setDeployError(text);
        toast.error(text || t('servers.deployError'));
    }, [t]);

    const start = useCallback(async (kind: 'agent' | 'provider') => {
        const artifact = kind === 'agent' ? agentArtifact.trim() : awgArtifact.trim();
        if (!artifact) {
            toast.error(t('servers.artifactRequired'));
            return;
        }
        setDeploymentKind(kind);
        setStarting(true);
        setDeployError('');
        setStep('connect');
        try {
            if (kind === 'agent') {
                await deployAgentPack(server.id, artifact);
            } else {
                await deployProvider(server.id, {name: 'awg', artifact, options: {userspace}});
            }
            setDeploying(true);
        } catch (error) {
            handleError(error);
        } finally {
            setStarting(false);
        }
    }, [agentArtifact, awgArtifact, handleError, server.id, t, userspace]);

    useEffect(() => {
        if (!deploying) return;
        let cancelled = false;
        const poll = async () => {
            try {
                const status = await getDeployStatus(server.id);
                if (cancelled) return;
                setStep(status.step);
                if (!status.done) return;
                setDeploying(false);
                if (status.error) {
                    setDeployError(status.error);
                    toast.error(status.error, {duration: 10000});
                    return;
                }
                toast.success(t('servers.deployedSuccess'));
                onChanged();
            } catch (error) {
                if (!cancelled) handleError(error);
            }
        };
        void poll();
        const interval = window.setInterval(() => void poll(), 1000);
        return () => {
            cancelled = true;
            window.clearInterval(interval);
        };
    }, [deploying, handleError, onChanged, server.id, t]);

    const submitUnlock = async (secret: string, applyToAll = false) => {
        setUnlocking(true);
        setUnlockError('');
        try {
            if (unlock === 'ssh') {
                if (!await unlockServerSSH(server.id, secret, applyToAll)) {
                    setUnlockError(t('auth.unlockError'));
                    return;
                }
            } else {
                await unlockServerSudo(server.id, secret);
            }
            setUnlock(null);
            await start(deploymentKind);
        } catch (error) {
            setUnlockError(message(error));
        } finally {
            setUnlocking(false);
        }
    };

    return (
        <div className="rounded-xl border border-border bg-card p-6 dark:border-white/5 dark:bg-white/3">
            <div className="mb-1 flex items-center gap-2 text-lg font-semibold"><Rocket size={20} />{t('servers.deployAgentTitle')}</div>
            <p className="mb-5 text-sm text-muted-foreground">{t('servers.agentModal.packDeployHint')}</p>
            <div className="space-y-4">
                <FormField label={t('servers.agentArtifact')}>
                    <input required value={agentArtifact} onChange={event => setAgentArtifact(event.target.value)} disabled={busy} className={cn(inputs.primary, 'font-mono text-xs')} />
                </FormField>
                <div className="flex justify-end">
                    <button type="button" onClick={() => void start('agent')} disabled={busy || !agentArtifact.trim()} className={cn(buttons.primary, 'inline-flex items-center gap-2')}>
                        <Rocket size={14} />{busy && deploymentKind === 'agent' ? t('servers.deploying') : t('servers.deployAgent')}
                    </button>
                </div>
            </div>
            <div className="mt-5 space-y-4 rounded-lg border border-border p-4 dark:border-white/10">
                <FormField label={t('servers.providerArtifact')}>
                    <input required value={awgArtifact} onChange={event => setAwgArtifact(event.target.value)} disabled={busy} className={cn(inputs.primary, 'font-mono text-xs')} />
                </FormField>
                <label className="flex items-center gap-3 text-sm">
                    <input type="checkbox" checked={userspace} onChange={event => setUserspace(event.target.checked)} disabled={busy} />
                    {t('servers.forceUserspace')}
                </label>
                <div className="flex justify-end">
                    <button type="button" onClick={() => void start('provider')} disabled={busy || !awgArtifact.trim()} className={cn(buttons.primary, 'inline-flex items-center gap-2')}>
                        <Rocket size={14} />{busy && deploymentKind === 'provider' ? t('servers.deploying') : t('servers.deployProvider')}
                    </button>
                </div>
            </div>
            {busy && step && <div className="mt-3 flex items-center gap-2 text-xs text-muted-foreground"><RefreshCw size={12} className="animate-spin" />{t(`servers.deploySteps.${step}`, step)}</div>}
            {deployError && !busy && <p className="mt-3 whitespace-pre-wrap break-words rounded-lg bg-red-500/10 p-3 text-xs text-red-600 dark:text-red-400">{deployError}</p>}
            {unlock === 'ssh' && <SSHPassphraseModal onSubmit={(secret, applyToAll) => submitUnlock(secret, applyToAll)} onClose={() => setUnlock(null)} loading={unlocking} error={unlockError} />}
            {unlock === 'sudo' && <SudoPasswordModal onSubmit={secret => submitUnlock(secret)} onClose={() => setUnlock(null)} loading={unlocking} error={unlockError} />}
        </div>
    );
}
