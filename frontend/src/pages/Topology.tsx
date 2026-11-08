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

import {useCallback, useEffect, useMemo, useState} from 'react';
import {useTranslation} from 'react-i18next';
import {
    Background,
    ConnectionMode,
    Controls,
    MarkerType,
    MiniMap,
    ReactFlow,
    useEdgesState,
    useNodesState,
    type Connection,
    type Edge,
    type Node,
    type NodeTypes,
} from '@xyflow/react';
import '@xyflow/react/dist/style.css';
import {Map as MapIcon, Network, Plus, Share2} from 'lucide-react';
import {toast} from 'sonner';
import {PageHeader} from '@/components/layout/PageHeader';
import {buttons} from '@/components/common/Modal';
import {AddServerModal, AddServiceModal} from '@/components/topology/TopologyModals';
import {TopologyInspector, type TopologySelection} from '@/components/topology/TopologyInspector';
import {TopologyMap} from '@/components/topology/TopologyMap';
import {TopologyInterfaceNode, TopologyServerNode, type InterfaceFlowNode, type ServerFlowNode} from '@/components/topology/TopologyNodes';
import {
    createNetworkConnection,
    deleteNetworkConnection,
    getTopology,
    listVPNPlugins,
    moveTopologyNodes,
} from '@/services/topology';
import {listTunnels} from '@/services/tunnels';
import {getServerAgentStatus, type AgentStatus} from '@/services/servers';
import type {PluginDescriptor, Topology as TopologyData, Tunnel, VPNService} from '@/types';

const emptyTopology: TopologyData = {servers: [], interfaces: [], services: [], connections: []};
const nodeTypes: NodeTypes = {server: TopologyServerNode, interface: TopologyInterfaceNode};
type TopologyNode = ServerFlowNode | InterfaceFlowNode;

