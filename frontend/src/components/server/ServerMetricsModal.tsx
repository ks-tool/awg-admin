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

import {useEffect, useState} from 'react';
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
import {formatBytes} from '@/lib/utils';
import type {SystemHistoryPoint} from '@/types';

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

export function ServerMetricsModal({serverId, serverName, onClose}: Props) {
    const {t} = useTranslation();
    const [points, setPoints] = useState<SystemHistoryPoint[] | null>(null);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        getServerMetricsHistory(serverId).then((hist) => {
            if (!cancelled) {
                setPoints(hist?.points ?? null);
                setLoading(false);
            }
        });
        return () => {
            cancelled = true;
        };
    }, [serverId]);

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

    return (
        <Modal title={`${t('servers.metricsTitle')} — ${serverName}`} onClose={onClose} size="lg">
            {loading ? (
                <div className="flex items-center justify-center py-16 text-muted-foreground dark:text-zinc-400">
                    <RefreshCw className="w-5 h-5 mr-2 animate-spin"/>
                    {t('common.loading')}
                </div>
            ) : !points || points.length === 0 ? (
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
            )}
        </Modal>
    );
}
