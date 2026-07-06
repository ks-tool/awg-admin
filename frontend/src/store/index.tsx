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

import { create } from 'zustand'
import type { Server, User, DashboardStats, PeerInfo, Interface, InterfaceConfig } from '@/types'
import * as usersService from '@/services/users'
import * as serversService from '@/services/servers'
import * as interfacesService from '@/services/interfaces'
import * as peersService from '@/services/peers'
import type {ServerInput} from "@/services/servers";
import {SSHPassphraseRequiredError} from "@/services/sshErrors";
import {byName} from "@/lib/utils";

/**
 * Helper function to calculate stats from servers and users
 */
function calculateStats(
  servers: Server[],
  users: User[],
  allPeers: PeerInfo[],
  interfaces: Map<string, Interface>,
): DashboardStats {
  let totalRxBytes = 0
  let totalTxBytes = 0

  // Sum traffic across all peers.
  allPeers.forEach((peer) => {
    totalRxBytes += peer.rx || 0
    totalTxBytes += peer.tx || 0
  })

  // Active peers = the ones that aren't deactivated (models.Peer.disabled — a
  // deactivated peer is kept but dropped from the live interface, so it can't
  // connect). Counted from `users`, since that's where the disabled flag lives;
  // the flattened allPeers PeerInfo list carries only placeholder online/rx/tx.
  let activePeers = 0
  users.forEach((u) => {
    for (const p of u.peers ?? []) {
      if (!p.disabled) activePeers++
    }
  })

  // A tunnel is a group of interfaces sharing a non-empty tunnel id (see
  // models.Interface.Tunnel); count the distinct ids.
  const tunnelIds = new Set<string>()
  interfaces.forEach((iface) => {
    if (iface.tunnel) tunnelIds.add(iface.tunnel)
  })

  return {
    totalServers: servers.length,
    onlineServers: servers.length, // TODO: Implement proper status checking
    totalPeers: allPeers.length,
    activePeers,
    totalUsers: users.length,
    totalTunnels: tunnelIds.size,
    totalRxBytes,
    totalTxBytes,
  }
}

interface AppState {
  // Data
  servers: Server[]
  peers: PeerInfo[]  // Flattened peer list
  users: User[]
  interfaces: Map<string, Interface>  // Cache interface objects by ID
  stats: DashboardStats | null
  selectedServerId: string | null
  selectedUserId: string | null
  
  // Loading states
  isLoading: boolean
  isLoadingServers: boolean
  isLoadingUsers: boolean
  lastRefresh: Date | null
  
  // Actions
  setSelectedServer: (id: string | null) => void
  setSelectedUser: (id: string | null) => void
  refreshData: () => Promise<void>
  
  // Server actions
  fetchServers: () => Promise<void>
  getServer: (id: string) => Promise<Server | null>
  createServer: (input: ServerInput) => Promise<Server | null>
  updateServer: (id: string, input: serversService.ServerInput) => Promise<Server | null>
  deleteServer: (id: string) => Promise<boolean>
  
  // User actions
  fetchUsers: () => Promise<void>
  getUser: (id: string) => Promise<User | null>
  createUser: (input: usersService.CreateUserInput) => Promise<User | null>
  updateUser: (id: string, input: usersService.UpdateUserInput) => Promise<User | null>
  deleteUser: (id: string) => Promise<boolean>
  
  // Peer actions
  addPeer: (userId: string, input: peersService.AddPeerInput) => Promise<boolean>
  deletePeer: (userId: string, publicKey: string) => Promise<boolean>
  
  // Interface actions
  listInterfacesForServer: (serverId: string) => Promise<Interface[] | null>
  createInterface: (serverId: string, config: InterfaceConfig) => Promise<Interface | null>
  deleteInterface: (serverId: string, interfaceId: string) => Promise<boolean>
  
  // Helper actions
  getInterfaceName: (interfaceId: string) => string | null
}

