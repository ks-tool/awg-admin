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

import {Handle, Position, type Node, type NodeProps} from '@xyflow/react';
import {Network, Server} from 'lucide-react';
import type {NetworkInterface, TopologyServer, VPNService} from '@/types';

export type ServerFlowNode = Node<{
    server: TopologyServer;
    services: VPNService[];
    onSelectService: (id: string) => void;
}, 'server'>;

export type InterfaceFlowNode = Node<{
    iface: NetworkInterface;
    service?: VPNService;
}, 'interface'>;

export function TopologyServerNode({data, selected}: NodeProps<ServerFlowNode>) {
    return (
        <div className={`w-64 rounded-xl border bg-card p-4 text-card-foreground shadow-lg transition-colors ${selected ? 'border-sky-500 ring-2 ring-sky-500/20' : 'border-border'}`}>
            <div className="flex items-center gap-3">
                <div className="rounded-lg bg-sky-500/15 p-2 text-sky-600 dark:text-sky-400"><Server size={20} /></div>
                <div className="min-w-0">
                    <div className="truncate font-semibold">{data.server.name}</div>
                    <div className="truncate font-mono text-xs text-muted-foreground">{data.server.address || '—'}</div>
                </div>
            </div>
            {data.services.length > 0 && (
                <div className="mt-3 space-y-1 border-t border-border pt-2">
                    {data.services.map(service => (
                        <button
                            key={service.id}
                            type="button"
                            onClick={event => { event.stopPropagation(); data.onSelectService(service.id); }}
                            className="nodrag flex w-full items-center justify-between rounded-md px-2 py-1 text-left text-xs hover:bg-muted"
                        >
                            <span className="truncate">{service.name}</span>
                            <span className={`ml-2 h-2 w-2 shrink-0 rounded-full ${service.status === 'running' ? 'bg-emerald-500' : service.status === 'error' ? 'bg-red-500' : 'bg-amber-500'}`} />
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}

export function TopologyInterfaceNode({data, selected}: NodeProps<InterfaceFlowNode>) {
    return (
        <div className={`relative w-56 rounded-xl border bg-card p-3 text-card-foreground shadow-lg transition-colors ${selected ? 'border-sky-500 ring-2 ring-sky-500/20' : 'border-border'}`}>
            <Handle type="target" position={Position.Left} className="!h-3 !w-3 !border-2 !border-background !bg-sky-500" />
            <div className="flex items-center gap-3">
                <div className="rounded-lg bg-violet-500/15 p-2 text-violet-600 dark:text-violet-400"><Network size={18} /></div>
                <div className="min-w-0">
                    <div className="truncate font-mono text-sm font-semibold">{data.iface.name}</div>
                    <div className="truncate font-mono text-xs text-muted-foreground">{data.iface.address || '—'}</div>
                </div>
            </div>
            {data.service && <div className="mt-2 truncate text-xs text-sky-600 dark:text-sky-400">{data.service.name}</div>}
            <Handle type="source" position={Position.Right} className="!h-3 !w-3 !border-2 !border-background !bg-sky-500" />
        </div>
    );
}
