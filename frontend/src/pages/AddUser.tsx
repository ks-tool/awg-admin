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

import type {ChangeEvent, SubmitEvent} from 'react';
import {useCallback, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {toast} from 'sonner';
import {ArrowLeft, User as UserIcon} from 'lucide-react';
import {PageHeader} from '@/components/layout/PageHeader';
import {useNavigation} from '@/contexts/NavigationContext';
import {useAppStore} from '@/store';
import {FormField} from '@/components/common/FormField';
import {buttons, inputs} from '@/components/common/Modal';
import {cn} from '@/lib/utils';

export default function AddUser() {
    const {t} = useTranslation();
    const {navigate} = useNavigation();
    const {createUser} = useAppStore();

    const [isLoading, setIsLoading] = useState(false);
    const [formData, setFormData] = useState({
        name: '',
        description: '',
    });

    const handleInputChange = useCallback(
        (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
            const {name, value} = e.target;
            setFormData(prev => ({...prev, [name]: value}));
        },
        []
    );

    const handleSubmit = async (e: SubmitEvent) => {
        e.preventDefault();

        if (!formData.name.trim()) {
            toast.error(t('users.nameRequired'));
            return;
        }

        setIsLoading(true);
        try {
            const newUser = await createUser({
                name: formData.name,
                description: formData.description || undefined,
            });

            if (newUser) {
                toast.success(t('users.userCreated'));
                navigate('users');
            } else {
                toast.error(t('users.createUserError'));
            }
        } catch (error) {
            console.error('Failed to create user:', error);
            toast.error(t('users.createUserError'));
        } finally {
            setIsLoading(false);
        }
    };

    const handleCancel = () => navigate('users');

    return (
        <div className="flex flex-col">
            <PageHeader
                title={t('users.addUser')}
                icon={UserIcon}
                description={t('users.addUserDescription')}
                actions={
                    <button
                        onClick={handleCancel}
                        className={cn(buttons.secondary, "flex items-center gap-2")}
                        title={t('users.backToUsers')}
                    >
                        <ArrowLeft size={14}/>
                    </button>
                }
            />

            <div className="p-8">
                <div className="max-w-2xl">
                    <form onSubmit={handleSubmit} className="space-y-6">
                        <FormField label={t('common.name')}>
                            <input
                                type="text"
                                name="name"
                                value={formData.name}
                                onChange={handleInputChange}
                                placeholder={t('users.namePlaceholder')}
                                className={inputs.primary}
                            />
                        </FormField>

                        <FormField label={t('common.description')}>
                            <textarea
                                name="description"
                                value={formData.description}
                                onChange={handleInputChange}
                                placeholder={t('common.descriptionPlaceholder')}
                                rows={4}
                                className={cn(inputs.primary, "resize-none")}
                            />
                        </FormField>

                        <div className="flex gap-3 pt-4">
                            <button type="submit" disabled={isLoading} className={buttons.primary}>
                                {isLoading ? `${t('common.creating')}...` : t('common.create')}
                            </button>
                            <button type="button" onClick={handleCancel} disabled={isLoading}
                                    className={buttons.secondary}>
                                {t('common.cancel')}
                            </button>
                        </div>
                    </form>
                </div>
            </div>
        </div>
    );
}