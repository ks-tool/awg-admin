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

import type {FormEvent} from 'react';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';
import {toast} from 'sonner';
import {ArrowLeft, Server} from 'lucide-react';
import {PageHeader} from '@/components/layout/PageHeader';
import {useNavigation} from '@/contexts/NavigationContext';
import {useAppStore} from '@/store';
import {buttons} from '@/components/common/Modal';
import {ServerFormFields} from '@/components/server/ServerFormFields';
import {formDataToServerInput, useServerForm} from '@/hooks/useServerForm';
import type {AuthType} from '@/hooks/useServerForm';

export default function AddServer() {
    const {t} = useTranslation();
    const {navigate} = useNavigation();
    const {createServer} = useAppStore();

    const [isLoading, setIsLoading] = useState(false);
    const [authType, setAuthType] = useState<AuthType>('key');
    const {formData, updateField} = useServerForm();

    const handleSubmit = async (e: FormEvent) => {
        e.preventDefault();

        if (!formData.name.trim()) return toast.error(t('servers.nameRequired'));
        if (!formData.ssh.host.trim()) return toast.error(t('servers.hostRequired'));
        if (!formData.agent.addr.trim()) return toast.error(t('servers.agentAddrRequired'));
        if (!formData.ssh.key.trim() && !formData.ssh.keyData.trim() && !formData.ssh.password.trim())
            return toast.error(t('servers.sshAuthRequired'));

        setIsLoading(true);
        try {
            const newServer = await createServer(formDataToServerInput(formData, authType));
            if (newServer) {
                toast.success(t('servers.serverCreated'));
                navigate('servers');
            } else {
                toast.error(t('servers.createServerError'));
            }
        } catch (error) {
            console.error('Failed to create server:', error);
            toast.error(t('servers.createServerError'));
        } finally {
            setIsLoading(false);
        }
    };

    return (
        <div className="flex flex-col">
            <PageHeader
                title={t('servers.addServer')}
                icon={Server}
                description={t('servers.addServerDescription')}
                actions={
                    <button
                        onClick={() => navigate('servers')}
                        className="rounded-lg px-4 py-2 text-sm font-medium bg-muted text-muted-foreground border border-border hover:bg-secondary dark:bg-zinc-500/15 dark:text-zinc-400 dark:border-zinc-500/25 dark:hover:bg-zinc-500/20 flex items-center gap-2"
                        title={t('servers.backToServers')}
                    >
                        <ArrowLeft size={14}/>
                    </button>
                }
            />

            <div className="p-8">
                <div className="max-w-2xl">
                    <form onSubmit={handleSubmit} className="space-y-6">
                        <ServerFormFields
                            formData={formData}
                            onChange={updateField}
                            disabled={isLoading}
                            showAuth
                            authType={authType}
                            onAuthTypeChange={setAuthType}
                        />

                        <div className="flex gap-3 pt-4">
                            <button type="submit" disabled={isLoading} className={buttons.primary}>
                                {isLoading ? `${t('common.creating')}...` : t('common.create')}
                            </button>
                            <button
                                type="button"
                                onClick={() => navigate('servers')}
                                disabled={isLoading}
                                className={buttons.secondary}
                            >
                                {t('common.cancel')}
                            </button>
                        </div>
                    </form>
                </div>
            </div>
        </div>
    );
}
