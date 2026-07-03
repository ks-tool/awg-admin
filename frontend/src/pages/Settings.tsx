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

import {useEffect, useRef, useState} from 'react';
import type {FormEvent} from 'react';
import { useTranslation } from 'react-i18next';
import { Settings as SettingsIcon, ScrollText, Save, RefreshCw, DatabaseBackup } from 'lucide-react';
import { toast } from 'sonner';
import { PageHeader } from '@/components/layout/PageHeader';
import { buttons, inputs, Modal } from '@/components/common/Modal';
import { FormField } from '@/components/common/FormField';
import { CopyButton } from '@/components/common/CopyButton';
import { useAuth } from '@/contexts/AuthContext';
import { changeCredentials, getCurrentUser, setBasicAuthEnabled } from '@/services/auth';
import { getLogs, saveLogs, debugLoggingEnabled, setDebugLogging } from '@/services/logs';
import { saveBackup } from '@/services/backup';
import { getAppVersion } from '@/services/version';
import { getCurrentApiMode } from '@/services/apiMode';
import { cn } from '@/lib/utils';

const LANGUAGES = [
    { code: 'en', label: 'English' },
    { code: 'ru', label: 'Русский' },
];

function ChangeCredentialsSection() {
    const { t } = useTranslation();
    const { username } = useAuth();
    const [currentPassword, setCurrentPassword] = useState('');
    const [newUsername, setNewUsername] = useState('');
    const [newPassword, setNewPassword] = useState('');
    const [loading, setLoading] = useState(false);

    const handleSubmit = async (e: FormEvent) => {
        e.preventDefault();
        if (!currentPassword.trim()) return toast.error(t('auth.currentPasswordRequired'));

        setLoading(true);
        try {
            const res = await changeCredentials(currentPassword, newUsername, newPassword);
            if (res.ok) {
                toast.success(t('auth.credentialsUpdated'));
                setCurrentPassword('');
                setNewUsername('');
                setNewPassword('');
            } else {
                toast.error(t('auth.credentialsUpdateError'));
            }
        } finally {
            setLoading(false);
        }
    };

    return (
        <form onSubmit={handleSubmit} className="p-4 bg-card border border-border rounded-lg space-y-4">
            <h2 className="text-sm font-semibold text-foreground">{t('auth.changeCredentials')}</h2>
            <p className="text-xs text-muted-foreground">{username}</p>

            <FormField label={t('auth.currentPassword')}>
                <input
                    type="password"
                    value={currentPassword}
                    onChange={e => setCurrentPassword(e.target.value)}
                    disabled={loading}
                    required
                    className={inputs.primary}
                />
            </FormField>

            <FormField label={t('auth.newUsername')}>
                <input
                    type="text"
                    value={newUsername}
                    onChange={e => setNewUsername(e.target.value)}
                    placeholder={t('auth.newUsernamePlaceholder')}
                    disabled={loading}
                    className={inputs.primary}
                />
            </FormField>

            <FormField label={t('auth.newPassword')}>
                <input
                    type="password"
                    value={newPassword}
                    onChange={e => setNewPassword(e.target.value)}
                    placeholder={t('auth.newPasswordPlaceholder')}
                    disabled={loading}
                    className={inputs.primary}
                />
            </FormField>

            <button type="submit" disabled={loading} className={buttons.primary}>
                {loading ? `${t('common.saving')}...` : t('common.save')}
            </button>
        </form>
    );
}

