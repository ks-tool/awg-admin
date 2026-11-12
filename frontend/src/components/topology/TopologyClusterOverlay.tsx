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

import {projectMercator} from '@zenith-visuals/maps';
import type {ServerCluster} from './mapClusters';

interface TopologyClusterOverlayProps {
    clusters: ServerCluster[];
    expandedCluster?: ServerCluster;
    expandedCoordinates: Map<string, {lat: number; lon: number}>;
    width: number;
    height: number;
    scale: number;
    transform: string;
}

export function TopologyClusterOverlay({clusters, expandedCluster, expandedCoordinates, width, height, scale, transform}: TopologyClusterOverlayProps) {
    const center = expandedCluster ? projectMercator(expandedCluster, width, height) : null;
    return (
        <svg aria-hidden="true" className="pointer-events-none absolute inset-0 overflow-visible" width={width} height={height} style={{transform, transformOrigin: '0 0'}}>
            {expandedCluster && center && expandedCluster.servers.map((server, index) => {
                const coordinates = expandedCoordinates.get(server.id);
                if (!coordinates) return null;
                const target = projectMercator(coordinates, width, height);
                const angle = index / expandedCluster.servers.length * Math.PI * 2 - Math.PI / 2;
                const offset = 13 / scale;
                return (
                    <g key={server.id}>
                        <line x1={center.x} y1={center.y} x2={target.x} y2={target.y} stroke="#0ea5e9" strokeWidth={1 / scale} strokeOpacity={0.55} />
                        <text
                            x={target.x + offset * Math.cos(angle)}
                            y={target.y + offset * Math.sin(angle) + 3 / scale}
                            textAnchor={Math.cos(angle) >= 0 ? 'start' : 'end'}
                            fontSize={10 / scale}
                            fontWeight={500}
                            fill="var(--foreground)"
                            paintOrder="stroke"
                            stroke="var(--card)"
                            strokeWidth={3 / scale}
                        >
                            {server.name}
                        </text>
                    </g>
                );
            })}
            {clusters.filter(cluster => cluster.servers.length > 1).map(cluster => {
                const point = projectMercator(cluster, width, height);
                return (
                    <text key={cluster.key} x={point.x} y={point.y + 3.5 / scale} textAnchor="middle" fontSize={9 / scale} fontWeight={700} fill="white">
                        {cluster.servers.length}
                    </text>
                );
            })}
        </svg>
    );
}
