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

import {useEffect, useState} from 'react';
import type {FormEvent} from 'react';
import { useTranslation } from 'react-i18next';
import { Settings as SettingsIcon } from 'lucide-react';
import { toast } from 'sonner';
import { PageHeader } from '@/components/layout/PageHeader';
import { buttons, inputs } from '@/components/common/Modal';
import { FormField } from '@/components/common/FormField';
import { useAuth } from '@/contexts/AuthContext';
import { changeCredentials, getCurrentUser, setBasicAuthEnabled } from '@/services/auth';

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

export default function Settings() {
    const { t, i18n } = useTranslation();
    const { enabled: authEnabled } = useAuth();

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

                {authEnabled && <BasicAuthSection />}
                {authEnabled && <ChangeCredentialsSection />}
            </div>
        </div>
    );
}
