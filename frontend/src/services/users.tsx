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
import type { User } from '@/types';

export interface CreateUserInput {
  name: string;
  description?: string;
}

export interface UpdateUserInput {
  name?: string;
  description?: string;
  disabled?: boolean;
}

/**
 * Get the appropriate client based on current API mode
 */
function getClient() {
  return getCurrentApiMode() === 'bindings' ? bindingsClient : null;
}

/**
 * Get all users
 */
export async function listUsers(): Promise<User[] | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.listUsers();
    if (error) {
      reportError('fetch-users', 'Failed to fetch users', error);
      return null;
    }
    return data as unknown as User[];
  }

  const { data, error } = await get<User[]>('/users/');
  if (error) {
    reportError('fetch-users', 'Failed to fetch users', error);
    return null;
  }
  return data;
}

/**
 * Get a single user by ID
 */
export async function getUser(userId: string): Promise<User | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.getUser(userId);
    if (error) {
      reportError(`fetch-user-${userId}`, `Failed to fetch user ${userId}`, error);
      return null;
    }
    return data as unknown as User;
  }

  const { data, error } = await get<User>(`/users/${userId}`);
  if (error) {
    reportError(`fetch-user-${userId}`, `Failed to fetch user ${userId}`, error);
    return null;
  }
  return data;
}

/**
 * Create a new user
 */
export async function createUser(input: CreateUserInput): Promise<User | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.createUser(input);
    if (error) {
      console.error('Failed to create user (bindings):', error);
      return null;
    }
    return data as unknown as User;
  }
  
  const { data, error } = await post<User, CreateUserInput>('/users', input);
  if (error) {
    console.error('Failed to create user:', error);
    return null;
  }
  return data;
}

/**
 * Update an existing user
 */
export async function updateUser(userId: string, input: UpdateUserInput): Promise<User | null> {
  const client = getClient();
  
  if (client) {
    const { data, error } = await client.updateUser(userId, input);
    if (error) {
      console.error(`Failed to update user ${userId} (bindings):`, error);
      return null;
    }
    return data as unknown as User;
  }
  
  const { data, error } = await put<User, UpdateUserInput>(`/users/${userId}`, input);
  if (error) {
    console.error(`Failed to update user ${userId}:`, error);
    return null;
  }
  return data;
}

/**
 * Delete a user
 */
export async function deleteUser(userId: string): Promise<boolean> {
  const client = getClient();
  
  if (client) {
    const { error } = await client.deleteUser(userId);
    if (error) {
      console.error(`Failed to delete user ${userId} (bindings):`, error);
      return false;
    }
    return true;
  }
  
  const { error } = await remove<void>(`/users/${userId}`);
  if (error) {
    console.error(`Failed to delete user ${userId}:`, error);
    return false;
  }
  return true;
}
