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

import {useEffect, useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    CategoryScale,
    Chart as ChartJS,
    Legend,
    LinearScale,
    LineElement,
    PointElement,
    Tooltip,
    type ChartOptions,
} from 'chart.js';
import {Line} from 'react-chartjs-2';
import {RefreshCw} from 'lucide-react';
import {Modal} from '@/components/common/Modal';
import {getServerMetricsHistory} from '@/services/servers';
import {useAppStore} from '@/store';
import {cn, formatBytes} from '@/lib/utils';
import {b64ToHex, isPeerConnected} from '@/lib/peers';
import type {PeerHistoryPoint, SystemHistory, SystemHistoryPoint} from '@/types';

ChartJS.register(CategoryScale, LinearScale, PointElement, LineElement, Tooltip, Legend);

interface Props {
    serverId: string;
    serverName: string;
    onClose: () => void;
}

// SystemHistoryPoint.timestamp is a Go time.Time, which encoding/json
// marshals as an RFC3339 string — tygo's generated TS type labels it
// `number` (it doesn't special-case time.Time), but the actual wire value
// is a string; new Date() parses either correctly so this is safe either
// way.
const timeLabel = (point: SystemHistoryPoint) =>
    new Date(point.timestamp as unknown as string).toLocaleTimeString([], {hour: '2-digit', minute: '2-digit'});

// tsMs parses a wire timestamp (RFC3339 string, typed as number by tygo — see
// the timeLabel note) to epoch milliseconds for range filtering/comparison.
const tsMs = (ts: unknown) => new Date(ts as string).getTime();

// Y-axis bounds are dynamic: max is the largest value actually present in
// the series rounded up to the next multiple of ten, min is the smallest
// value rounded down to the previous multiple of ten — not a fixed 0/100,
// so a chart that never leaves e.g. 30-40% isn't squashed against the
// bottom of the plot.
const roundUpToTen = (n: number) => Math.ceil(n / 10) * 10;
const roundDownToTen = (n: number) => Math.floor(n / 10) * 10;

function axisBounds(...series: number[][]): {min: number; max: number} {
    const flat = series.flat();
    if (flat.length === 0) return {min: 0, max: 10};

    const max = roundUpToTen(Math.max(...flat));
    let min = roundDownToTen(Math.min(...flat));
    if (min >= max) min = max - 10; // flat series (every value the same)
    return {min, max};
}

// Chart height is fixed (not maintainAspectRatio) so three stacked charts
// plus headings/legend/modal chrome still fit a 1200x800 desktop window
// without scrolling.
const CHART_HEIGHT = 130;

const baseOptions: ChartOptions<'line'> = {
    responsive: true,
    maintainAspectRatio: false,
    animation: false,
    interaction: {mode: 'index', intersect: false},
    elements: {point: {radius: 0}},
    plugins: {
        legend: {labels: {boxWidth: 12, font: {size: 11}}},
    },
    scales: {
        x: {ticks: {maxTicksLimit: 6, font: {size: 10}}},
        y: {ticks: {font: {size: 10}}},
    },
};

// activitySeries turns a peer's cumulative rx+tx counters into per-interval
// traffic (bytes moved between consecutive samples) — the actual "activity",
// since the raw counters only ever climb. The first sample has no predecessor
// to diff against, so the series starts one point in; a counter that went
// backwards (interface recreated) is clamped to 0.
function activitySeries(points: PeerHistoryPoint[]): number[] {
    const out: number[] = [];
    for (let i = 1; i < points.length; i++) {
        const delta = (points[i].rx + points[i].tx) - (points[i - 1].rx + points[i - 1].tx);
        out.push(delta > 0 ? delta : 0);
    }
    return out;
}

const shortKey = (pk: string) => (pk.length > 12 ? `${pk.slice(0, 10)}…` : pk);

