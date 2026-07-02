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

import { useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {LayoutDashboard, Server, Network, Users, Waypoints, RefreshCw} from 'lucide-react'
import { useAppStore } from '@/store'
import { StatCard } from '@/components/common/StatCard'
import { StatusBadge } from '@/components/common/StatusBadge'
import { PageHeader } from '@/components/layout/PageHeader'
import { getServerMetrics, getServerAgentStatus, getServerHostInfo, type AgentStatus } from '@/services/servers'
import { ServerMetricsModal } from '@/components/server/ServerMetricsModal'
import { HostInfoBadges } from '@/components/server/HostInfoBadges'
import { b64ToHex, isPeerConnected } from '@/lib/peers'
import type { HostInfo, MetricsSnapshot } from '@/types'
import {useAutoRefresh} from "@/hooks/useAutoRefresh.tsx";

export default function Dashboard() {
    const { t } = useTranslation()
    const { stats, servers, users, refreshData } = useAppStore()
    const [metricsByServer, setMetricsByServer] = useState<Record<string, MetricsSnapshot | null>>({})
    const [agentStatusByServer, setAgentStatusByServer] = useState<Record<string, AgentStatus | null>>({})
    const [hostInfoByServer, setHostInfoByServer] = useState<Record<string, HostInfo | null>>({})
    const [metricsModalServerId, setMetricsModalServerId] = useState<string | null>(null)

    // Per-server peer counts (active + total). Peers live under users, each
    // tagged with its interface id — so a peer belongs to the server that owns
    // its interface. "active" = not deactivated (models.Peer.disabled). (This
    // column previously rendered server.interfaces.length, i.e. the interface
    // count, so a server with one interface and no peers showed "1".)
    const peerCountByServer = useMemo(() => {
        const ifaceToServer = new Map<string, string>()
        for (const s of servers) for (const ifaceId of s.interfaces ?? []) ifaceToServer.set(ifaceId, s.id)
        const counts: Record<string, { active: number; total: number }> = {}
        for (const u of users) for (const p of u.peers ?? []) {
            const sid = ifaceToServer.get(p.interface)
            if (!sid) continue
            const c = counts[sid] ?? (counts[sid] = { active: 0, total: 0 })
            c.total++
            if (!p.disabled) c.active++
        }
        return counts
    }, [servers, users])

    // Currently-connected peers, per server and in total. Derived from each
    // server's latest /metrics snapshot: a peer counts as connected when its
    // last handshake is recent (isPeerConnected). Tunnel gateway peers are NOT
    // counted — only keys that belong to a real user peer are (a gateway peer,
    // added by BuildTunnel, is not a user peer), which is exactly "don't count
    // tunnels". A server whose metrics haven't loaded yet contributes 0.
    const connectedPeers = useMemo(() => {
        // Hex public keys of every user peer, to tell client peers (counted)
        // from tunnel gateway peers (skipped). The agent keys metrics by hex.
        const userHex = new Set<string>()
        for (const u of users) for (const p of u.peers ?? []) {
            const hex = b64ToHex(p.pk)
            if (hex) userHex.add(hex)
        }
        const byServer: Record<string, number> = {}
        let total = 0
        let loaded = 0
        for (const s of servers) {
            const snap = metricsByServer[s.id]
            if (!snap) continue
            loaded++
            // Judge "recent" against the AGENT's own clock, not the browser's:
            // both the handshake and this sample's timestamp come from the agent
            // host, so wall-clock skew between it and the admin cancels. The
            // sample lags real time by ≤ one collection interval (~45s), which
            // the 3-min window absorbs. Falls back to browser now if absent.
            const agentNow = snap.system?.timestamp
                ? new Date(snap.system.timestamp as unknown as string).getTime()
                : Date.now()
            let n = 0
            for (const iface of snap.interfaces ?? []) {
                for (const peer of iface.peers ?? []) {
                    if (userHex.has(peer.publicKey) && isPeerConnected(peer.lastHandshake, agentNow)) n++
                }
            }
            byServer[s.id] = n
            total += n
        }
        // partial = some servers reported but not all: `total` is then only a
        // lower bound (unreachable servers contribute 0), so the aggregate is
        // shown approximate rather than as a confident, complete number.
        return { byServer, total, loaded, partial: loaded > 0 && loaded < servers.length }
    }, [servers, users, metricsByServer])

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
                // is still signalled separately by the agent-status badge.
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

    // Agent status applies to every server (mTLS included): it probes the agent
    // itself and combines that with transport liveness into a tri-state badge.
    // Best-effort like metrics — a failed tick yields null ('unknown') rather
    // than throwing.
    useEffect(() => {
        let cancelled = false
        Promise.all(servers.map(async (server) => [server.id, await getServerAgentStatus(server.id)] as const))
            .then((entries) => {
                if (!cancelled) setAgentStatusByServer(Object.fromEntries(entries))
            })
        return () => {
            cancelled = true
        }
    }, [servers])

    // Host info (backend, Docker, interface kinds) is gathered by the agent once
    // at startup, so it's effectively static — merge rather than replace so a
    // transient unreachable tick keeps the last known value instead of blanking
    // the cell (same treatment as metrics; "—" only until the first success).
    useEffect(() => {
        let cancelled = false
        Promise.all(servers.map(async (server) => [server.id, await getServerHostInfo(server.id)] as const))
            .then((entries) => {
                if (cancelled) return
                setHostInfoByServer((prev) => {
                    const next: Record<string, HostInfo | null> = { ...prev }
                    for (const [id, info] of entries) {
                        if (info !== null) next[id] = info
                        else if (!(id in next)) next[id] = null
                    }
                    return next
                })
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
                        label={t('dashboard.totalTunnels')}
                        value={stats.totalTunnels}
                        icon={Waypoints}
                        accent="amber"
                    />
                    <StatCard
                        label={t('dashboard.totalUsers')}
                        value={stats.totalUsers}
                        icon={Users}
                        accent="violet"
                    />
                    <StatCard
                        label={t('dashboard.totalPeers')}
                        value={stats.totalPeers}
                        sub={`${stats.activePeers} ${t('dashboard.activePeers').toLowerCase()} · ${connectedPeers.loaded === 0 ? '—' : (connectedPeers.partial ? '~' : '') + connectedPeers.total} ${t('dashboard.onlinePeers').toLowerCase()}`}
                        icon={Network}
                        accent="emerald"
                    />
                </div>

                <div>
                    {/* Servers table */}
                    <div className="rounded-xl border border-border bg-card dark:border-white/5 dark:bg-white/3 overflow-x-auto">
                        <div className="border-b border-white/5 px-5 py-4">
                            <h2 className="text-sm font-semibold text-zinc-500">{t('nav.servers')}</h2>
                        </div>
                        <table className="w-full text-sm whitespace-nowrap">
                            <thead>
                                <tr className="border-b border-white/5">
                                    <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.name')}
                                    </th>
                                    <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.host')}
                                    </th>
                                    <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.agentStatus')}
                                    </th>
                                    <th className="px-5 py-3 text-left text-xs font-medium uppercase tracking-wider text-zinc-600">
                                        {t('servers.agentType')}
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
                                                <StatusBadge
                                                    status={agentStatusByServer[server.id] ?? 'unknown'}
                                                    size="sm"
                                                />
                                            </td>
                                            <td className="px-5 py-3">
                                                <HostInfoBadges info={hostInfoByServer[server.id]} />
                                            </td>
                                            <td className="px-5 py-3 text-right font-mono dark:text-zinc-400">
                                                <span className="inline-flex items-center justify-end gap-2">
                                                    {/* Connected now (green, live dot) — shown once this
                                                        server's metrics have loaded; excludes tunnel peers. */}
                                                    {server.id in connectedPeers.byServer && (
                                                        <span
                                                            className="inline-flex items-center gap-1 text-emerald-600 dark:text-emerald-400"
                                                            title={t('dashboard.connectedPeersTooltip', {count: connectedPeers.byServer[server.id]})}
                                                        >
                                                            <span className="h-1.5 w-1.5 rounded-full bg-emerald-400 animate-pulse" />
                                                            {connectedPeers.byServer[server.id]}
                                                        </span>
                                                    )}
                                                    {/* Provisioned: active, with total muted only when some
                                                        peers are deactivated (active < total). */}
                                                    {(() => {
                                                        const c = peerCountByServer[server.id]
                                                        if (!c) return <span>0</span>
                                                        return c.active < c.total
                                                            ? <span title={t('dashboard.activePeersOfTotal', {active: c.active, total: c.total})}>
                                                                  {c.active}
                                                                  <span className="text-zinc-600"> / {c.total}</span>
                                                              </span>
                                                            : <span>{c.total}</span>
                                                    })()}
                                                </span>
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
