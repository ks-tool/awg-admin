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
import type {ChangeEvent} from 'react';
import {useTranslation} from 'react-i18next';
import {toast} from 'sonner';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs} from '@/components/common/Modal';
import {cn} from '@/lib/utils';
import type {AuthType, ServerFormData} from '@/hooks/useServerForm';
import {generateAgentTLS} from '@/services/servers';
import type {Server} from '@/types';

interface Props {
    formData: ServerFormData;
    onChange: (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => void;
    disabled?: boolean;
    /** Показывать блок SSH-аутентификации (ключ / пароль). */
    showAuth?: boolean;
    authType?: AuthType;
    onAuthTypeChange?: (type: AuthType) => void;
    /** ID существующего сервера — нужен только для генерации mTLS-сертификатов. */
    serverId?: string;
    /** Уже выпущены сертификаты для агента (CA непустой). */
    hasTLSCertificates?: boolean;
    /** Вызывается после успешной (пере)генерации сертификатов. */
    onCertificatesGenerated?: (server: Server) => void;
    /** У сервера уже есть загруженный в БД ключ (формы редактирования). */
    hasStoredKey?: boolean;
}

/**
 * Переиспользуемые поля формы сервера.
 * Используется в AddServer (создание) и ServerDetail (редактирование).
 */
export function ServerFormFields({
    formData,
    onChange,
    disabled = false,
    showAuth = false,
    authType = 'key',
    onAuthTypeChange,
    serverId,
    hasTLSCertificates = false,
    onCertificatesGenerated,
    hasStoredKey = false,
}: Props) {
    const {t} = useTranslation();
    const [generatingTLS, setGeneratingTLS] = useState(false);
    const [keyFileName, setKeyFileName] = useState('');

    // Reading the file client-side works the same way in both the Wails
    // desktop window and a plain browser tab — no native file dialog/path
    // resolution needed, so the key's content goes straight into the DB.
    const handleKeyFileUpload = async (e: ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file) return;
        const text = await file.text();
        setKeyFileName(file.name);
        onChange({target: {name: 'ssh.keyData', value: text, type: 'text'}} as unknown as ChangeEvent<HTMLInputElement>);
        onChange({target: {name: 'ssh.key', value: '', type: 'text'}} as unknown as ChangeEvent<HTMLInputElement>);
        e.target.value = '';
    };

    const handleClearUploadedKey = () => {
        setKeyFileName('');
        onChange({target: {name: 'ssh.keyData', value: '', type: 'text'}} as unknown as ChangeEvent<HTMLInputElement>);
    };

    // Without mTLS the agent is only ever reached through the SSH tunnel
    // (which always lands on the agent's own loopback), so the address field
    // is locked to 127.0.0.1 — only the port is meaningful. Enabling mTLS
    // switches to a direct connection, so the host part needs to become the
    // server's real address; auto-fill it from the SSH host the first time
    // TLS is turned on, keeping whatever port was already set.
    const handleTlsToggle = (e: ChangeEvent<HTMLInputElement>) => {
        const enabling = e.target.checked;
        onChange(e);

        const port = formData.agent.addr.split(':')[1] || '8080';
        const newAddr = enabling
            ? (formData.ssh.host.trim() ? `${formData.ssh.host.trim()}:${port}` : formData.agent.addr)
            : `127.0.0.1:${port}`;

        onChange({target: {name: 'agent.addr', value: newAddr, type: 'text'}} as unknown as ChangeEvent<HTMLInputElement>);
    };

    const handleGenerateCertificates = async () => {
        if (!serverId) return;
        setGeneratingTLS(true);
        try {
            const server = await generateAgentTLS(serverId);
            if (server) {
                onCertificatesGenerated?.(server);
            } else {
                toast.error(t('servers.certificatesGenerateError'));
            }
        } finally {
            setGeneratingTLS(false);
        }
    };

