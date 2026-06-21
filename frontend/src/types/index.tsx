export * from './admin';
export * from './agent';

export interface DashboardStats {
  totalServers: number
  onlineServers: number
  totalPeers: number
  activePeers: number
  totalUsers: number
  totalTunnels: number
  totalRxBytes: number
  totalTxBytes: number
}
