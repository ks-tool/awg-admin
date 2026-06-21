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
    onSubmit: (passphrase: string, applyToAll: boolean) => void | Promise<void>;
    onClose: () => void;
    loading?: boolean;
    /** Сообщение об ошибке предыдущей попытки (например, неверный пароль). */
    error?: string;
}

/**
 * Запрашивает пароль (passphrase) для расшифровки защищённого SSH-ключа при
 * подключении к серверу. Чекбокс "использовать для всех подключений"
 * кэширует введённый пароль на стороне backend на время текущей сессии и
 * использует его как запасной вариант для любого другого ключа, которому
 * тоже потребуется пароль.
 */
export function SSHPassphraseModal({onSubmit, onClose, loading = false, error}: Props) {
    const {t} = useTranslation();
    const [passphrase, setPassphrase] = useState('');
    const [applyToAll, setApplyToAll] = useState(false);

    const handleSubmit = async () => {
        if (!passphrase) return;
        await onSubmit(passphrase, applyToAll);
    };

    return (
        <Modal title={t('auth.sshKeyProtectedTitle')} onClose={onClose} loading={loading}>
            <div className="space-y-4">
                <p className="text-sm text-muted-foreground dark:text-zinc-400">
                    {t('auth.sshKeyProtectedDescription')}
                </p>

                <FormField label={t('auth.keyPassphrase')}>
                    <input
                        type="password"
                        value={passphrase}
                        onChange={e => setPassphrase(e.target.value)}
                        disabled={loading}
                        autoFocus
                        className={inputs.primary}
                    />
                </FormField>

                {error && (
                    <p className="text-sm text-red-600 dark:text-red-400">{error}</p>
                )}

                <label className="flex items-center">
                    <input
                        type="checkbox"
                        checked={applyToAll}
                        onChange={e => setApplyToAll(e.target.checked)}
                        disabled={loading}
                        className="rounded border-white/10 bg-white/5 text-sky-500 focus:ring-sky-500 focus:ring-offset-0"
                    />
                    <span className="ml-3 text-sm font-medium text-foreground dark:text-zinc-300">
                        {t('auth.useForAllConnections')}
                    </span>
                </label>

                <div className="flex gap-3 pt-4">
                    <button
                        type="button"
                        onClick={handleSubmit}
                        disabled={loading || !passphrase}
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