// Sparkline is a small inline area chart that fills the width of its container
// — deliberately short (not a full Chart.js instance per row, which would be
// heavy for many peers) — for showing a peer's activity trend beside its
// identifier. The SVG stretches horizontally to whatever room the row gives it
// (preserveAspectRatio="none") while the stroke stays crisp (non-scaling).
function Sparkline({values, height = 28}: {values: number[]; height?: number}) {
    if (values.length === 0) {
        return <div className="w-full" style={{height}} aria-hidden="true"/>;
    }

    const vbWidth = 1000; // coordinate space; the SVG scales to the container's real width
    const max = Math.max(...values, 1);
    const n = values.length;
    const pad = 2;
    const h = height - pad * 2;
    const xy = values.map((v, i) => {
        const x = n > 1 ? (i / (n - 1)) * vbWidth : vbWidth / 2;
        const y = pad + h - (v / max) * h;
        return `${x.toFixed(1)},${y.toFixed(1)}`;
    });

    return (
        <svg
            width="100%"
            height={height}
            viewBox={`0 0 ${vbWidth} ${height}`}
            preserveAspectRatio="none"
            className="block"
        >
            <polygon points={`0,${height} ${xy.join(' ')} ${vbWidth},${height}`} fill="rgba(56,189,248,0.15)"/>
            <polyline
                points={xy.join(' ')}
                fill="none"
                stroke="rgb(56,189,248)"
                strokeWidth={1.5}
                strokeLinejoin="round"
                vectorEffect="non-scaling-stroke"
            />
        </svg>
    );
}

type MetricsTab = 'system' | 'peers';

type RangeKey = '1h' | '3h' | '6h' | '24h' | 'all';

// Selectable display windows for the CPU/RAM/Network/peers charts. `ms: null`
// means "all retained history" (up to the agent's 48h retention). The window is
// anchored to the newest retained sample, so it's a relative "last N" view.
const RANGES: {key: RangeKey; ms: number | null}[] = [
    {key: '1h', ms: 60 * 60 * 1000},
    {key: '3h', ms: 3 * 60 * 60 * 1000},
    {key: '6h', ms: 6 * 60 * 60 * 1000},
    {key: '24h', ms: 24 * 60 * 60 * 1000},
    {key: 'all', ms: null},
];