export const useAppStore = create<AppState>((set, get) => ({
  // Initial state
  servers: [],
  peers: [],
  users: [],
  interfaces: new Map(),
  stats: null,
  selectedServerId: null,
  selectedUserId: null,
  isLoading: false,
  isLoadingServers: false,
  isLoadingUsers: false,
  lastRefresh: null,

  // Basic actions
  setSelectedServer: (id) => set({ selectedServerId: id }),
  setSelectedUser: (id) => set({ selectedUserId: id }),

  refreshData: async () => {
    set({ isLoading: true })
    try {
      await Promise.all([get().fetchServers(), get().fetchUsers()])
      set({ lastRefresh: new Date() })
    } catch (error) {
      console.error('Failed to refresh data:', error)
    } finally {
      set({ isLoading: false })
    }
  },

  // Server actions
  fetchServers: async () => {
    set({ isLoadingServers: true })
    try {
      const servers = await serversService.listServers()
      if (servers) {
        // Keep the canonical list alphabetical so every consumer (Dashboard,
        // Servers page, tunnel/peer server dropdowns) renders in a stable order.
        servers.sort(byName((s) => s.name))
        const interfacesMap = new Map<string, Interface>()

        // Fetch all interfaces for all servers concurrently
        const fetches = servers.flatMap((server) =>
          (server.interfaces || []).map((interfaceId) =>
            interfacesService.getInterface(server.id, interfaceId)
              .then((iface) => {
                if (iface) interfacesMap.set(interfaceId, iface)
              })
              .catch((error) => {
                console.error(`Failed to fetch interface ${interfaceId}:`, error)
              })
          )
        )
        await Promise.all(fetches)

        set({
          servers,
          interfaces: interfacesMap,
        })

        // Update stats after loading interfaces
        const { users, peers } = get()
        set({ stats: calculateStats(servers, users, peers, interfacesMap) })
      }
    } catch (error) {
      console.error('Failed to fetch servers:', error)
    } finally {
      set({ isLoadingServers: false })
    }
  },

  getServer: async (id) => {
    try {
      return await serversService.getServer(id)
    } catch (error) {
      console.error('Failed to get server:', error)
      return null
    }
  },

  createServer: async (input) => {
    try {
      const server = await serversService.createServer(input)
      if (server) {
        const { servers, users, peers, interfaces } = get()
        const newServers = [...servers, server].sort(byName((s) => s.name))
        set({ 
          servers: newServers,
          stats: calculateStats(newServers, users, peers, interfaces)
        })
      }
      return server
    } catch (error) {
      if (error instanceof SSHPassphraseRequiredError) throw error
      console.error('Failed to create server:', error)
      return null
    }
  },

  updateServer: async (id, input) => {
    try {
      const server = await serversService.updateServer(id, input)
      if (server) {
        const { servers, users, peers, interfaces } = get()
        // Re-sort in case the name changed (an edit shouldn't reorder silently,
        // but a rename should land in its new alphabetical slot).
        const newServers = servers.map((s) => (s.id === id ? server : s)).sort(byName((s) => s.name))
        set({
          servers: newServers,
          stats: calculateStats(newServers, users, peers, interfaces)
        })
      }
      return server
    } catch (error) {
      if (error instanceof SSHPassphraseRequiredError) throw error
      console.error('Failed to update server:', error)
      return null
    }
  },

  deleteServer: async (id) => {
    try {
      const success = await serversService.deleteServer(id)
      if (success) {
        const { servers, users, peers, interfaces } = get()
        const newServers = servers.filter((s) => s.id !== id)
        set({ 
          servers: newServers,
          stats: calculateStats(newServers, users, peers, interfaces)
        })
      }
      return success
    } catch (error) {
      console.error('Failed to delete server:', error)
      return false
    }
  },

  // User actions
  fetchUsers: async () => {
    set({ isLoadingUsers: true })
    try {
      const users = await usersService.listUsers()
      if (users) {
        // Alphabetical users, and alphabetical peers within each user, so the
        // Users list and UserDetail render in a stable order.
        users.forEach((u) => u.peers?.sort(byName((p) => p.name)))
        users.sort(byName((u) => u.name))
        set({ users })
        
        // Extract all peers from all users for the flattened peer list
        // Note: User.peers are Peer objects with {pk, interface, disabled}
        // For display purposes, we count them for stats
        // Full PeerInfo data would come from device info endpoints
        const allPeers: PeerInfo[] = []
        users.forEach(user => {
          if (user.peers && user.peers.length > 0) {
            // Create placeholder PeerInfo objects for counting
            user.peers.forEach(peer => {
              allPeers.push({
                publicKey: peer.pk,
                endpoint: '',
                lastHandshake: 0,
                rx: 0,
                tx: 0,
                online: false,
                updated_at: 0,
              })
            })
          }
        })
        
        set({ peers: allPeers })
        
        // Update stats after users are fetched
        const { servers, interfaces } = get()
        set({ stats: calculateStats(servers, users, allPeers, interfaces) })
      }
    } catch (error) {
      console.error('Failed to fetch users:', error)
    } finally {
      set({ isLoadingUsers: false })
    }
  },

  getUser: async (id) => {
    try {
      return await usersService.getUser(id)
    } catch (error) {
      console.error('Failed to get user:', error)
      return null
    }
  },

  createUser: async (input) => {
    try {
      const user = await usersService.createUser(input)
      if (user) {
        const { users, servers, peers, interfaces } = get()
        const newUsers = [...users, user].sort(byName((u) => u.name))
        set({ 
          users: newUsers,
          stats: calculateStats(servers, newUsers, peers, interfaces)
        })
      }
      return user
    } catch (error) {
      console.error('Failed to create user:', error)
      return null
    }
  },

  updateUser: async (id, input) => {
    try {
      const user = await usersService.updateUser(id, input)
      if (user) {
        const { users, servers, peers, interfaces } = get()
        const newUsers = users.map((u) => (u.id === id ? user : u)).sort(byName((u) => u.name))
        set({
          users: newUsers,
          stats: calculateStats(servers, newUsers, peers, interfaces)
        })
      }
      return user
    } catch (error) {
      console.error('Failed to update user:', error)
      return null
    }
  },

  deleteUser: async (id) => {
    try {
      const success = await usersService.deleteUser(id)
      if (success) {
        const { users, servers, peers, interfaces } = get()
        const newUsers = users.filter((u) => u.id !== id)
        set({ 
          users: newUsers,
          stats: calculateStats(servers, newUsers, peers, interfaces)
        })
      }
      return success
    } catch (error) {
      console.error('Failed to delete user:', error)
      return false
    }
  },

  // Peer actions
  addPeer: async (userId: string, input: peersService.AddPeerInput) => {
    try {
      const success = await peersService.addPeer(userId, input)
      if (success) {
        // Refresh users and peers after adding a peer
        await get().refreshData()
      }
      return success
    } catch (error) {
      // Re-throw so the caller can surface the specific reason (e.g. a duplicate
      // or out-of-subnet IP) to the user instead of a generic failure.
      console.error('Failed to add peer:', error)
      throw error
    }
  },

  deletePeer: async (userId: string, publicKey: string) => {
    try {
      const success = await peersService.deletePeer(userId, publicKey)
      if (success) {
        // Refresh users and peers after deleting a peer
        await get().refreshData()
      }
      return success
    } catch (error) {
      console.error('Failed to delete peer:', error)
      return false
    }
  },

  // Interface actions
  listInterfacesForServer: async (serverId: string) => {
    try {
      const interfaces = await interfacesService.listInterfaces(serverId)
      if (interfaces) {
        // Cache all interfaces in the store
        const { interfaces: existingInterfaces } = get()
        const newInterfaces = new Map(existingInterfaces)
        interfaces.forEach(iface => {
          newInterfaces.set(iface.id, iface)
        })
        set({ interfaces: newInterfaces })
      }
      return interfaces
    } catch (error) {
      console.error(`Failed to list interfaces for server ${serverId}:`, error)
      return null
    }
  },

  createInterface: async (serverId: string, config: InterfaceConfig) => {
    try {
      const iface = await interfacesService.createInterface(serverId, config)
      if (iface) {
        const { interfaces } = get()
        const newInterfaces = new Map(interfaces)
        newInterfaces.set(iface.id, iface)
        set({ interfaces: newInterfaces })
        // Refresh servers to update interfaces list
        await get().fetchServers()
      }
      return iface
    } catch (error) {
      // Re-throw so the caller can surface the specific reason (e.g. a
      // validation conflict like a duplicate name/port/subnet) to the user,
      // rather than a generic failure.
      console.error('Failed to create interface:', error)
      throw error
    }
  },

  deleteInterface: async (serverId: string, interfaceId: string) => {
    try {
      const success = await interfacesService.deleteInterface(serverId, interfaceId)
      if (success) {
        const { interfaces } = get()
        const newInterfaces = new Map(interfaces)
        newInterfaces.delete(interfaceId)
        set({ interfaces: newInterfaces })
        // Refresh servers to update interfaces list
        await get().fetchServers()
      }
      return success
    } catch (error) {
      console.error('Failed to delete interface:', error)
      return false
    }
  },

  // Get interface name from interface ID using already-loaded servers data
  getInterfaceName: (interfaceId: string) => {
    const { interfaces } = get()
    const iface = interfaces.get(interfaceId)
    return iface?.iface || null
  },
}))

