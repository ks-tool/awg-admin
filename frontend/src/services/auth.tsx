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

// Login/session is an HTTP-only concept — the Wails desktop app talks to Go
// directly via bindings and never goes through internal/api, so it has
// nothing to authenticate against. Callers should only use this module when
// getCurrentApiMode() === 'http' (see apiMode.tsx).

import {get, patch, post} from './api';

export interface CurrentUser {
    username: string;
    basicAuthEnabled: boolean;
}

/** Returns the logged-in user, or null if there's no valid session. */
export async function getCurrentUser(): Promise<CurrentUser | null> {
    const {data, error} = await get<CurrentUser>('/auth/me');
    if (error) return null;
    return data;
}

/**
 * Toggles HTTP Basic Auth in front of the whole standalone server (login
 * page, static assets, API — everything), checked against this same
 * admin account. Off by default.
 */
export async function setBasicAuthEnabled(enabled: boolean): Promise<{ok: boolean; error?: string}> {
    const {error} = await patch<void, {enabled: boolean}>('/auth/basic-auth', {enabled});
    if (error) return {ok: false, error};
    return {ok: true};
}

export async function login(username: string, password: string): Promise<{ok: boolean; error?: string}> {
    const {error} = await post<void, {username: string; password: string}>('/auth/login', {username, password});
    if (error) return {ok: false, error};
    return {ok: true};
}

export async function logout(): Promise<void> {
    await post<void, undefined>('/auth/logout', undefined);
}

export async function changeCredentials(
    currentPassword: string,
    newUsername: string,
    newPassword: string,
): Promise<{ok: boolean; error?: string}> {
    const {error} = await post<void, {currentPassword: string; newUsername: string; newPassword: string}>(
        '/auth/change-credentials',
        {currentPassword, newUsername, newPassword},
    );
    if (error) return {ok: false, error};
    return {ok: true};
}
