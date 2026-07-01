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

import { useTranslation } from 'react-i18next'
import { Users as UsersIcon, Plus, RefreshCw, User as UserIcon } from 'lucide-react'
import { useAppStore } from '@/store'
import { StatusBadge } from '@/components/common/StatusBadge'
import { PageHeader } from '@/components/layout/PageHeader'
import { useNavigation } from '@/contexts/NavigationContext'
import {useAutoRefresh} from "@/hooks/useAutoRefresh";
import {buttons} from "@/components/common/Modal";

export default function Users() {
    const { t } = useTranslation()
    const { users, isLoadingUsers, refreshData, setSelectedUser } = useAppStore()
    const { navigate } = useNavigation()

    // Auto-fetch data on component mount
    useAutoRefresh(refreshData)

    const handleAddUser = () => {
        navigate('add-user')
    }

    const handleSelectUser = (userId: string) => {
        setSelectedUser(userId)
        navigate('user-detail')
    }

    return (
        <div className="flex flex-col">
            <PageHeader
                title={t('users.title')}
                icon={UsersIcon}
                description={`${users.length} ${t('common.total')}`}
                actions={
                    <button 
                        onClick={handleAddUser}
                        className={buttons.primary}
                        title={t('users.addUser')}
                    >
                        <Plus size={14} />
                    </button>
                }
            />

            <div className="p-8">
                <div className="rounded-xl border border-white/5 overflow-hidden">
                    <table className="w-full text-sm">
                        <thead>
                            <tr className="border-b border-white/5 bg-white/2">
                                <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                    {t('common.name')}
                                </th>
                                <th className="px-5 py-3 text-center text-xs font-medium uppercase tracking-wider text-zinc-600">
                                    {t('users.peers')}
                                </th>
                                <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                    {t('common.description')}
                                </th>
                                <th className="px-5 py-3 text-center text-xs font-medium uppercase tracking-wider text-zinc-600">
                                    {t('common.status')}
                                </th>
                            </tr>
                        </thead>
                        <tbody className="divide-y divide-white/3">
                            {users.length === 0 ? (
                                <tr>
                                    <td colSpan={4} className="py-16 text-center text-zinc-600">
                                        {isLoadingUsers ? (
                                            <>
                                                <RefreshCw className="w-4 h-4 inline mr-2 animate-spin" />
                                                {t('common.loading')}
                                            </>
                                        ) : (
                                            t('users.noUsers')
                                        )}
                                    </td>
                                </tr>
                            ) : (
                                users.map((user) => {
                                    const initials = user.name
                                        ? user.name.split(' ').map(n => n[0]).join('').toUpperCase().slice(0, 2)
                                        : '?'
                                    
                                    return (
                                        <tr 
                                            key={user.id} 
                                            onClick={() => handleSelectUser(user.id)}
                                            className="hover:bg-white/2 transition-colors cursor-pointer"
                                        >
                                            <td className="px-5 py-3">
                                                <div className="flex items-center gap-3">
                                                    <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted dark:bg-white/8 border border-border dark:border-white/10 font-semibold text-foreground dark:text-zinc-300 text-xs">
                                                        {initials === '?' ? (
                                                            <UserIcon size={14} />
                                                        ) : (
                                                            initials
                                                        )}
                                                    </div>
                                                    <div className="flex-1 min-w-0">
                                                        <div className="font-medium text-foreground dark:text-zinc-200 truncate">{user.name || '—'}</div>
                                                    </div>
                                                </div>
                                            </td>
                                            <td className="px-5 py-3 text-center font-mono text-muted-foreground dark:text-zinc-400">
                                                {user.peers?.length || 0}
                                            </td>
                                            <td className="px-5 py-3 text-xs text-zinc-600 max-w-xs truncate">
                                                {user.description || '—'}
                                            </td>
                                            <td className="px-5 py-3 text-center">
                                                <StatusBadge status={!user.disabled ? 'enabled' : 'disabled'} size="sm" />
                                            </td>
                                        </tr>
                                    )
                                })
                            )}
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    )
}
