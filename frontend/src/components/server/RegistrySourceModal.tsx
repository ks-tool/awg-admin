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
import {toast} from 'sonner';
import {FormField} from '@/components/common/FormField';
import {Modal} from '@/components/common/Modal';
import {buttons, inputs} from '@/components/common/controls';
import {createRegistrySource, updateRegistrySource} from '@/services/registrySources';
import {cn} from '@/lib/utils';
import type {RegistrySource} from '@/types';

export function RegistrySourceModal({source, onClose, onSaved}: {
    source?: RegistrySource;
    onClose: () => void;
    onSaved: (source: RegistrySource) => void;
}) {
    const {t} = useTranslation();
    const [name, setName] = useState(source?.name ?? '');
    const [url, setUrl] = useState(source?.url ?? '');
    const [agentRepository, setAgentRepository] = useState(source?.agentRepository ?? '');
    const [providerRepository, setProviderRepository] = useState(source?.providerRepository ?? '');
    const [username, setUsername] = useState(source?.username ?? '');
    const [password, setPassword] = useState('');
    const [plainHttp, setPlainHttp] = useState(source?.plainHttp ?? false);
    const [saving, setSaving] = useState(false);

    const save = async () => {
        if (!name.trim() || !url.trim()) {
            toast.error(t('servers.registryFieldsRequired'));
            return;
        }
        setSaving(true);
        try {
            const input = {
                name: name.trim(), url: url.trim(), agentRepository: agentRepository.trim(),
                providerRepository: providerRepository.trim(), username: username.trim(), password, plainHttp,
            };
            const saved = source
                ? await updateRegistrySource(source.id, input)
                : await createRegistrySource(input);
            onSaved(saved);
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('servers.registrySaveError'));
        } finally {
            setSaving(false);
        }
    };

    return (
        <Modal title={source ? t('servers.editRegistryTitle') : t('servers.addRegistryTitle')} onClose={onClose} loading={saving}>
            <div className="space-y-4">
                <FormField label={t('servers.sourceName')}>
                    <input value={name} onChange={event => setName(event.target.value)} disabled={saving} className={inputs.primary} placeholder="Private OCI" />
                </FormField>
                <FormField label={t('servers.registryAddress')}>
                    <input value={url} onChange={event => setUrl(event.target.value)} disabled={saving} className={cn(inputs.primary, 'font-mono text-xs')} placeholder="oci://registry.example:5000" />
                </FormField>
                <FormField label={t('servers.agentRepository')}>
                    <input value={agentRepository} onChange={event => setAgentRepository(event.target.value)} disabled={saving} className={cn(inputs.primary, 'font-mono text-xs')} placeholder="oci://ghcr.io/ks-tool/vpn-agent" />
                </FormField>
                <FormField label={t('servers.providerCatalog')}>
                    <>
                        <input value={providerRepository} onChange={event => setProviderRepository(event.target.value)} disabled={saving} className={cn(inputs.primary, 'font-mono text-xs')} placeholder="oci://ghcr.io/ks-tool/vpn-providers" />
                        <p className="text-xs text-muted-foreground">{t('settings.repositoryDefaultsHint')}</p>
                    </>
                </FormField>
                <div className="grid grid-cols-2 gap-3">
                    <FormField label={t('servers.registryUsername')}>
                        <input value={username} onChange={event => setUsername(event.target.value)} disabled={saving} className={inputs.primary} autoComplete="username" />
                    </FormField>
                    <FormField label={t('servers.registryPassword')}>
                        <input type="password" value={password} onChange={event => setPassword(event.target.value)} disabled={saving} className={inputs.primary} autoComplete="new-password" placeholder={source?.hasPassword ? t('servers.registryPasswordUnchanged') : ''} />
                    </FormField>
                </div>
                <label className="flex items-center gap-3 text-sm">
                    <input type="checkbox" checked={plainHttp} onChange={event => setPlainHttp(event.target.checked)} disabled={saving} />
                    {t('servers.registryPlainHttp')}
                </label>
                <p className="text-xs text-muted-foreground">{t('servers.registryHint')}</p>
                <div className="flex justify-end gap-2 pt-2">
                    <button type="button" onClick={onClose} disabled={saving} className={buttons.secondary}>{t('common.cancel')}</button>
                    <button type="button" onClick={() => void save()} disabled={saving} className={buttons.primary}>{saving ? t('common.saving') : t('common.save')}</button>
                </div>
            </div>
        </Modal>
    );
}
