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

import {useEffect, useMemo, useRef, useState, type CSSProperties, type PointerEvent as ReactPointerEvent} from 'react';
import {useTranslation} from 'react-i18next';
import {GeoScatter, type GeoPoint} from '@zenith-visuals/maps';
import {MapPin, MapPinned, RotateCcw, ZoomIn, ZoomOut} from 'lucide-react';
import type {NetworkConnection, NetworkInterface, TopologyServer, Tunnel, VPNService} from '@/types';
import type {AgentStatus} from '@/services/servers';
import {centeredMapPan, clampMapPan, fitMapViewport} from './mapViewport';
import {TopologyTunnels} from './TopologyTunnels';
import {WorldLand} from './WorldLand';

const mapTheme = {
    colors: {
        background: 'transparent',
        surface: 'var(--popover)',
        muted: 'transparent',
        primary: '#0ea5e9',
        text: 'var(--foreground)',
        textMuted: 'var(--muted-foreground)',
        border: 'var(--border)',
    },
    typography: {fontFamily: 'var(--font-sans)'},
};

const serviceStatusColors = ['#ef4444', '#f59e0b', '#22c55e'];

function serviceStatusValue(status: string): number {
    if (status === 'error') return 0;
    if (status === 'running') return 2;
    return 1;
}

function serviceStatusColor(status: string): string {
    return serviceStatusColors[serviceStatusValue(status)];
}

function agentStatusValue(status: AgentStatus): number {
    if (status === 'down') return 0;
    if (status === 'ok') return 2;
    return 1;
}

interface TopologyMapProps {
    servers: TopologyServer[];
    interfaces: NetworkInterface[];
    services: VPNService[];
    connections: NetworkConnection[];
    tunnels: Tunnel[];
    agentStatuses: Record<string, AgentStatus | null>;
    onSelect: (id: string) => void;
}

