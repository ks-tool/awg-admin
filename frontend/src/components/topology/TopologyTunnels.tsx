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

import {useId, useMemo} from 'react';
import {projectMercator} from '@zenith-visuals/maps';
import type {NetworkConnection, NetworkInterface, ProviderLink, TopologyServer} from '@/types';
import {mergeProviderLinks} from './connections';

interface TopologyTunnelsProps {
    servers: TopologyServer[];
    interfaces: NetworkInterface[];
    connections: NetworkConnection[];
    links: ProviderLink[];
    width: number;
    height: number;
    transform: string;
    zoom: number;
}

export function TopologyTunnels({servers, interfaces, connections, links, width, height, transform, zoom}: TopologyTunnelsProps) {
    const markerID = useId().replaceAll(':', '');
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
            return [{
                id: connection.id,
                d: `M${start.x},${start.y} Q${control.x},${control.y} ${end.x},${end.y}`,
                label: `${source.name}/${sourceInterface.name} → ${target.name}/${targetInterface.name}`,
            }];
        });
    }, [allConnections, height, interfaces, servers, width, zoom]);

    return (
        <svg aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-visible" width={width} height={height} style={{transform, transformOrigin: '0 0'}}>
            <defs>
                <marker id={markerID} markerWidth={9 / zoom} markerHeight={9 / zoom} viewBox="0 0 6 6" refX={5} refY={3} orient="auto" markerUnits="userSpaceOnUse">
                    <path d="M0,0 L6,3 L0,6 Z" fill="#0ea5e9" />
                </marker>
            </defs>
            {paths.map(path => (
                <path key={path.id} d={path.d} fill="none" stroke="#0ea5e9" strokeWidth={1.5 / zoom} strokeOpacity={0.8} markerEnd={`url(#${markerID})`}>
                    <title>{path.label}</title>
                </path>
            ))}
        </svg>
    );
}
