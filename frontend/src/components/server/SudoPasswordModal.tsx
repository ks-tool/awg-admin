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
import {useTranslation} from 'react-i18next';
import {Modal, buttons, inputs} from '@/components/common/Modal';
import {FormField} from '@/components/common/FormField';
import {cn} from '@/lib/utils';

interface Props {
    onSubmit: (password: string) => void | Promise<void>;
    onClose: () => void;
    loading?: boolean;
    /** Сообщение об ошибке предыдущей попытки (например, неверный пароль). */
    error?: string;
}

/**
 * Запрашивает пароль sudo для развёртывания агента под пользователем SSH,
 * который не является root: privileged-команды деплоя выполняются через sudo,
 * а хост требует пароль (passwordless sudo пробуется первым и это окно не
 * показывает). Пароль кэшируется на стороне backend только для этого сервера на
 * время сессии.
 */
export function SudoPasswordModal({onSubmit, onClose, loading = false, error}: Props) {
    const {t} = useTranslation();
    const [password, setPassword] = useState('');

    const handleSubmit = async () => {
        if (!password) return;
        await onSubmit(password);
    };

    return (
        <Modal title={t('auth.sudoRequiredTitle')} onClose={onClose} loading={loading}>
            <div className="space-y-4">
                <p className="text-sm text-muted-foreground dark:text-zinc-400">
                    {t('auth.sudoRequiredDescription')}
                </p>

                <FormField label={t('auth.sudoPassword')}>
                    <input
                        type="password"
                        value={password}
                        onChange={e => setPassword(e.target.value)}
                        disabled={loading}
                        autoFocus
                        className={inputs.primary}
                    />
                </FormField>

                {error && (
                    <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
                )}

                <div className="flex gap-3 pt-4">
                    <button
                        type="button"
                        onClick={handleSubmit}
                        disabled={loading || !password}
                        className={cn('flex-1', buttons.primary)}
                    >
                        {loading ? t('auth.connecting') : t('auth.connect')}
                    </button>
                    <button type="button" onClick={onClose} disabled={loading} className={cn('flex-1', buttons.secondary)}>
                        {t('common.cancel')}
                    </button>
                </div>
            </div>
        </Modal>
    );
}
