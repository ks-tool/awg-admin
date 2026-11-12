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
import {Network} from 'lucide-react';
import type {NetworkInterface, TopologyServer, VPNService} from '@/types';

export type InterfaceFlowNode = Node<{
    iface: NetworkInterface;
    server?: TopologyServer;
    service?: VPNService;
}, 'interface'>;

export function TopologyInterfaceNode({data, selected}: NodeProps<InterfaceFlowNode>) {
    return (
        <div className={`relative w-56 rounded-xl border bg-card p-3 text-card-foreground shadow-lg transition-colors ${selected ? 'border-sky-500 ring-2 ring-sky-500/20' : 'border-border'}`}>
            <Handle type="target" position={Position.Left} className="h-3! w-3! border-2! border-background! bg-sky-500!" />
            <div className="flex items-center gap-3">
                <div className="rounded-lg bg-violet-500/15 p-2 text-violet-600 dark:text-violet-400"><Network size={18} /></div>
                <div className="min-w-0">
                    <div className="truncate text-xs font-semibold">
                        {data.server ? `${data.server.name}[${data.server.address || '—'}]` : '—'}
                    </div>
                    <div className="truncate font-mono text-sm font-semibold">{data.iface.name}</div>
                    <div className="truncate font-mono text-xs text-muted-foreground">{data.iface.address || '—'}</div>
                </div>
            </div>
            {data.service && <div className="mt-2 truncate text-xs text-sky-600 dark:text-sky-400">{data.service.name}</div>}
            <Handle type="source" position={Position.Right} className="h-3! w-3! border-2! border-background! bg-sky-500!" />
        </div>
    );
}
