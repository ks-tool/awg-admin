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
import {ArrowRight, Map as MapIcon, Plus, Settings as SettingsIcon, Share2, Users as UsersIcon} from 'lucide-react';
import {toast} from 'sonner';
import {NavigationProvider, type PageId} from '@/contexts/NavigationContext';
import Users from './Users';
import AddUser from './AddUser';
import UserDetail from './UserDetail';
import Settings from './Settings';
import {buttons, Modal} from '@/components/common/Modal';
import {ConfirmModal} from '@/components/common/ConfirmModal';
import {ServerMetricsModal} from '@/components/server/ServerMetricsModal';
import {AddServerModal} from '@/components/topology/TopologyModals';
import {TopologyInspector, type TopologySelection} from '@/components/topology/TopologyInspector';
import {TopologyMap} from '@/components/topology/TopologyMap';
import {TopologyInterfaceNode, type InterfaceFlowNode} from '@/components/topology/TopologyNodes';
import {canCreateTopologyConnection, mergeTopologyConnections} from '@/components/topology/connections';
import {
    createNetworkConnection,
    deleteNetworkConnection,
    getTopology,
    listVPNPlugins,
    moveTopologyNodes,
} from '@/services/topology';
import {listTunnels} from '@/services/tunnels';
import {getServerAgentStatus, type AgentStatus} from '@/services/servers';
import {useAppStore} from '@/store';
import {useAutoRefresh} from '@/hooks/useAutoRefresh';
import type {PluginDescriptor, Topology as TopologyData, Tunnel} from '@/types';

const emptyTopology: TopologyData = {servers: [], interfaces: [], services: [], connections: []};
const nodeTypes: NodeTypes = {interface: TopologyInterfaceNode};
type TopologyNode = InterfaceFlowNode;
type TopologyView = 'map' | 'graph' | 'users' | 'settings';
type UserView = 'list' | 'detail';

