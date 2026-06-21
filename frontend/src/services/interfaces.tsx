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

import { get, post, put, remove } from './api';
import { getCurrentApiMode } from './apiMode';
import { bindingsClient } from './bindingsClient';
import { reportError } from './errorReporting';
import type { Interface, InterfaceConfig } from '@/types';

/**
 * Get the appropriate client based on current API mode
 */
function getClient() {
  return getCurrentApiMode() === 'bindings' ? bindingsClient : null;
}

/**
 * Get all interfaces for a server
 */
export async function listInterfaces(serverId: string): Promise<Interface[] | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.listInterfaces(serverId);
    if (error) {
      reportError(`fetch-interfaces-${serverId}`, `Failed to fetch interfaces for server ${serverId}`, error);
      return null;
    }
    return data as unknown as Interface[];
  }

  const { data, error } = await get<Interface[]>(`/servers/${serverId}/interfaces/`);
  if (error) {
    reportError(`fetch-interfaces-${serverId}`, `Failed to fetch interfaces for server ${serverId}`, error);
    return null;
  }
  return data;
}

/**
 * Get a single interface by ID
 */
export async function getInterface(serverId: string, interfaceId: string): Promise<Interface | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.getInterface(serverId, interfaceId);
    if (error) {
      reportError(`fetch-interface-${interfaceId}`, `Failed to fetch interface ${interfaceId}`, error);
      return null;
    }
    return data as unknown as Interface;
  }

  const { data, error } = await get<Interface>(
    `/servers/${serverId}/interfaces/${interfaceId}`
  );
  if (error) {
    reportError(`fetch-interface-${interfaceId}`, `Failed to fetch interface ${interfaceId}`, error);
    return null;
  }
  return data;
}

/**
 * Create a new interface
 */
export async function createInterface(
  serverId: string,
  config: InterfaceConfig
): Promise<Interface | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.createInterface(serverId, config);
    if (error) {
      console.error(`Failed to create interface on server ${serverId} (bindings):`, error);
      return null;
    }
    return data as unknown as Interface;
  }
  
  const { data, error } = await post<Interface, InterfaceConfig>(
    `/servers/${serverId}/interfaces`,
    config
  );
  if (error) {
    console.error(`Failed to create interface on server ${serverId}:`, error);
    return null;
  }
  return data;
}

/**
 * Update interface configuration
 */
export async function updateInterfaceConfig(
  serverId: string,
  interfaceId: string,
  config: InterfaceConfig
): Promise<Interface | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.updateInterfaceConfig(serverId, interfaceId, config);
    if (error) {
      console.error(`Failed to update interface ${interfaceId} config (bindings):`, error);
      return null;
    }
    return data as unknown as Interface;
  }
  
  const { data, error } = await put<Interface, InterfaceConfig>(
    `/servers/${serverId}/interfaces/${interfaceId}/config`,
    config
  );
  if (error) {
    console.error(`Failed to update interface ${interfaceId} config:`, error);
    return null;
  }
  return data;
}

/**
 * Delete an interface
 */
export async function deleteInterface(serverId: string, interfaceId: string): Promise<boolean> {
  const client = getClient();
  
  if (client) {
    const { error } = await client.deleteInterface(serverId, interfaceId);
    if (error) {
      console.error(`Failed to delete interface ${interfaceId} (bindings):`, error);
      return false;
    }
    return true;
  }
  
  const { error } = await remove<void>(`/servers/${serverId}/interfaces/${interfaceId}`);
  if (error) {
    console.error(`Failed to delete interface ${interfaceId}:`, error);
    return false;
  }
  return true;
}