export function TopologyMap({servers, interfaces, services, connections, tunnels, agentStatuses, onSelect}: TopologyMapProps) {
    const {t} = useTranslation();
    const containerRef = useRef<HTMLDivElement>(null);
    const [size, setSize] = useState({width: 0, height: 560});
    const [zoom, setZoom] = useState(1);
    const [pan, setPan] = useState({x: 0, y: 0});
    const [dragging, setDragging] = useState(false);
    const dragRef = useRef<{pointerId: number; startX: number; startY: number; startPan: {x: number; y: number}; moved: boolean} | null>(null);
    const suppressClickRef = useRef(false);
    useEffect(() => {
        if (!containerRef.current) return;
        const observer = new ResizeObserver(entries => {
            setSize({
                width: Math.round(entries[0].contentRect.width),
                height: Math.max(420, Math.round(entries[0].contentRect.height)),
            });
            setZoom(1);
            setDragging(false);
            dragRef.current = null;
        });
        observer.observe(containerRef.current);
        return () => observer.disconnect();
    }, []);
    useEffect(() => {
        if (!containerRef.current) return;
        const container = containerRef.current;
        const removeNativeTooltips = () => container.querySelectorAll('circle > title').forEach(title => title.remove());
        removeNativeTooltips();
        const observer = new MutationObserver(removeNativeTooltips);
        observer.observe(container, {childList: true, subtree: true});
        return () => observer.disconnect();
    }, []);
    const fit = useMemo(() => fitMapViewport(size.width, size.height), [size]);
    const scale = fit.scale * zoom;
    const currentPan = zoom === 1 ? {x: fit.x, y: fit.y} : pan;
    const transform = `translate(${currentPan.x}px, ${currentPan.y}px) scale(${scale})`;
    const updateZoom = (nextZoom: number) => {
        const boundedZoom = Math.min(6, Math.max(1, nextZoom));
        setPan(centeredMapPan(currentPan, zoom, boundedZoom, fit, size.width, size.height));
        setZoom(boundedZoom);
    };
    const pointerDown = (event: ReactPointerEvent<HTMLDivElement>) => {
        suppressClickRef.current = false;
        if (zoom === 1 || event.button !== 0) return;
        dragRef.current = {pointerId: event.pointerId, startX: event.clientX, startY: event.clientY, startPan: currentPan, moved: false};
    };
    const pointerMove = (event: ReactPointerEvent<HTMLDivElement>) => {
        const drag = dragRef.current;
        if (!drag || drag.pointerId !== event.pointerId) return;
        const dx = event.clientX - drag.startX;
        const dy = event.clientY - drag.startY;
        if (!drag.moved && Math.hypot(dx, dy) < 3) return;
        if (!drag.moved) {
            event.currentTarget.setPointerCapture(event.pointerId);
            setDragging(true);
        }
        drag.moved = true;
        setPan(clampMapPan({x: drag.startPan.x + dx, y: drag.startPan.y + dy}, fit, size.width, size.height, zoom));
    };
    const pointerEnd = (event: ReactPointerEvent<HTMLDivElement>) => {
        const drag = dragRef.current;
        if (!drag || drag.pointerId !== event.pointerId) return;
        suppressClickRef.current = drag.moved;
        if (drag.moved) window.setTimeout(() => {suppressClickRef.current = false;}, 0);
        dragRef.current = null;
        setDragging(false);
        if (event.currentTarget.hasPointerCapture(event.pointerId)) event.currentTarget.releasePointerCapture(event.pointerId);
    };
    const located = useMemo(() => servers.filter(server => server.latitude != null && server.longitude != null), [servers]);
    const unlocated = useMemo(() => servers.filter(server => server.latitude == null || server.longitude == null), [servers]);
    const points = useMemo<GeoPoint[]>(() => located.map(server => {
        const serverServices = services.filter(service => service.serverId === server.id);
        const agentStatus = agentStatuses[server.id];
        const serviceValue = serverServices.length === 0 ? 1 : Math.min(...serverServices.map(service => serviceStatusValue(service.status)));
        return {
            id: server.id,
            label: server.name,
            lat: server.latitude!,
            lon: server.longitude!,
            value: agentStatus ? agentStatusValue(agentStatus) : serviceValue,
        };
    }), [agentStatuses, located, services]);
    const presentStatuses = [...new Set(points.map(point => point.value ?? 1))].sort((a, b) => a - b);
    const pointTheme = {
        ...mapTheme,
        colors: {
            ...mapTheme.colors,
            primary: presentStatuses.length === 1 ? serviceStatusColors[presentStatuses[0]] : mapTheme.colors.primary,
        },
        sequential: presentStatuses.map(status => serviceStatusColors[status]),
    };

    return (
        <div ref={containerRef} className="relative h-full min-h-[420px]">
            <div
                className={`absolute inset-0 touch-none overflow-hidden bg-muted/40 ${zoom > 1 ? dragging ? 'cursor-grabbing' : 'cursor-grab' : ''}`}
                style={{'--topology-map-transform': transform} as CSSProperties}
                onPointerDown={pointerDown}
                onPointerMove={pointerMove}
                onPointerUp={pointerEnd}
                onPointerCancel={() => {dragRef.current = null; setDragging(false); suppressClickRef.current = false;}}
                onClickCapture={event => {
                    if (!suppressClickRef.current) return;
                    event.preventDefault();
                    event.stopPropagation();
                    suppressClickRef.current = false;
                }}
            >
                <WorldLand width={size.width} height={size.height} transform={transform} />
                <TopologyTunnels servers={servers} interfaces={interfaces} connections={connections} tunnels={tunnels} width={size.width} height={size.height} transform={transform} zoom={zoom} />
                <GeoScatter
                    className="topology-scatter"
                    data={points}
                projection="mercator"
                radiusRange={[5.5 / zoom, 5.5 / zoom]}
                showGraticule
                theme={pointTheme}
                width={size.width || undefined}
                height={size.height}
                labels={{ariaLabel: t('topology.mapAria'), empty: t('topology.mapEmpty')}}
                onPointClick={point => {if (point.id) onSelect(point.id);}}
                renderTooltip={point => {
                    const server = servers.find(item => item.id === point.id);
                    const serverServices = services.filter(service => service.serverId === point.id);
                    const agentStatus = point.id ? agentStatuses[point.id] : null;
                    return (
                        <div>
                            <div className="font-semibold">{point.label}</div>
                            {server?.location && <div className="mt-1 text-xs opacity-70">{server.location}</div>}
                            {server?.address && <div className="mt-1 font-mono text-xs opacity-70">{server.address}</div>}
                            <div className="mt-1 text-xs opacity-70">{point.lat.toFixed(4)}, {point.lon.toFixed(4)}</div>
                            {agentStatus && (
                                <div className="mt-2 flex items-center gap-2 border-t border-border pt-2 text-xs">
                                    <span className="h-2 w-2 rounded-full" style={{backgroundColor: serviceStatusColors[agentStatusValue(agentStatus)]}} />
                                    <span>{t(`status.${agentStatus}`)}</span>
                                </div>
                            )}
                            {serverServices.length > 0 && (
                                <div className="mt-2 space-y-1 border-t border-border pt-2">
                                    {serverServices.map(service => (
                                        <div key={service.id} className="flex items-center gap-2 text-xs">
                                            <span className="h-2 w-2 shrink-0 rounded-full" style={{backgroundColor: serviceStatusColor(service.status)}} />
                                            <span>{service.name}</span>
                                            <span className="ml-auto pl-3 opacity-70">{t(`topology.status.${service.status}`, {defaultValue: service.status})}</span>
                                        </div>
                                    ))}
                                </div>
                            )}
                        </div>
                    );
                }}
                renderEmpty={() => (
                    <div className="pointer-events-none flex h-full min-h-[420px] flex-col items-center justify-center text-center">
                        <MapPinned className="mb-3 text-sky-500" size={34} />
                        <h2 className="font-semibold">{t('topology.mapEmpty')}</h2>
                        <p className="mt-2 max-w-sm text-sm text-muted-foreground">{t('topology.mapEmptyHint')}</p>
                    </div>
                )}
            />
            </div>
            <div className="absolute right-4 top-4 z-20 flex items-center gap-1 rounded-lg border border-border bg-card/90 p-1 shadow-sm backdrop-blur">
                <button type="button" aria-label={t('topology.zoomOut')} title={t('topology.zoomOut')} disabled={zoom <= 1} onClick={() => updateZoom(zoom - 0.5)} className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40">
                    <ZoomOut size={16} />
                </button>
                <button type="button" aria-label={t('topology.resetZoom')} title={t('topology.resetZoom')} disabled={zoom <= 1} onClick={() => updateZoom(1)} className="flex min-w-16 items-center justify-center gap-1 rounded-md px-2 py-1.5 text-xs text-muted-foreground hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40">
                    <RotateCcw size={12} />{Math.round(zoom * 100)}%
                </button>
                <button type="button" aria-label={t('topology.zoomIn')} title={t('topology.zoomIn')} disabled={zoom >= 6} onClick={() => updateZoom(zoom + 0.5)} className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground disabled:cursor-not-allowed disabled:opacity-40">
                    <ZoomIn size={16} />
                </button>
            </div>
            <div className="pointer-events-none absolute left-4 top-4 rounded-lg border border-border bg-card/90 px-3 py-2 text-xs text-muted-foreground shadow-sm backdrop-blur">
                {t('topology.locatedServers', {located: located.length, total: servers.length})}
            </div>
            {unlocated.length > 0 && (
                <div className="absolute bottom-4 left-4 max-w-[min(28rem,calc(100%-2rem))] rounded-xl border border-border bg-card/95 p-3 shadow-lg backdrop-blur">
                    <div className="mb-2 flex items-center gap-2 text-xs font-medium text-muted-foreground">
                        <MapPin size={14} />{t('topology.unlocatedServers')}
                    </div>
                    <div className="flex max-h-32 flex-wrap gap-2 overflow-y-auto">
                        {unlocated.map(server => (
                            <div key={server.id}>
                                <button type="button" onClick={() => onSelect(server.id)} className="rounded-md bg-muted px-2.5 py-1.5 text-xs hover:bg-sky-500/15 hover:text-sky-600 dark:hover:text-sky-400">
                                    {server.name}
                                </button>
                                {server.locationError && <div className="mt-1 max-w-52 text-[10px] text-red-500">{server.locationError}</div>}
                            </div>
                        ))}
                    </div>
                </div>
            )}
            <div className="absolute bottom-4 right-4 text-[10px] text-muted-foreground">
                <a href="https://kiankyars.github.io/datacenter-atlas/" target="_blank" rel="noreferrer" className="hover:text-foreground">Data Center Atlas</a>
                {' · '}
                <a href="https://www.naturalearthdata.com/" target="_blank" rel="noreferrer" className="hover:text-foreground">Natural Earth</a>
            </div>
        </div>
    );
}