export default function Topology() {
    const {t} = useTranslation();
    const users = useAppStore(state => state.users);
    const refreshData = useAppStore(state => state.refreshData);
    useAutoRefresh(refreshData);
    const [topology, setTopology] = useState<TopologyData>(emptyTopology);
    const [plugins, setPlugins] = useState<PluginDescriptor[]>([]);
    const [tunnels, setTunnels] = useState<Tunnel[]>([]);
    const [agentStatuses, setAgentStatuses] = useState<Record<string, AgentStatus | null>>({});
    const [loading, setLoading] = useState(true);
    const [selection, setSelection] = useState<TopologySelection | null>(null);
    const [view, setView] = useState<TopologyView>('map');
    const [userView, setUserView] = useState<UserView>('list');
    const [serverModal, setServerModal] = useState(false);
    const [addUserModal, setAddUserModal] = useState(false);
    const [metricsModalServerId, setMetricsModalServerId] = useState<string | null>(null);
    const [connectionToCreate, setConnectionToCreate] = useState<{source: string; target: string} | null>(null);
    const [creatingConnection, setCreatingConnection] = useState(false);
    const [connectionToDelete, setConnectionToDelete] = useState<Edge | null>(null);
    const [deletingConnection, setDeletingConnection] = useState(false);
    const [nodes, setNodes, onNodesChange] = useNodesState<TopologyNode>([]);
    const [edges, setEdges, onEdgesChange] = useEdgesState<Edge>([]);

    const navigate = useCallback((page: PageId) => {
        if (page === 'users') {
            setUserView('list');
            setView('users');
        } else if (page === 'add-user') {
            setUserView('list');
            setView('users');
            setAddUserModal(true);
        } else if (page === 'user-detail') {
            setUserView('detail');
            setView('users');
        } else if (page === 'settings') {
            setView('settings');
        }
    }, []);

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

    const graphNodes = useMemo<TopologyNode[]>(() => topology.interfaces.map(iface => ({
        id: iface.id,
        type: 'interface',
        position: {x: iface.x, y: iface.y},
        data: {
            iface,
            server: topology.servers.find(server => server.id === iface.serverId),
            service: topology.services.find(service => service.id === iface.serviceId),
        },
    })), [topology]);

    useEffect(() => setNodes(graphNodes), [graphNodes, setNodes]);
    const graphConnections = useMemo(() => mergeTopologyConnections(topology.connections, tunnels), [topology.connections, tunnels]);
    useEffect(() => {
        setEdges(graphConnections.map(connection => ({
            id: connection.id,
            source: connection.sourceId,
            target: connection.targetId,
            markerEnd: {type: MarkerType.ArrowClosed, color: '#0ea5e9'},
            style: {stroke: '#0ea5e9', strokeWidth: 2},
        })));
    }, [graphConnections, setEdges]);

    const canConnect = useCallback((connection: {source: string | null; target: string | null}) => {
        if (!connection.source || !connection.target) return false;
        return canCreateTopologyConnection(graphConnections, connection.source, connection.target);
    }, [graphConnections]);
    const handleConnect = useCallback((connection: Connection) => {
        if (!connection.source || !connection.target) return;
        if (!canConnect(connection)) {
            toast.error(t('topology.connectionCycle'));
            return;
        }
        setConnectionToCreate({source: connection.source, target: connection.target});
    }, [canConnect, t]);
    const confirmCreateConnection = useCallback(async () => {
        if (!connectionToCreate) return;
        setCreatingConnection(true);
        try {
            if (!canConnect(connectionToCreate)) {
                toast.error(t('topology.connectionCycle'));
                setConnectionToCreate(null);
                return;
            }
            await createNetworkConnection(connectionToCreate.source, connectionToCreate.target);
            await refresh();
            setConnectionToCreate(null);
            toast.success(t('topology.connectionCreated'));
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('topology.saveError'));
        } finally {
            setCreatingConnection(false);
        }
    }, [canConnect, connectionToCreate, refresh, t]);
    const connectionLabel = useCallback((id: string) => {
        const iface = topology.interfaces.find(item => item.id === id);
        const server = topology.servers.find(item => item.id === iface?.serverId);
        if (!iface) return id;
        return `${server ? `${server.name}[${server.address || '—'}]` : '—'} / ${iface.name}`;
    }, [topology.interfaces, topology.servers]);

    const selectedConnection = edges.find(edge => edge.selected && !edge.id.startsWith('tunnel:'));
    useEffect(() => {
        if (!selectedConnection || connectionToCreate || connectionToDelete) return;
        const handleKeyDown = (event: KeyboardEvent) => {
            if (event.key !== 'Delete' && event.key !== 'Backspace') return;
            if (event.target instanceof HTMLElement && event.target.closest('input, textarea, select, [contenteditable="true"]')) return;
            event.preventDefault();
            setConnectionToDelete(selectedConnection);
        };
        window.addEventListener('keydown', handleKeyDown);
        return () => window.removeEventListener('keydown', handleKeyDown);
    }, [selectedConnection, connectionToCreate, connectionToDelete]);
    const confirmDeleteConnection = useCallback(async () => {
        if (!connectionToDelete) return;
        setDeletingConnection(true);
        try {
            await deleteNetworkConnection(connectionToDelete.id);
            await refresh();
            setConnectionToDelete(null);
            toast.success(t('topology.deleted'));
        } catch (error) {
            toast.error(error instanceof Error ? error.message : t('topology.deleteError'));
        } finally {
            setDeletingConnection(false);
        }
    }, [connectionToDelete, refresh, t]);

    return (
        <NavigationProvider navigate={navigate}>
        <div className="flex h-screen flex-col overflow-hidden">
            <div className="relative flex min-h-0 flex-1 overflow-hidden">
                <div className="relative min-w-0 flex-1 bg-muted/20">
                    <div className="absolute right-4 top-4 z-30 flex rounded-lg border border-border bg-card/90 p-1 shadow-sm backdrop-blur" aria-label={t('topology.view')}>
                        <button
                            type="button"
                            onClick={() => view === 'users' ? navigate('add-user') : setServerModal(true)}
                            disabled={view === 'settings'}
                            className="mr-1 rounded-md bg-sky-600 p-2 text-white shadow-sm transition-colors hover:bg-sky-500 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-sky-600"
                            title={view === 'users' ? t('users.addUser') : t('topology.addServer')}
                            aria-label={view === 'users' ? t('users.addUser') : t('topology.addServer')}
                        >
                            <Plus size={16} />
                        </button>
                        <button type="button" onClick={() => setView('map')} aria-pressed={view === 'map'} title={t('topology.mapView')} className={`rounded-md p-2 ${view === 'map' ? 'bg-card text-sky-600 shadow-sm dark:text-sky-400' : 'text-muted-foreground'}`}><MapIcon size={16} /></button>
                        <button type="button" onClick={() => setView('graph')} aria-pressed={view === 'graph'} title={t('topology.graphView')} className={`rounded-md p-2 ${view === 'graph' ? 'bg-card text-sky-600 shadow-sm dark:text-sky-400' : 'text-muted-foreground'}`}><Share2 size={16} /></button>
                        <button type="button" onClick={() => navigate('users')} aria-pressed={view === 'users'} title={t('nav.users')} aria-label={t('nav.users')} className={`rounded-md p-2 ${view === 'users' ? 'bg-card text-sky-600 shadow-sm dark:text-sky-400' : 'text-muted-foreground'}`}><UsersIcon size={16} /></button>
                        <button type="button" onClick={() => navigate('settings')} aria-pressed={view === 'settings'} title={t('nav.settings')} aria-label={t('nav.settings')} className={`rounded-md p-2 ${view === 'settings' ? 'bg-card text-sky-600 shadow-sm dark:text-sky-400' : 'text-muted-foreground'}`}><SettingsIcon size={16} /></button>
                    </div>
                    {(view === 'map' || view === 'graph') && loading && <div className="absolute inset-0 z-10 flex items-center justify-center bg-background/60 text-sm text-muted-foreground">{t('common.loading')}</div>}
                    {(view === 'map' || view === 'graph') && !loading && topology.servers.length === 0 && (
                        <div className="pointer-events-none absolute inset-0 z-10 flex items-center justify-center">
                            <div className="max-w-sm rounded-xl border border-border bg-card p-6 text-center shadow-lg">
                                <Share2 className="mx-auto mb-3 text-sky-500" size={32} />
                                <h2 className="font-semibold">{t('topology.emptyTitle')}</h2>
                                <p className="mt-2 text-sm text-muted-foreground">{t('topology.emptyHint')}</p>
                            </div>
                        </div>
                    )}
                    {view === 'users' || view === 'settings' ? (
                        <div className="h-full overflow-y-auto pt-14">
                            {view === 'settings' ? <Settings /> : userView === 'detail' ? <UserDetail /> : <Users />}
                        </div>
                    ) : view === 'graph' ? (
                        <>
                            <ReactFlow<TopologyNode, Edge>
                                nodes={nodes}
                                edges={edges}
                                nodeTypes={nodeTypes}
                                onNodesChange={onNodesChange}
                                onEdgesChange={onEdgesChange}
                                onConnect={handleConnect}
                                connectionMode={ConnectionMode.Strict}
                                isValidConnection={canConnect}
                                onNodeDragStop={(_, node: Node) => {
                                    void moveTopologyNodes([{id: node.id, x: node.position.x, y: node.position.y}]).catch(error => {
                                        toast.error(error instanceof Error ? error.message : t('topology.saveError'));
                                    });
                                }}
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
                            totalUsers={users.length}
                            onSelect={setMetricsModalServerId}
                        />
                    )}
                </div>
                {selection && (view === 'map' || view === 'graph') && (
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
            {connectionToCreate && (
                <Modal title={t('topology.createConnection')} onClose={() => setConnectionToCreate(null)} loading={creatingConnection}>
                    <p className="mb-4 text-sm text-muted-foreground">{t('topology.confirmCreateConnection')}</p>
                    <div className="mb-6 flex items-center gap-3 rounded-lg bg-muted p-4 text-sm">
                        <span className="min-w-0 flex-1 break-words">{connectionLabel(connectionToCreate.source)}</span>
                        <ArrowRight className="shrink-0 text-sky-500" size={20} />
                        <span className="min-w-0 flex-1 break-words text-right">{connectionLabel(connectionToCreate.target)}</span>
                    </div>
                    <div className="flex justify-end gap-2">
                        <button type="button" onClick={() => setConnectionToCreate(null)} disabled={creatingConnection} className={buttons.secondary}>{t('common.cancel')}</button>
                        <button type="button" onClick={() => void confirmCreateConnection()} disabled={creatingConnection} className={buttons.primary}>{t('common.create')}</button>
                    </div>
                </Modal>
            )}
            {connectionToDelete && (
                <ConfirmModal
                    title={t('topology.deleteConnection')}
                    message={t('topology.confirmDeleteConnection', {
                        source: connectionLabel(connectionToDelete.source),
                        target: connectionLabel(connectionToDelete.target),
                    })}
                    onConfirm={() => void confirmDeleteConnection()}
                    onCancel={() => setConnectionToDelete(null)}
                    loading={deletingConnection}
                />
            )}
            {metricsModalServerId && (
                <ServerMetricsModal
                    serverId={metricsModalServerId}
                    serverName={topology.servers.find(server => server.id === metricsModalServerId)?.name ?? ''}
                    topologyServer={topology.servers.find(server => server.id === metricsModalServerId)}
                    topology={topology}
                    onChanged={refresh}
                    onClose={() => setMetricsModalServerId(null)}
                />
            )}
            {addUserModal && <AddUser onClose={() => setAddUserModal(false)} />}
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
        </div>
        </NavigationProvider>
    );
}
