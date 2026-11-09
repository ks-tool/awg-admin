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
import type { Server, User, Interface, InterfaceConfig } from '@/types'
import * as usersService from '@/services/users'
import * as serversService from '@/services/servers'
import * as interfacesService from '@/services/interfaces'
import * as peersService from '@/services/peers'
import type {ServerInput} from "@/services/servers";
import {SSHPassphraseRequiredError} from "@/services/sshErrors";
import {byName} from "@/lib/utils";

interface AppState {
  servers: Server[]
  users: User[]
  interfaces: Map<string, Interface>
  selectedUserId: string | null
  isLoadingUsers: boolean

  setSelectedUser: (id: string | null) => void
  refreshData: () => Promise<void>

  fetchServers: () => Promise<void>
  updateServer: (id: string, input: ServerInput) => Promise<Server | null>

  fetchUsers: () => Promise<void>
  createUser: (input: usersService.CreateUserInput) => Promise<User | null>
  updateUser: (id: string, input: usersService.UpdateUserInput) => Promise<User | null>
  deleteUser: (id: string) => Promise<boolean>

  addPeer: (userId: string, input: peersService.AddPeerInput) => Promise<boolean>
  deletePeer: (userId: string, publicKey: string) => Promise<boolean>

  listInterfacesForServer: (serverId: string) => Promise<Interface[] | null>
  createInterface: (serverId: string, config: InterfaceConfig) => Promise<Interface | null>
  deleteInterface: (serverId: string, interfaceId: string) => Promise<boolean>

  getInterfaceName: (interfaceId: string) => string | null
}

export const useAppStore = create<AppState>((set, get) => ({
  servers: [],
  users: [],
  interfaces: new Map(),
  selectedUserId: null,
  isLoadingUsers: false,

  setSelectedUser: (id) => set({ selectedUserId: id }),

  refreshData: async () => {
    await Promise.all([get().fetchServers(), get().fetchUsers()])
  },

  // Server actions
  fetchServers: async () => {
    try {
      const servers = await serversService.listServers()
      if (servers) {
        servers.sort(byName((s) => s.name))
        const interfacesMap = new Map<string, Interface>()

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
        set({ servers, interfaces: interfacesMap })
      }
    } catch (error) {
      console.error('Failed to fetch servers:', error)
    }
  },

  updateServer: async (id, input) => {
    try {
      const server = await serversService.updateServer(id, input)
      if (server) {
        const { servers } = get()
        const newServers = servers.map((s) => (s.id === id ? server : s)).sort(byName((s) => s.name))
        set({ servers: newServers })
      }
      return server
    } catch (error) {
      if (error instanceof SSHPassphraseRequiredError) throw error
      console.error('Failed to update server:', error)
      return null
    }
  },

  // User actions
  fetchUsers: async () => {
    set({ isLoadingUsers: true })
    try {
      const users = await usersService.listUsers()
      if (users) {
        users.forEach((u) => u.peers?.sort(byName((p) => p.name)))
        users.sort(byName((u) => u.name))
        set({ users })
      }
    } catch (error) {
      console.error('Failed to fetch users:', error)
    } finally {
      set({ isLoadingUsers: false })
    }
  },

  createUser: async (input) => {
    try {
      const user = await usersService.createUser(input)
      if (user) {
        const { users } = get()
        set({ users: [...users, user].sort(byName((u) => u.name)) })
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
        const { users } = get()
        set({ users: users.map((u) => (u.id === id ? user : u)).sort(byName((u) => u.name)) })
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
        const { users } = get()
        set({ users: users.filter((u) => u.id !== id) })
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

