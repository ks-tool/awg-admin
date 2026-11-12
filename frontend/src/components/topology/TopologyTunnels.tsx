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

import {useId, useMemo, useState, type PointerEvent as ReactPointerEvent} from 'react';
import {useTranslation} from 'react-i18next';
import {projectMercator} from '@zenith-visuals/maps';
import type {NetworkConnection, NetworkInterface, ProviderLink, TopologyServer, TunnelMetric} from '@/types';
import {mergeProviderLinks} from './connections';

const latencyColors = {blue: '#0ea5e9', yellow: '#f59e0b', red: '#ef4444'};

function latencyColor(p95?: number): string {
    if (p95 == null || p95 <= 100) return latencyColors.blue;
    if (p95 <= 250) return latencyColors.yellow;
    return latencyColors.red;
}

interface TopologyTunnelsProps {
    servers: TopologyServer[];
    interfaces: NetworkInterface[];
    connections: NetworkConnection[];
    links: ProviderLink[];
    metrics: Record<string, TunnelMetric>;
    width: number;
    height: number;
    transform: string;
    zoom: number;
}

export function TopologyTunnels({servers, interfaces, connections, links, metrics, width, height, transform, zoom}: TopologyTunnelsProps) {
    const {t} = useTranslation();
    const markerID = useId().replaceAll(':', '');
    const [hovered, setHovered] = useState<{id: string; x: number; y: number} | null>(null);
    const allConnections = useMemo(() => mergeProviderLinks(connections, links), [connections, links]);
    const paths = useMemo(() => {
        if (width === 0) return [];
        const interfaceByID = new Map(interfaces.map(iface => [iface.id, iface]));
        const serverByID = new Map(servers.map(server => [server.id, server]));
        return allConnections.flatMap(connection => {
            const sourceInterface = interfaceByID.get(connection.sourceId);
            const targetInterface = interfaceByID.get(connection.targetId);
            const source = sourceInterface && serverByID.get(sourceInterface.serverId);
            const target = targetInterface && serverByID.get(targetInterface.serverId);
            if (!sourceInterface || !targetInterface || !source || !target || source.id === target.id
                || source.latitude == null || source.longitude == null || target.latitude == null || target.longitude == null) {
                return [];
            }
            const sourcePoint = projectMercator({lat: source.latitude, lon: source.longitude}, width, height);
            const targetPoint = projectMercator({lat: target.latitude, lon: target.longitude}, width, height);
            const dx = targetPoint.x - sourcePoint.x;
            const dy = targetPoint.y - sourcePoint.y;
            const distance = Math.hypot(dx, dy);
            if (distance < 1) return [];
            const padding = Math.min(12, distance / 4) / zoom;
            const start = {x: sourcePoint.x + dx / distance * padding, y: sourcePoint.y + dy / distance * padding};
            const end = {x: targetPoint.x - dx / distance * padding, y: targetPoint.y - dy / distance * padding};
            const curvature = Math.min(70, distance * 0.12);
            const control = {
                x: (start.x + end.x) / 2 - dy / distance * curvature,
                y: (start.y + end.y) / 2 + dx / distance * curvature,
            };
            const metric = metrics[connection.id];
            const p95 = metric?.latencyP95Ms;
            const tooltip = [`${source.name}/${sourceInterface.name} → ${target.name}/${targetInterface.name}`];
            if (p95 == null) {
                tooltip.push(metric?.error ? `${t('topology.tunnelMeasurementError')}: ${metric.error}` : t('topology.tunnelNoMetrics'));
            } else {
                tooltip.push(`p95 ${p95.toFixed(1)} ms`);
                if (p95 > 250) tooltip.push(t('topology.tunnelHighLatency'));
            }
            return [{
                id: connection.id,
                d: `M${start.x},${start.y} Q${control.x},${control.y} ${end.x},${end.y}`,
                color: latencyColor(p95),
                tooltip,
            }];
        });
    }, [allConnections, height, interfaces, metrics, servers, t, width, zoom]);
    const hoveredPath = hovered ? paths.find(path => path.id === hovered.id) : undefined;
    const showTooltip = (event: ReactPointerEvent<SVGGElement>, id: string) => {
        const bounds = event.currentTarget.ownerSVGElement?.parentElement?.getBoundingClientRect();
        if (!bounds) return;
        setHovered({id, x: event.clientX - bounds.left, y: event.clientY - bounds.top});
    };

    return (
        <>
            <svg aria-hidden="true" className="pointer-events-none absolute inset-0 z-10 overflow-visible" width={width} height={height} style={{transform, transformOrigin: '0 0'}}>
                <defs>
                    {Object.entries(latencyColors).map(([name, color]) => (
                        <marker key={name} id={`${markerID}-${name}`} markerWidth={9 / zoom} markerHeight={9 / zoom} viewBox="0 0 6 6" refX={5} refY={3} orient="auto" markerUnits="userSpaceOnUse">
                            <path d="M0,0 L6,3 L0,6 Z" fill={color} />
                        </marker>
                    ))}
                </defs>
                {paths.map(path => {
                    const marker = Object.entries(latencyColors).find(([, color]) => color === path.color)?.[0] ?? 'blue';
                    return (
                        <g
                            key={path.id}
                            onPointerEnter={event => showTooltip(event, path.id)}
                            onPointerMove={event => showTooltip(event, path.id)}
                            onPointerLeave={() => setHovered(current => current?.id === path.id ? null : current)}
                        >
                            <path d={path.d} fill="none" stroke={path.color} strokeWidth={1.5 / zoom} strokeOpacity={0.8} markerEnd={`url(#${markerID}-${marker})`} pointerEvents="stroke" />
                            <path d={path.d} fill="none" stroke="transparent" strokeWidth={12 / zoom} pointerEvents="stroke" />
                        </g>
                    );
                })}
            </svg>
            {hovered && hoveredPath && (
                <div
                    data-testid="tunnel-tooltip"
                    className="pointer-events-none absolute z-20 max-w-80 -translate-x-1/2 translate-y-3 rounded-lg border border-l-2 bg-popover px-3 py-2 text-xs whitespace-pre-line text-popover-foreground shadow-xl"
                    style={{left: hovered.x, top: hovered.y, borderLeftColor: hoveredPath.color}}
                >
                    {hoveredPath.tooltip.map((line, index) => <div key={index} className={index === 0 ? 'font-semibold' : 'mt-1'}>{line}</div>)}
                </div>
            )}
        </>
    );
}