export default function Topology() {
    const {t} = useTranslation();
    const [topology, setTopology] = useState<TopologyData>(emptyTopology);
    const [plugins, setPlugins] = useState<PluginDescriptor[]>([]);
    const [tunnels, setTunnels] = useState<Tunnel[]>([]);
    const [agentStatuses, setAgentStatuses] = useState<Record<string, AgentStatus | null>>({});
    const [loading, setLoading] = useState(true);
    const [selection, setSelection] = useState<TopologySelection | null>(null);
    const [view, setView] = useState<'graph' | 'map'>('graph');
    const [serverModal, setServerModal] = useState(false);
    const [serviceModal, setServiceModal] = useState(false);
    const [nodes, setNodes, onNodesChange] = useNodesState<TopologyNode>([]);
    const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);

    const refreshAgentStatuses = useCallback(async (servers: TopologyData['servers']) => {
        const entries = await Promise.all(servers.filter(server => server.legacy).map(async server => [
            server.id,
            await getServerAgentStatus(server.id),
        ] as const));
        setAgentStatuses(previous => ({...previous, ...Object.fromEntries(entries)}));
    }, []);
    const refresh = useCallback(async () => {
        const data = await getTopology();
        setTopology(data);
        void refreshAgentStatuses(data.servers);
    }, [refreshAgentStatuses]);

    useEffect(() => {
        void Promise.all([getTopology(), listVPNPlugins(), listTunnels()]).then(([topologyData, pluginData, tunnelData]) => {
            setTopology(topologyData);
            setPlugins(pluginData);
            setTunnels(tunnelData ?? []);
            void refreshAgentStatuses(topologyData.servers);
        }).catch(error => {
            toast.error(error instanceof Error ? error.message : t('topology.loadError'));
        }).finally(() => setLoading(false));
    }, [refreshAgentStatuses, t]);

    const selectService = useCallback((id: string) => setSelection({kind: 'service', id}), []);

    const graphNodes = useMemo<TopologyNode[]>(() => {
        const serverNodes: ServerFlowNode[] = topology.servers.map(server => ({
            id: server.id,
            type: 'server',
            position: {x: server.x, y: server.y},
            data: {
                server,
                services: topology.services.filter((service: VPNService) => service.serverId === server.id),
                onSelectService: selectService,
            },
        }));
        const interfaceNodes: InterfaceFlowNode[] = topology.interfaces.map(iface => ({
            id: iface.id,
            type: 'interface',
            position: {x: iface.x, y: iface.y},
            data: {iface, service: topology.services.find(service => service.id === iface.serviceId)},
        }));
        return [...serverNodes, ...interfaceNodes];
    }, [topology, selectService]);

    useEffect(() => setNodes(graphNodes), [graphNodes, setNodes]);
    useEffect(() => {
        setEdges(topology.connections.map(connection => ({
            id: connection.id,
            source: connection.sourceId,
            target: connection.targetId,
            markerEnd: {type: MarkerType.ArrowClosed, color: '#0ea5e9'},
            style: {stroke: '#0ea5e9', strokeWidth: 2},
        })));
    }, [topology.connections, setEdges]);

    const handleConnect = useCallback(async (connection: Connection) => {
        if (!connection.source || !connection.target) return;
        try {
            await createNetworkConnection(connection.source, connection.target);
            await refresh();
            toast.success(t('topology.connectionCreated'));
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('topology.saveError'));
        }
    }, [refresh, t]);

    const removeEdge = useCallback(async (id: string) => {
        try {
            await deleteNetworkConnection(id);
            await refresh();
            toast.success(t('topology.deleted'));
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('topology.deleteError'));
        }
    }, [refresh, t]);

    return (
        <div className="flex flex-col">
            <PageHeader
                title={t('nav.topology')}
                icon={Share2}
                description={t('topology.description')}
                actions={
                    <>
                        <div className="mr-1 flex rounded-lg bg-muted p-1" aria-label={t('topology.view')}>
                            <button type="button" onClick={() => setView('graph')} aria-pressed={view === 'graph'} title={t('topology.graphView')} className={`rounded-md p-2 ${view === 'graph' ? 'bg-card text-sky-600 shadow-sm dark:text-sky-400' : 'text-muted-foreground'}`}><Share2 size={16} /></button>
                            <button type="button" onClick={() => setView('map')} aria-pressed={view === 'map'} title={t('topology.mapView')} className={`rounded-md p-2 ${view === 'map' ? 'bg-card text-sky-600 shadow-sm dark:text-sky-400' : 'text-muted-foreground'}`}><MapIcon size={16} /></button>
                        </div>
                        <button type="button" onClick={() => setServiceModal(true)} disabled={topology.servers.length === 0 || !plugins.some(plugin => plugin.available)} className={buttons.secondary} title={t('topology.addService')}>
                            <Network size={15} />{t('topology.addService')}
                        </button>
                        <button type="button" onClick={() => setServerModal(true)} className={buttons.primary} title={t('topology.addServer')}>
                            <Plus size={15} />{t('topology.addServer')}
                        </button>
                    </>
                }
            />
            <div className="flex h-[calc(100vh-150px)] min-h-[560px] overflow-hidden">
                <div className="relative min-w-0 flex-1 bg-muted/20">
                    {loading && <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/60 text-sm text-muted-foreground">{t('common.loading')}</div>}
                    {!loading && topology.servers.length === 0 && (
                        <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center">
                            <div className="max-w-sm rounded-xl border border-border bg-card p-6 text-center shadow-lg">
                                <Share2 className="mx-auto mb-3 text-sky-500" size={32} />
                                <h2 className="font-semibold">{t('topology.emptyTitle')}</h2>
                                <p className="mt-2 text-sm text-muted-foreground">{t('topology.emptyHint')}</p>
                            </div>
                        </div>
                    )}
                    {view === 'graph' ? (
                        <>
                            <ReactFlow<TopologyNode, Edge>
                                nodes={nodes}
                                edges={edges}
                                nodeTypes={nodeTypes}
                                onNodesChange={onNodesChange}
                                onEdgesChange={onEdgesChange}
                                onConnect={handleConnect}
                                connectionMode={ConnectionMode.Strict}
                                isValidConnection={connection => connection.source !== connection.target}
                                onNodeClick={(_, node: Node) => {
                                    if (node.type === 'server' || node.type === 'interface') setSelection({kind: node.type, id: node.id});
                                }}
                                onNodeDragStop={(_, node: Node) => {
                                    void moveTopologyNodes([{id: node.id, x: node.position.x, y: node.position.y}]).catch(error => {
                                        toast.error(error instanceof Error ? error.message : t('topology.saveError'));
                                    });
                                }}
                                onEdgeDoubleClick={(_, edge: Edge) => void removeEdge(edge.id)}
                                onPaneClick={() => setSelection(null)}
                                deleteKeyCode={null}
                                fitView
                                minZoom={0.25}
                                maxZoom={1.75}
                                proOptions={{hideAttribution: false}}
                            >
                                <Background gap={20} />
                                <Controls />
                                <MiniMap pannable zoomable />
                            </ReactFlow>
                            <div className="pointer-events-none absolute bottom-3 left-1/2 z-10 -translate-x-1/2 rounded-full border border-border bg-card/95 px-4 py-2 text-center text-xs text-muted-foreground shadow-sm">
                                {t('topology.canvasHint')}
                            </div>
                        </>
                    ) : (
                        <TopologyMap
                            servers={topology.servers}
                            interfaces={topology.interfaces}
                            services={topology.services}
                            connections={topology.connections}
                            tunnels={tunnels}
                            agentStatuses={agentStatuses}
                            onSelect={id => setSelection({kind: 'server', id})}
                        />
                    )}
                </div>
                {selection && (
                    <TopologyInspector
                        selection={selection}
                        topology={topology}
                        plugins={plugins}
                        onClose={() => setSelection(null)}
                        onSelectService={selectService}
                        refresh={refresh}
                    />
                )}
            </div>
            {serverModal && (
                <AddServerModal
                    onClose={() => setServerModal(false)}
                    onCreated={data => {
                        setTopology(data);
                        setServerModal(false);
                        const server = data.servers[data.servers.length - 1];
                        if (server) setSelection({kind: 'server', id: server.id});
                        void refresh();
                    }}
                />
            )}
            {serviceModal && (
                <AddServiceModal
                    topology={topology}
                    plugins={plugins}
                    onClose={() => setServiceModal(false)}
                    onCreated={service => {
                        setServiceModal(false);
                        setSelection({kind: 'service', id: service.id});
                        void refresh();
                    }}
                />
            )}
        </div>
    );
}
