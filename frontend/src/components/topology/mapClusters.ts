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
import type {TopologyServer} from '@/types';

export interface ServerCluster {
    key: string;
    servers: TopologyServer[];
    lat: number;
    lon: number;
}

export function clusterTopologyServers(servers: TopologyServer[]): ServerCluster[] {
    const clusters = new Map<string, ServerCluster>();
    for (const server of servers) {
        if (server.latitude == null || server.longitude == null) continue;
        const key = `${server.latitude.toFixed(6)}:${server.longitude.toFixed(6)}`;
        const cluster = clusters.get(key);
        if (cluster) cluster.servers.push(server);
        else clusters.set(key, {key, servers: [server], lat: server.latitude, lon: server.longitude});
    }
    return [...clusters.values()];
}

export function expandedClusterCoordinates(cluster: ServerCluster, width: number, height: number, scale: number): Map<string, {lat: number; lon: number}> {
    const coordinates = new Map<string, {lat: number; lon: number}>();
    if (width <= 0 || height <= 0 || scale <= 0) return coordinates;
    const center = projectMercator(cluster, width, height);
    const radius = Math.max(48, cluster.servers.length * 11) / scale;
    cluster.servers.forEach((server, index) => {
        const angle = index / cluster.servers.length * Math.PI * 2 - Math.PI / 2;
        const x = center.x + radius * Math.cos(angle);
        const y = center.y + radius * Math.sin(angle);
        const mercator = (height / 2 - y) * Math.PI * 2 / width;
        coordinates.set(server.id, {
            lat: (2 * Math.atan(Math.exp(mercator)) - Math.PI / 2) * 180 / Math.PI,
            lon: x / width * 360 - 180,
        });
    });
    return coordinates;
}