function BasicAuthSection() {
    const { t } = useTranslation();
    const [enabled, setEnabled] = useState(false);
    const [loaded, setLoaded] = useState(false);
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        let cancelled = false;
        getCurrentUser().then(user => {
            if (cancelled) return;
            setEnabled(user?.basicAuthEnabled ?? false);
            setLoaded(true);
        });
        return () => {
            cancelled = true;
        };
    }, []);

    const handleToggle = async () => {
        const next = !enabled;
        setSaving(true);
        try {
            const res = await setBasicAuthEnabled(next);
            if (res.ok) {
                setEnabled(next);
                toast.success(next ? t('settings.basicAuthEnabledMsg') : t('settings.basicAuthDisabledMsg'));
            } else {
                toast.error(t('settings.basicAuthUpdateError'));
            }
        } finally {
            setSaving(false);
        }
    };

    if (!loaded) return null;

    return (
        <div className="p-4 bg-card border border-border rounded-lg space-y-2">
            <div className="flex items-center justify-between">
                <span className="text-sm font-medium text-muted-foreground">{t('settings.basicAuth')}</span>
                <label className="flex items-center cursor-pointer">
                    <input
                        type="checkbox"
                        checked={enabled}
                        onChange={handleToggle}
                        disabled={saving}
                        className="rounded border-input bg-background text-sky-500 focus:ring-sky-500 focus:ring-offset-0 disabled:opacity-50 dark:border-white/10 dark:bg-white/5"
                    />
                </label>
            </div>
            <p className="text-xs text-muted-foreground">{t('settings.basicAuthHint')}</p>
        </div>
    );
}

