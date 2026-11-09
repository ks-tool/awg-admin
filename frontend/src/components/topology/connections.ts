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

import type {NetworkConnection, Tunnel} from '@/types';

export function mergeTopologyConnections(connections: NetworkConnection[], tunnels: Tunnel[]): NetworkConnection[] {
    const tunnelConnections = tunnels.flatMap(tunnel => tunnel.members.slice(1).map((member, index) => ({
        id: `tunnel:${tunnel.id}:${index}`,
        sourceId: tunnel.members[index].ifaceId,
        targetId: member.ifaceId,
    })));
    const seen = new Set<string>();
    return [...connections, ...tunnelConnections].filter(connection => {
        const key = `${connection.sourceId}:${connection.targetId}`;
        if (seen.has(key)) return false;
        seen.add(key);
        return true;
    });
}

export function canCreateTopologyConnection(connections: NetworkConnection[], source: string, target: string): boolean {
    if (source === target || connections.some(connection => connection.sourceId === source && connection.targetId === target)) return false;
    const next = new Map<string, string[]>();
    for (const connection of connections) {
        const targets = next.get(connection.sourceId) ?? [];
        targets.push(connection.targetId);
        next.set(connection.sourceId, targets);
    }
    const visited = new Set<string>();
    const pending = [target];
    while (pending.length > 0) {
        const current = pending.pop()!;
        if (current === source) return false;
        if (visited.has(current)) continue;
        visited.add(current);
        pending.push(...(next.get(current) ?? []));
    }
    return true;
}
