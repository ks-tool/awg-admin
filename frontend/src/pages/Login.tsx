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

import {useState} from 'react';
import type {FormEvent} from 'react';
import {useTranslation} from 'react-i18next';
import {ShieldCheck} from 'lucide-react';
import {buttons, inputs} from '@/components/common/Modal';
import {FormField} from '@/components/common/FormField';
import {cn} from '@/lib/utils';
import {login} from '@/services/auth';

interface Props {
    onSuccess: (username: string) => void;
}

export default function Login({onSuccess}: Props) {
    const {t} = useTranslation();
    const [username, setUsername] = useState('admin');
    const [password, setPassword] = useState('');
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string>();

    const handleSubmit = async (e: FormEvent) => {
        e.preventDefault();
        setLoading(true);
        setError(undefined);
        try {
            const res = await login(username, password);
            if (res.ok) {
                onSuccess(username);
            } else {
                setError(t('auth.loginError'));
            }
        } finally {
            setLoading(false);
        }
    };

    return (
        <div className="min-h-screen flex items-center justify-center bg-background px-4">
            <form
                onSubmit={handleSubmit}
                className="w-full max-w-sm space-y-5 rounded-xl border border-border bg-card p-8 shadow-2xl dark:border-white/5 dark:bg-zinc-900"
            >
                <div className="flex flex-col items-center gap-2 mb-2">
                    <ShieldCheck size={32} className="text-sky-500"/>
                    <h1 className="text-lg font-semibold text-foreground dark:text-zinc-100">
                        {t('auth.signInTitle')}
                    </h1>
                </div>

                <FormField label={t('auth.username')}>
                    <input
                        type="text"
                        value={username}
                        onChange={e => setUsername(e.target.value)}
                        disabled={loading}
                        autoFocus
                        required
                        className={inputs.primary}
                    />
                </FormField>

                <FormField label={t('auth.password')}>
                    <input
                        type="password"
                        value={password}
                        onChange={e => setPassword(e.target.value)}
                        disabled={loading}
                        required
                        className={inputs.primary}
                    />
                </FormField>

                {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}

                <button type="submit" disabled={loading} className={cn('w-full', buttons.primary)}>
                    {loading ? t('auth.loggingIn') : t('auth.login')}
                </button>
            </form>
        </div>
    );
}