// Desktop-only: view and save the process's captured stdout logs. In http
// mode there is no log buffer to read, so LogsSection is never rendered.
function LogsModal({ onClose }: { onClose: () => void }) {
    const { t } = useTranslation();
    const [logs, setLogs] = useState('');
    const [loading, setLoading] = useState(true);
    const [saving, setSaving] = useState(false);
    const [debug, setDebug] = useState(false);
    const scrollRef = useRef<HTMLPreElement>(null);

    const load = async () => {
        setLoading(true);
        try {
            setLogs(await getLogs());
        } finally {
            setLoading(false);
        }
    };

    useEffect(() => {
        load();
        void debugLoggingEnabled().then(setDebug);
    }, []);

    const handleToggleDebug = async (enabled: boolean) => {
        setDebug(enabled); // optimistic
        if (!(await setDebugLogging(enabled))) {
            setDebug(!enabled); // revert on failure (reportError already toasted)
        }
    };

    // Jump to the newest lines whenever the content changes (they matter most).
    useEffect(() => {
        const el = scrollRef.current;
        if (el) el.scrollTop = el.scrollHeight;
    }, [logs]);

    const handleSave = async () => {
        setSaving(true);
        try {
            if (await saveLogs()) toast.success(t('settings.logsSaved'));
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal title={t('settings.logsTitle')} onClose={onClose} size="lg">
            <div className="flex flex-col gap-4">
                <div className="flex items-center justify-between gap-2">
                    <button
                        onClick={load}
                        disabled={loading}
                        className={cn('inline-flex items-center gap-2', buttons.secondary)}
                    >
                        <RefreshCw size={14} className={loading ? 'animate-spin' : undefined} />
                        {t('common.refresh')}
                    </button>
                    <div className="flex items-center gap-3">
                        <label className="inline-flex cursor-pointer items-center gap-2 text-sm text-muted-foreground">
                            <input
                                type="checkbox"
                                checked={debug}
                                onChange={(e) => handleToggleDebug(e.target.checked)}
                                className="h-4 w-4 rounded border-input accent-primary"
                            />
                            {t('settings.debugLogging')}
                        </label>
                        {logs && <CopyButton value={logs} />}
                    </div>
                </div>
                <p className="text-xs text-muted-foreground">{t('settings.debugLoggingHint')}</p>

                {loading ? (
                    <div className="py-12 text-center text-sm text-muted-foreground">{t('common.loading')}</div>
                ) : logs ? (
                    <pre
                        ref={scrollRef}
                        className="max-h-[55vh] overflow-auto whitespace-pre-wrap break-all rounded-lg border border-border bg-muted/40 p-3 font-mono text-xs text-foreground dark:border-white/10 dark:bg-black/30 dark:text-zinc-200"
                    >
                        {logs}
                    </pre>
                ) : (
                    <div className="py-12 text-center text-sm text-muted-foreground">{t('settings.logsEmpty')}</div>
                )}

                <div className="flex items-center justify-end gap-2">
                    <button
                        onClick={handleSave}
                        disabled={saving || !logs}
                        className={cn('inline-flex items-center gap-2', buttons.primary)}
                    >
                        <Save size={16} />
                        {t('settings.saveLogs')}
                    </button>
                    <button onClick={onClose} className={buttons.secondary}>{t('common.close')}</button>
                </div>
            </div>
        </Modal>
    );
}

function LogsSection() {
    const { t } = useTranslation();
    const [open, setOpen] = useState(false);

    return (
        <div className="p-4 bg-card border border-border rounded-lg space-y-2">
            <div className="flex items-center justify-between">
                <span className="text-sm font-medium text-muted-foreground">{t('settings.logs')}</span>
                <button
                    onClick={() => setOpen(true)}
                    className={cn('inline-flex items-center gap-2', buttons.secondary)}
                >
                    <ScrollText size={16} />
                    {t('settings.viewLogs')}
                </button>
            </div>
            <p className="text-xs text-muted-foreground">{t('settings.logsHint')}</p>
            {open && <LogsModal onClose={() => setOpen(false)} />}
        </div>
    );
}

// Full backup of the admin database (servers, users, peers, credentials) as a
// JSON dump restorable with `awg-migrate import`. Available in both desktop
// (native save dialog) and standalone (browser download) modes.
function BackupSection() {
    const { t } = useTranslation();
    const [saving, setSaving] = useState(false);

    const handleBackup = async () => {
        setSaving(true);
        try {
            if (await saveBackup()) toast.success(t('settings.backupSaved'));
        } finally {
            setSaving(false);
        }
    };

    return (
        <div className="p-4 bg-card border border-border rounded-lg space-y-2">
            <div className="flex items-center justify-between">
                <span className="text-sm font-medium text-muted-foreground">{t('settings.backup')}</span>
                <button
                    onClick={handleBackup}
                    disabled={saving}
                    className={cn('inline-flex items-center gap-2', buttons.secondary)}
                >
                    {saving ? <RefreshCw size={16} className="animate-spin" /> : <DatabaseBackup size={16} />}
                    {t('settings.createBackup')}
                </button>
            </div>
            <p className="text-xs text-muted-foreground">{t('settings.backupHint')}</p>
        </div>
    );
}

export default function Settings() {
    const { t, i18n } = useTranslation();
    const { enabled: authEnabled } = useAuth();
    const isDesktop = getCurrentApiMode() === 'bindings';
    const [appVersion, setAppVersion] = useState<string | null>(null);

    useEffect(() => {
        let cancelled = false;
        getAppVersion().then((v) => {
            if (!cancelled) setAppVersion(v);
        });
        return () => {
            cancelled = true;
        };
    }, []);

    const handleLanguageChange = (lang: string) => {
        i18n.changeLanguage(lang);
    };

    return (
        <div className="flex flex-col">
            <PageHeader
                title={t('settings.title')}
                icon={SettingsIcon}
            />

            <div className="px-8 py-6 space-y-4">
                <div className="p-4 bg-card border border-border rounded-lg">
                    <div className="flex items-center justify-between">
                        <span className="text-sm font-medium text-muted-foreground">{t('settings.language')}</span>
                        <select
                            value={i18n.language}
                            onChange={(e) => handleLanguageChange(e.target.value)}
                            className="w-auto rounded-lg border border-input bg-input px-3 py-1.5 text-sm text-foreground focus:border-sky-500/50 focus:outline-none transition-colors dark:border-white/10 dark:bg-white/5 dark:text-zinc-100 dark:focus:bg-white/10"
                        >
                            {LANGUAGES.map(({code, label}) => (
                                <option key={code} value={code}>{label}</option>
                            ))}
                        </select>
                    </div>
                </div>

                <BackupSection />
                {isDesktop && <LogsSection />}
                {authEnabled && <BasicAuthSection />}
                {authEnabled && <ChangeCredentialsSection />}

                {appVersion && (
                    <div className="p-4 bg-card border border-border rounded-lg">
                        <div className="flex items-center justify-between">
                            <span className="text-sm font-medium text-muted-foreground">{t('settings.version')}</span>
                            <span className="font-mono text-sm text-foreground">{appVersion}</span>
                        </div>
                    </div>
                )}
            </div>
        </div>
    );
}
