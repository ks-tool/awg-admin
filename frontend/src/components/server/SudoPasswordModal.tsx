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
import {FormField} from '@/components/common/FormField';
import {Modal} from '@/components/common/Modal';
import {buttons, inputs} from '@/components/common/controls';
import {cn} from '@/lib/utils';

interface Props {
    onSubmit: (password: string) => Promise<void>;
    onClose: () => void;
    loading: boolean;
    error?: string;
}

export function SudoPasswordModal({onSubmit, onClose, loading, error}: Props) {
    const {t} = useTranslation();
    const [password, setPassword] = useState('');

    return (
        <Modal title={t('auth.sudoRequiredTitle')} onClose={onClose} loading={loading} dimmed={false}>
            <form onSubmit={event => {event.preventDefault(); void onSubmit(password);}} className="space-y-4">
                <p className="text-sm text-muted-foreground">{t('auth.sudoRequiredDescription')}</p>
                <FormField label={t('auth.sudoPassword')}>
                    <input autoFocus required type="password" value={password} onChange={event => setPassword(event.target.value)} disabled={loading} className={inputs.primary} />
                </FormField>
                {error && <p className="text-sm text-red-600 dark:text-red-400">{error}</p>}
                <div className="flex justify-end gap-2">
                    <button type="button" onClick={onClose} disabled={loading} className={buttons.secondary}>{t('common.cancel')}</button>
                    <button type="submit" disabled={loading || !password} className={cn(buttons.primary)}>
                        {loading ? t('auth.connecting') : t('auth.connect')}
                    </button>
                </div>
            </form>
        </Modal>
    );
}
