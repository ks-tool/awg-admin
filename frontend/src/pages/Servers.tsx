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

import {useState} from 'react'
import {useTranslation} from 'react-i18next'
import {Plus, RefreshCw, Server} from 'lucide-react'
import {useAppStore} from '@/store'
import {CopyButton} from '@/components/common/CopyButton'
import {SearchBar} from '@/components/common/SearchBar'
import {PageHeader} from '@/components/layout/PageHeader'
import {useNavigation} from '@/contexts/NavigationContext'
import {useAutoRefresh} from "@/hooks/useAutoRefresh.tsx";

export default function Servers() {
    const { t } = useTranslation()
    const { servers, isLoadingServers, refreshData, setSelectedServer } = useAppStore()
    const { navigate } = useNavigation()
    const [search, setSearch] = useState('')

    // Auto-fetch data on component mount
    useAutoRefresh(refreshData)

    const handleAddServer = () => {
        navigate('add-server')
    }

    const handleServerClick = (serverId: string) => {
        setSelectedServer(serverId)
        navigate('server-detail')
    }

    const filtered = servers.filter((s) => {
        const q = search.toLowerCase()
        return !q ||
            s.name.toLowerCase().includes(q) ||
            s.ssh.host.toLowerCase().includes(q) ||
            s.info?.location?.toLowerCase().includes(q)
    })

    return (
        <div className="flex flex-col">
            <PageHeader
                title={t('servers.title')}
                icon={Server}
                description={`${servers.length} ${t('servers.configuredSuffix')}`}
                actions={
                    <button 
                        onClick={handleAddServer}
                        className="rounded-lg px-4 py-2 text-sm font-medium transition-colors bg-sky-100 text-sky-700 border border-sky-200 hover:bg-sky-200 dark:bg-sky-500/15 dark:text-sky-400 dark:border-sky-500/25 dark:hover:bg-sky-500/20"
                        title={t('servers.addServer')}
                    >
                        <Plus size={14} />
                    </button>
                }
            />

            {/* Search filter */}
            <div className="flex items-center gap-3 border-b border-white/5 px-8 py-4">
                <SearchBar value={search} onChange={setSearch} />
            </div>

            <div className="p-8 space-y-3">
                {filtered.length === 0 && (
                    <div className="py-24 text-center text-zinc-600">
                        {isLoadingServers ? (
                            <>
                                <RefreshCw className="w-4 h-4 inline mr-2 animate-spin" />
                                {t('common.loading')}
                            </>
                        ) : (
                            t('servers.noServers')
                        )}
                    </div>
                )}
                {filtered.map((server) => (
                    <div
                        key={server.id}
                        onClick={() => handleServerClick(server.id)}
                        className="group rounded-xl border border-border bg-card dark:border-white/5 dark:bg-white/3 p-5 transition-all cursor-pointer"
                    >
                        <div className="flex flex-wrap items-start gap-6">
                            {/* Name + description */}
                            <div className="min-w-56">
                                <div className="font-semibold text-muted-foreground mb-1">{server.name}</div>
                                {server.info?.description && (
                                    <div className="text-xs text-zinc-500 mb-2">{server.info.description}</div>
                                )}
                                {server.info?.location && (
                                    <div className="text-xs text-zinc-600 mb-2">{server.info.location}</div>
                                )}
                                {server.info?.tags && server.info.tags.length > 0 && (
                                    <div className="mt-2 flex flex-wrap gap-1">
                                        {server.info.tags.map((tag) => (
                                            <span
                                                key={tag}
                                                className="rounded px-1.5 py-0.5 text-[10px] font-medium bg-white/5 text-zinc-500"
                                            >
                                                {tag}
                                            </span>
                                        ))}
                                    </div>
                                )}
                            </div>

                            {/* SSH Connection */}
                            <div className="flex-1 min-w-48">
                                <p className="mb-1 text-[10px] uppercase tracking-wider text-zinc-600">
                                    {t('servers.host')}
                                </p>
                                <div className="flex items-center gap-1 mb-1">
                                    <span className="font-mono text-sm text-foreground dark:text-zinc-300">
                                        {server.ssh.host}:{server.ssh.port || 22}
                                    </span>
                                    <CopyButton value={`${server.ssh.host}:${server.ssh.port || 22}`} />
                                </div>
                                {server.ssh.user && (
                                    <div className="text-xs text-zinc-600 font-mono">{t('common.user')}: {server.ssh.user}</div>
                                )}
                            </div>

                            {/* Agent Connection */}
                            <div className="flex-1 min-w-48">
                                <p className="mb-1 text-[10px] uppercase tracking-wider text-zinc-600">
                                    {t('servers.agentLabel')}
                                </p>
                                <div className="flex items-center gap-1">
                                    <span className="font-mono text-sm text-foreground dark:text-zinc-300 truncate">
                                        {server.agent.addr}
                                    </span>
                                    <CopyButton value={server.agent.addr} />
                                </div>
                                {server.agent.tls && (
                                    <div className="text-xs text-emerald-600 mt-1">{t('servers.tlsEnabledBadge')}</div>
                                )}
                            </div>

                            {/* Interfaces count */}
                            <div className="text-right">
                                <div className="text-2xl font-bold text-foreground dark:text-zinc-200">
                                    {server.interfaces?.length || 0}
                                </div>
                                <div className="text-xs text-zinc-600">{t('servers.interfacesCount')}</div>
                            </div>
                        </div>
                    </div>
                ))}
            </div>
        </div>
    )
}