    return (
        <div className="space-y-4">
            {/* Name */}
            <FormField label={t('common.name')}>
                <input
                    type="text"
                    name="name"
                    value={formData.name}
                    onChange={onChange}
                    placeholder={t('servers.namePlaceholder')}
                    disabled={disabled}
                    required
                    className={inputs.primary}
                />
            </FormField>

            {/* Description */}
            <FormField label={t('common.description')}>
                <textarea
                    name="info.description"
                    value={formData.info.description}
                    onChange={onChange}
                    placeholder={t('common.descriptionPlaceholder')}
                    disabled={disabled}
                    rows={3}
                    className={cn(inputs.primary, 'resize-none')}
                />
            </FormField>

            {/* Location */}
            <FormField label={t('servers.location')}>
                <input
                    type="text"
                    name="info.location"
                    value={formData.info.location}
                    onChange={onChange}
                    placeholder={t('servers.locationPlaceholder')}
                    disabled={disabled}
                    className={inputs.primary}
                />
            </FormField>

            {/* SSH host + port */}
            <div className="grid grid-cols-2 gap-4">
                <FormField label={t('servers.host')}>
                    <input
                        type="text"
                        name="ssh.host"
                        value={formData.ssh.host}
                        onChange={onChange}
                        placeholder="example.com"
                        disabled={disabled}
                        required
                        className={inputs.primary}
                    />
                </FormField>
                <FormField label={t('servers.port')}>
                    <input
                        type="number"
                        name="ssh.port"
                        value={formData.ssh.port}
                        onChange={onChange}
                        min="1"
                        max="65535"
                        disabled={disabled}
                        className={inputs.primary}
                    />
                </FormField>
            </div>

            {/* SSH user */}
            <FormField label={t('servers.sshUser')}>
                <input
                    type="text"
                    name="ssh.user"
                    value={formData.ssh.user}
                    onChange={onChange}
                    placeholder="root"
                    disabled={disabled}
                    className={inputs.primary}
                />
            </FormField>

            {/* SSH authentication */}
            {showAuth && onAuthTypeChange && (
                <>
                    <div>
                        <label className="block text-sm font-medium text-foreground dark:text-zinc-300 mb-3">
                            {t('servers.sshAuthentication')}
                        </label>
                        <div className="flex gap-4 mb-4">
                            {(['key', 'password'] as AuthType[]).map(type => (
                                <label key={type} className="flex items-center">
                                    <input
                                        type="radio"
                                        checked={authType === type}
                                        onChange={() => onAuthTypeChange(type)}
                                        disabled={disabled}
                                        className="rounded border-input bg-background dark:border-white/10 dark:bg-white/5 text-sky-500"
                                    />
                                    <span className="ml-2 text-sm text-foreground dark:text-zinc-300">
                                        {type === 'key' ? t('servers.authTypeKey') : t('servers.authTypePassword')}
                                    </span>
                                </label>
                            ))}
                        </div>
                    </div>

                    {authType === 'key' ? (
                        <FormField label={t('servers.keyFile')}>
                            <div className="flex items-center gap-2">
                                <input
                                    type="file"
                                    id="ssh-key-upload"
                                    className="hidden"
                                    onChange={handleKeyFileUpload}
                                    disabled={disabled}
                                />
                                <label
                                    htmlFor="ssh-key-upload"
                                    className={cn(buttons.secondary, 'cursor-pointer', disabled && 'pointer-events-none opacity-50')}
                                >
                                    {t('servers.uploadKeyFile')}
                                </label>
                                {(keyFileName || formData.ssh.keyData) ? (
                                    <span className="text-xs text-muted-foreground dark:text-zinc-500 flex items-center gap-2">
                                        {keyFileName || t('servers.keyUploaded')}
                                        <button
                                            type="button"
                                            onClick={handleClearUploadedKey}
                                            disabled={disabled}
                                            className="text-red-500 hover:underline"
                                        >
                                            ✕
                                        </button>
                                    </span>
                                ) : hasStoredKey ? (
                                    <span className="text-xs text-emerald-600 dark:text-emerald-400">
                                        {t('servers.keyAlreadyStored')}
                                    </span>
                                ) : null}
                            </div>
                        </FormField>
                    ) : (
                        <FormField label={t('auth.password')}>
                            <input
                                type="password"
                                name="ssh.password"
                                value={formData.ssh.password}
                                onChange={onChange}
                                placeholder={t('servers.sshPasswordPlaceholder')}
                                disabled={disabled}
                                className={inputs.primary}
                            />
                        </FormField>
                    )}
                </>
            )}

            {/* Agent address */}
            <FormField label={t('servers.agentAddress')}>
                <input
                    type="text"
                    name="agent.addr"
                    value={formData.agent.addr}
                    onChange={onChange}
                    placeholder="localhost:8080"
                    disabled={disabled || !formData.agent.tlsEnabled}
                    title={formData.agent.tlsEnabled ? undefined : t('servers.agentAddressHint')}
                    required
                    className={inputs.primary}
                />
            </FormField>

            {/* Agent TLS */}
            <FormField label="">
                <label className="flex items-center">
                    <input
                        type="checkbox"
                        name="agent.tlsEnabled"
                        checked={formData.agent.tlsEnabled}
                        onChange={handleTlsToggle}
                        disabled={disabled}
                        className="rounded border-white/10 bg-white/5 text-sky-500 focus:ring-sky-500 focus:ring-offset-0"
                    />
                    <span className="ml-3 text-sm font-medium text-foreground dark:text-zinc-300">
                        {t('servers.agentEnabledTls')}
                    </span>
                </label>
            </FormField>

            {formData.agent.tlsEnabled && serverId && (
                <FormField label="">
                    <div className="flex items-center gap-3">
                        <button
                            type="button"
                            onClick={handleGenerateCertificates}
                            disabled={disabled || generatingTLS}
                            className={buttons.secondary}
                        >
                            {generatingTLS
                                ? t('common.generating')
                                : hasTLSCertificates
                                    ? t('servers.regenerateTlsCerts')
                                    : t('servers.generateTlsCerts')}
                        </button>
                        {hasTLSCertificates && (
                            <span className="text-xs text-muted-foreground dark:text-zinc-500">
                                {t('servers.certificatesIssued')}
                            </span>
                        )}
                    </div>
                </FormField>
            )}
        </div>
    );
}
