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

import {buttons, Modal} from './Modal';
import {useTranslation} from 'react-i18next';

interface Props {
    title: string;
    message: string;
    confirmLabel?: string;
    onConfirm: () => void;
    onCancel: () => void;
    loading?: boolean;
}

// window.confirm()/alert() never render in the Wails desktop webview, so any
// destructive action gated on confirm() silently no-ops when running as a
// desktop app — use this instead of confirm() for anything destructive.
export function ConfirmModal({title, message, confirmLabel, onConfirm, onCancel, loading}: Props) {
    const {t} = useTranslation();

    return (
        <Modal title={title} onClose={onCancel} loading={loading}>
            <p className="text-sm text-muted-foreground dark:text-zinc-400 mb-6">{message}</p>
            <div className="flex justify-end gap-2">
                <button onClick={onCancel} disabled={loading} className={buttons.secondary}>
                    {t('common.cancel')}
                </button>
                <button onClick={onConfirm} disabled={loading} className={buttons.danger}>
                    {confirmLabel ?? t('common.delete')}
                </button>
            </div>
        </Modal>
    );
}