export function ServerMetricsModal({serverId, serverName, onClose}: Props) {
    const {t} = useTranslation();
    const users = useAppStore((s) => s.users);
    const servers = useAppStore((s) => s.servers);
    const interfacesById = useAppStore((s) => s.interfaces);
    const [history, setHistory] = useState<SystemHistory | null>(null);
    const [loading, setLoading] = useState(true);
    const [tab, setTab] = useState<MetricsTab>('system');
    const [range, setRange] = useState<RangeKey>('all');

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        getServerMetricsHistory(serverId).then((hist) => {
            if (!cancelled) {
                setHistory(hist ?? null);
                setLoading(false);
            }
        });
        return () => {
            cancelled = true;
        };
    }, [serverId]);

    // Restrict every series (system points + each peer's points) to the
    // selected window. The cutoff is relative to the newest retained sample —
    // the agent's own clock — so it lines up with how the online dot is judged.
    // Peers with no sample left in the window drop out of the peers tab.
    const filtered = useMemo(() => {
        if (!history) return null;
        const rangeMs = RANGES.find((r) => r.key === range)?.ms ?? null;
        if (rangeMs == null) return history;

        let latest = 0;
        for (const p of history.points) latest = Math.max(latest, tsMs(p.timestamp));
        for (const iface of history.interfaces)
            for (const pr of iface.peers)
                for (const pt of pr.points) latest = Math.max(latest, tsMs(pt.timestamp));
        if (!Number.isFinite(latest) || latest <= 0) return history;
        const cutoff = latest - rangeMs;

        return {
            points: history.points.filter((p) => tsMs(p.timestamp) >= cutoff),
            interfaces: history.interfaces
                .map((iface) => ({
                    ...iface,
                    peers: iface.peers
                        .map((pr) => ({...pr, points: pr.points.filter((pt) => tsMs(pt.timestamp) >= cutoff)}))
                        .filter((pr) => pr.points.length > 0),
                }))
                .filter((iface) => iface.peers.length > 0),
        };
    }, [history, range]);

    const points = filtered?.points ?? null;

    // Resolve a peer's public key to "<user>/<peer>" using the admin's own
    // user→peer records (the agent only knows public keys). Unknown keys —
    // e.g. a peer added directly on the box — fall back to a short key.
    const peerLabels = useMemo(() => {
        const m = new Map<string, string>();
        for (const u of users) {
            for (const p of u.peers ?? []) {
                const hex = b64ToHex(p.pk);
                if (hex) m.set(hex, `${u.name}/${p.name}`);
            }
        }
        return m;
    }, [users]);

    // For each of THIS server's tunnel-member interfaces (by name), the tunnel's
    // two member servers as "<entry-server>/<exit-server>". An unnamed peer on
    // such an interface is the tunnel's gateway peer (the other member — not a
    // user peer), so it's labelled with the tunnel it belongs to rather than a
    // bare short key. Entry (has a listen port) is put first for a stable order.
    const tunnelLabelByIface = useMemo(() => {
        const byTunnel = new Map<string, {name: string; entry: boolean}[]>();
        for (const s of servers) {
            for (const id of s.interfaces ?? []) {
                const iface = interfacesById.get(id);
                if (!iface?.tunnel) continue;
                const arr = byTunnel.get(iface.tunnel) ?? [];
                arr.push({name: s.name, entry: !!iface.listen});
                byTunnel.set(iface.tunnel, arr);
            }
        }
        const map = new Map<string, string>();
        const server = servers.find((s) => s.id === serverId);
        for (const id of server?.interfaces ?? []) {
            const iface = interfacesById.get(id);
            if (!iface?.tunnel) continue;
            const members = (byTunnel.get(iface.tunnel) ?? [])
                .slice()
                .sort((a, b) => (a.entry === b.entry ? 0 : a.entry ? -1 : 1))
                .map((m) => m.name);
            map.set(iface.iface, members.join('/'));
        }
        return map;
    }, [servers, interfacesById, serverId]);

    const peerRows = useMemo(() => {
        const rows = (filtered?.interfaces ?? []).flatMap((iface) =>
            iface.peers.map((peer) => {
                const activity = activitySeries(peer.points);
                const known = peerLabels.get(peer.publicKey);
                const tunnel = tunnelLabelByIface.get(iface.interface);
                const label = known
                    ?? (tunnel
                        ? `${t('servers.metricsTunnelPeer')} ${tunnel}`
                        : shortKey(peer.publicKey));
                // "Online now" is judged from the peer's most recent retained
                // sample: its lastHandshake against that sample's own timestamp
                // (the agent's clock), so host clock skew cancels — same rule as
                // the Dashboard's connected count.
                const last = peer.points[peer.points.length - 1];
                const online = last
                    ? isPeerConnected(last.lastHandshake, new Date(last.timestamp as unknown as string).getTime())
                    : false;
                return {
                    publicKey: peer.publicKey,
                    label,
                    online,
                    activity,
                    total: activity.reduce((sum, v) => sum + v, 0),
                };
            }),
        );
        // Busiest peers first, like Uptrace's GROUPS ordered by count.
        return rows.sort((a, b) => b.total - a.total);
    }, [filtered, peerLabels, tunnelLabelByIface, t]);

    const labels = points?.map(timeLabel) ?? [];

    const cpuData = {
        labels,
        datasets: [
            {
                label: t('servers.metricsCpu'),
                data: points?.map((p) => p.cpuPercent) ?? [],
                borderColor: 'rgb(56, 189, 248)',
                backgroundColor: 'rgba(56, 189, 248, 0.15)',
                fill: true,
                tension: 0.2,
            },
        ],
    };

    const memData = {
        labels,
        datasets: [
            {
                label: t('servers.metricsMemUsed'),
                data: points?.map((p) => p.memUsedBytes) ?? [],
                borderColor: 'rgb(52, 211, 153)',
                backgroundColor: 'rgba(52, 211, 153, 0.15)',
                fill: true,
                tension: 0.2,
            },
            {
                label: t('servers.metricsMemTotal'),
                data: points?.map((p) => p.memTotalBytes) ?? [],
                borderColor: 'rgb(161, 161, 170)',
                borderDash: [4, 4],
                pointRadius: 0,
                tension: 0.2,
            },
        ],
    };

    const netData = {
        labels,
        datasets: [
            {
                label: t('servers.metricsNetRx'),
                data: points?.map((p) => p.netRxBytes) ?? [],
                borderColor: 'rgb(168, 85, 247)',
                backgroundColor: 'rgba(168, 85, 247, 0.15)',
                fill: true,
                tension: 0.2,
            },
            {
                label: t('servers.metricsNetTx'),
                data: points?.map((p) => p.netTxBytes) ?? [],
                borderColor: 'rgb(251, 146, 60)',
                backgroundColor: 'rgba(251, 146, 60, 0.15)',
                fill: true,
                tension: 0.2,
            },
        ],
    };

    const cpuBounds = axisBounds(points?.map((p) => p.cpuPercent) ?? []);
    const cpuOptions: ChartOptions<'line'> = {
        ...baseOptions,
        scales: {...baseOptions.scales, y: {...cpuBounds, ticks: {font: {size: 10}, callback: (v) => `${v}%`}}},
    };

    const memBounds = axisBounds(points?.map((p) => p.memUsedBytes) ?? [], points?.map((p) => p.memTotalBytes) ?? []);
    const memOptions: ChartOptions<'line'> = {
        ...baseOptions,
        scales: {...baseOptions.scales, y: {...memBounds, ticks: {font: {size: 10}, callback: (v) => formatBytes(Number(v))}}},
    };

    const netBounds = axisBounds(points?.map((p) => p.netRxBytes) ?? [], points?.map((p) => p.netTxBytes) ?? []);
    const netOptions: ChartOptions<'line'> = {
        ...baseOptions,
        scales: {...baseOptions.scales, y: {...netBounds, ticks: {font: {size: 10}, callback: (v) => formatBytes(Number(v))}}},
    };

    const tabs: MetricsTab[] = ['system', 'peers'];

    return (
        <Modal title={`${t('servers.metricsTitle')} — ${serverName}`} onClose={onClose} size="lg">
            {loading ? (
                <div className="flex items-center justify-center py-16 text-muted-foreground dark:text-zinc-400">
                    <RefreshCw className="w-5 h-5 mr-2 animate-spin"/>
                    {t('common.loading')}
                </div>
            ) : (
                <div>
                    <div className="mb-3 flex items-end justify-between gap-2 border-b border-border dark:border-white/10">
                        <div className="flex gap-1">
                            {tabs.map((tk) => (
                                <button
                                    key={tk}
                                    type="button"
                                    onClick={() => setTab(tk)}
                                    className={cn(
                                        '-mb-px border-b-2 px-3 py-2 text-sm font-medium transition-colors',
                                        tab === tk
                                            ? 'border-sky-500 text-foreground dark:text-zinc-100'
                                            : 'border-transparent text-muted-foreground hover:text-foreground dark:text-zinc-500 dark:hover:text-zinc-300',
                                    )}
                                >
                                    {t(tk === 'system' ? 'servers.metricsTabSystem' : 'servers.metricsTabPeers')}
                                </button>
                            ))}
                        </div>
                        <div className="mb-1 flex items-center gap-1" role="group" aria-label={t('servers.metricsRange')}>
                            {RANGES.map((r) => (
                                <button
                                    key={r.key}
                                    type="button"
                                    onClick={() => setRange(r.key)}
                                    className={cn(
                                        'rounded px-2 py-1 text-xs font-medium transition-colors',
                                        range === r.key
                                            ? 'bg-sky-500 text-white'
                                            : 'text-muted-foreground hover:bg-black/5 hover:text-foreground dark:text-zinc-400 dark:hover:bg-white/10 dark:hover:text-zinc-200',
                                    )}
                                >
                                    {t(`servers.metricsRange_${r.key}`)}
                                </button>
                            ))}
                        </div>
                    </div>

                    {tab === 'system' ? (
                        !points || points.length === 0 ? (
                            <div className="py-16 text-center text-sm text-zinc-600">{t('servers.metricsNoData')}</div>
                        ) : (
                            <div className="space-y-3">
                                <div>
                                    <h4 className="text-xs font-medium text-muted-foreground dark:text-zinc-400 mb-1">
                                        {t('servers.metricsCpu')}
                                    </h4>
                                    <div style={{height: CHART_HEIGHT}}>
                                        <Line data={cpuData} options={cpuOptions}/>
                                    </div>
                                </div>
                                <div>
                                    <h4 className="text-xs font-medium text-muted-foreground dark:text-zinc-400 mb-1">
                                        {t('servers.metricsMemory')}
                                    </h4>
                                    <div style={{height: CHART_HEIGHT}}>
                                        <Line data={memData} options={memOptions}/>
                                    </div>
                                </div>
                                <div>
                                    <h4 className="text-xs font-medium text-muted-foreground dark:text-zinc-400 mb-1">
                                        {t('servers.metricsNetwork')}
                                    </h4>
                                    <div style={{height: CHART_HEIGHT}}>
                                        <Line data={netData} options={netOptions}/>
                                    </div>
                                </div>
                            </div>
                        )
                    ) : peerRows.length === 0 ? (
                        <div className="py-16 text-center text-sm text-zinc-600">{t('servers.metricsPeersNoData')}</div>
                    ) : (
                        <table className="w-full text-sm">
                            <thead>
                                <tr className="border-b border-border dark:border-white/10 text-xs uppercase tracking-wider text-zinc-500">
                                    <th className="whitespace-nowrap py-2 pr-3 text-left font-medium">{t('servers.metricsPeerColumn')}</th>
                                    <th className="w-full py-2 pl-3 text-left font-medium">{t('servers.metricsActivity')}</th>
                                </tr>
                            </thead>
                            <tbody>
                                {peerRows.map((row) => (
                                    <tr key={row.publicKey} className="border-b border-white/5 last:border-0">
                                        <td className="whitespace-nowrap py-2 pr-3 align-middle">
                                            <span className="flex items-center gap-2">
                                                <span
                                                    className={cn(
                                                        'h-2 w-2 shrink-0 rounded-full',
                                                        row.online ? 'bg-emerald-500' : 'bg-zinc-300 dark:bg-zinc-600',
                                                    )}
                                                    title={t(row.online ? 'servers.peerOnline' : 'servers.peerOffline')}
                                                    aria-label={t(row.online ? 'servers.peerOnline' : 'servers.peerOffline')}
                                                />
                                                <span className="font-mono text-xs dark:text-zinc-300" title={row.publicKey}>
                                                    {row.label}
                                                </span>
                                            </span>
                                        </td>
                                        <td className="w-full py-2 pl-3 align-middle">
                                            <div className="flex items-center gap-3">
                                                <span className="shrink-0 text-xs tabular-nums text-muted-foreground dark:text-zinc-500">
                                                    {formatBytes(row.total)}
                                                </span>
                                                <div className="min-w-0 flex-1">
                                                    <Sparkline values={row.activity}/>
                                                </div>
                                            </div>
                                        </td>
                                    </tr>
                                ))}
                            </tbody>
                        </table>
                    )}
                </div>
            )}
        </Modal>
    );
}
