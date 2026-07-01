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

import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { LayoutDashboard, Server, Network, Users, Cable, RefreshCw } from 'lucide-react'
import { useAppStore } from '@/store'
import { StatCard } from '@/components/common/StatCard'
import { StatusBadge } from '@/components/common/StatusBadge'
import { PageHeader } from '@/components/layout/PageHeader'
import { getServerMetrics, getServerTunnelStatus } from '@/services/servers'
import { ServerMetricsModal } from '@/components/server/ServerMetricsModal'
import type { MetricsSnapshot } from '@/types'
import {useAutoRefresh} from "@/hooks/useAutoRefresh.tsx";

export default function Dashboard() {
    const { t } = useTranslation()
    const { stats, servers, refreshData } = useAppStore()
    const [metricsByServer, setMetricsByServer] = useState<Record<string, MetricsSnapshot | null>>({})
    const [tunnelOpenByServer, setTunnelOpenByServer] = useState<Record<string, boolean | null>>({})
    const [metricsModalServerId, setMetricsModalServerId] = useState<string | null>(null)

    // Auto-fetch data on component mount
    useAutoRefresh(refreshData)

    // Agent metrics are best-effort/secondary, so fetched separately from
    // the main store data — a server with an unreachable agent shouldn't
    // block the rest of the dashboard from loading.
    useEffect(() => {
        let cancelled = false
        Promise.all(servers.map(async (server) => [server.id, await getServerMetrics(server.id)] as const))
            .then((entries) => {
                if (cancelled) return
                // Merge rather than replace the whole map: a single failing tick
                // (agent momentarily unreachable — getServerMetrics swallows the
                // error to null) must not blank a row that was already populated.
                // Keep the last known snapshot on a transient miss; "—" is only
                // shown until a server's very first successful fetch. Reachability
                // is still signalled separately by the tunnel/online badge.
                setMetricsByServer((prev) => {
                    const next: Record<string, MetricsSnapshot | null> = { ...prev }
                    for (const [id, snapshot] of entries) {
                        if (snapshot !== null) next[id] = snapshot
                        else if (!(id in next)) next[id] = null
                    }
                    return next
                })
            })
        return () => {
            cancelled = true
        }
    }, [servers])

    // Tunnel status only applies to servers without mTLS configured — mTLS
    // servers are reached directly and never have one, so skip the call for
    // those instead of reporting a misleading "closed".
    useEffect(() => {
        let cancelled = false
        const tunnelledServers = servers.filter((server) => !server.agent?.tls)
        Promise.all(tunnelledServers.map(async (server) => [server.id, await getServerTunnelStatus(server.id)] as const))
            .then((entries) => {
                if (!cancelled) setTunnelOpenByServer(Object.fromEntries(entries))
            })
        return () => {
            cancelled = true
        }
    }, [servers])

    if (!stats) {
        return (
            <div className="flex flex-col">
                <PageHeader title={t('dashboard.title')} icon={LayoutDashboard} />
                <div className="p-8 flex items-center justify-center h-screen text-muted-foreground dark:text-zinc-400">
                    <RefreshCw className="w-5 h-5 mr-2 animate-spin" />
                    {t('common.loading')}
                </div>
            </div>
        )
    }

    return (
        <div className="flex flex-col">
            <PageHeader title={t('dashboard.title')} icon={LayoutDashboard} />

            <div className="p-8 space-y-8">
                {/* Stats grid */}
                <div className="grid grid-cols-2 gap-4 xl:grid-cols-4">
                    <StatCard
                        label={t('dashboard.totalServers')}
                        value={stats.totalServers}
                        sub={`${stats.totalServers} ${t('dashboard.servers').toLowerCase()}`}
                        icon={Server}
                        accent="sky"
                    />
                    <StatCard
                        label={t('dashboard.totalPeers')}
                        value={stats.totalPeers}
                        sub={`${stats.activePeers} ${t('dashboard.activePeers').toLowerCase()}`}
                        icon={Network}
                        accent="emerald"
                    />
                    <StatCard
                        label={t('dashboard.totalUsers')}
                        value={stats.totalUsers}
                        icon={Users}
                        accent="violet"
                    />
                    <StatCard
                        label={t('dashboard.totalTunnels')}
                        value={stats.totalTunnels}
                        icon={Cable}
                        accent="amber"
                    />
                </div>

                <div>
                    {/* Servers table */}
                    <div className="rounded-xl border border-border bg-card dark:border-white/5 dark:bg-white/3 overflow-hidden">
                        <div className="border-b border-white/5 px-5 py-4">
                            <h2 className="text-sm font-semibold text-zinc-500">{t('nav.servers')}</h2>
                        </div>
                        <table className="w-full text-sm">
                            <thead>
                                <tr className="border-b border-white/5">
                                    <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.name')}
                                    </th>
                                    <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.host')}
                                    </th>
                                    <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.tunnelStatus')}
                                    </th>
                                    <th className="px-5 py-3 text-right text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.peers')}
                                    </th>
                                    <th className="px-5 py-3 text-right text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.loadAverage')}
                                    </th>
                                    <th className="px-5 py-3 text-right text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        RAM
                                    </th>
                                </tr>
                            </thead>
                            <tbody>
                                {servers.map((server) => {
                                    const metrics = metricsByServer[server.id]?.system
                                    return (
                                        <tr
                                            key={server.id}
                                            onClick={() => setMetricsModalServerId(server.id)}
                                            className="cursor-pointer border-b border-white/3 last:border-0 hover:bg-white/2 transition-colors"
                                        >
                                            <td className="px-5 py-3">
                                                <div className="font-medium dark:text-zinc-200">{server.name}</div>
                                            </td>
                                            <td className="px-5 py-3">
                                                <div className="text-xs dark:text-zinc-400 font-mono">{server.ssh.host}</div>
                                            </td>
                                            <td className="px-5 py-3">
                                                {server.agent?.tls ? (
                                                    <span className="text-xs text-zinc-600">{t('servers.tunnelNotApplicable')}</span>
                                                ) : (
                                                    <StatusBadge
                                                        status={
                                                            tunnelOpenByServer[server.id] == null
                                                                ? 'unknown'
                                                                : tunnelOpenByServer[server.id]
                                                                    ? 'online'
                                                                    : 'offline'
                                                        }
                                                        size="sm"
                                                    />
                                                )}
                                            </td>
                                            <td className="px-5 py-3 text-right font-mono dark:text-zinc-400">
                                                {server.interfaces?.length || 0}
                                            </td>
                                            <td className="px-5 py-3 text-right font-mono text-xs dark:text-zinc-400">
                                                {metrics
                                                    ? `${metrics.load1.toFixed(2)} / ${metrics.load5.toFixed(2)} / ${metrics.load15.toFixed(2)}`
                                                    : '—'}
                                            </td>
                                            <td className="px-5 py-3 text-right font-mono text-xs dark:text-zinc-400">
                                                {metrics && metrics.memTotalBytes > 0
                                                    ? `${((metrics.memUsedBytes / metrics.memTotalBytes) * 100).toFixed(0)}%`
                                                    : '—'}
                                            </td>
                                        </tr>
                                    )
                                })}
                            </tbody>
                        </table>
                        {servers.length === 0 && (
                            <div className="px-5 py-8 text-center text-sm text-zinc-600">
                                {t('common.noData')}
                            </div>
                        )}
                    </div>
                </div>
            </div>

            {metricsModalServerId && (
                <ServerMetricsModal
                    serverId={metricsModalServerId}
                    serverName={servers.find((s) => s.id === metricsModalServerId)?.name ?? ''}
                    onClose={() => setMetricsModalServerId(null)}
                />
            )}
        </div>
    )
}
