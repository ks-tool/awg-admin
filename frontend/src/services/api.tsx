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

// Determine API base URL based on environment
// In development: use localhost:18989 (backend server)
// In production (Wails): use current origin (embedded backend)
const getApiBaseUrl = (): string => {
    if (import.meta.env.VITE_API_BASE_URL) {
        return import.meta.env.VITE_API_BASE_URL;
    }

    // For Wails app (production), use same origin
    if (window.location.protocol === 'http:' && window.location.hostname === 'localhost') {
        // Check if this is a Wails app by checking for wails runtime
        // In Wails, the frontend is embedded and serves from the same origin
        if (window.location.port === '3000' || !window.location.port) {
            return window.location.origin;
        }
    }

    // For development, use the backend API server
    return 'http://localhost:8080';
};

const API_BASE_URL = getApiBaseUrl();

interface ApiResponse<T> {
    data: T;
    error?: string;
}

// readErrorBody returns the response body text to use as the error message.
// The backend's error responses are plain text (see internal/api/error.go),
// not JSON — used as-is so substring markers like SSH_PASSPHRASE_REQUIRED
// survive the round trip, matching what bindings mode already preserves via
// the raw Go error string.
async function readErrorBody(response: Response, status: number): Promise<string> {
    const text = await response.text().catch(() => '');
    return text || `HTTP error! status: ${status}`;
}

export async function get<T>(
    endpoint: string,
    options?: RequestInit
): Promise<ApiResponse<T>> {
    try {
        const {headers: extraHeaders, ...restOptions} = options ?? {};
        const response = await fetch(`${API_BASE_URL}${endpoint}`, {
            method: 'GET',
            headers: {
                'Content-Type': 'application/json',
                ...extraHeaders,
            },
            // Carries the session cookie set by /auth/login — needed for
            // `npm run dev` where the frontend (:5173) and backend (:8080)
            // are different origins; same-origin deployments send it by
            // default anyway, so this is harmless there.
            credentials: 'include',
            ...restOptions,
        });

        if (!response.ok && response.status !== 204) {
            throw new Error(await readErrorBody(response, response.status));
        }

        const text = await response.text();
        const data: T = text ? JSON.parse(text) : (null as T);
        return {data};
    } catch (error) {
        console.error(`API GET ${endpoint} failed:`, error);
        return {
            data: null as T,
            error: error instanceof Error ? error.message : 'Unknown error',
        };
    }
}

export async function post<T, B = unknown>(
    endpoint: string,
    body: B,
    options?: RequestInit
): Promise<ApiResponse<T>> {
    try {
        const {headers: extraHeaders, ...restOptions} = options ?? {};
        const response = await fetch(`${API_BASE_URL}${endpoint}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                ...extraHeaders,
            },
            body: JSON.stringify(body),
            credentials: 'include',
            ...restOptions,
        });

        if (!response.ok && response.status !== 204) {
            throw new Error(await readErrorBody(response, response.status));
        }

        const text = await response.text();
        const data: T = text ? JSON.parse(text) : (null as T);
        return {data};
    } catch (error) {
        console.error(`API POST ${endpoint} failed:`, error);
        return {
            data: null as T,
            error: error instanceof Error ? error.message : 'Unknown error',
        };
    }
}

export async function put<T, B = unknown>(
    endpoint: string,
    body: B,
    options?: RequestInit
): Promise<ApiResponse<T>> {
    try {
        const {headers: extraHeaders, ...restOptions} = options ?? {};
        const response = await fetch(`${API_BASE_URL}${endpoint}`, {
            method: 'PUT',
            headers: {
                'Content-Type': 'application/json',
                ...extraHeaders,
            },
            body: JSON.stringify(body),
            credentials: 'include',
            ...restOptions,
        });

        if (!response.ok && response.status !== 204) {
            throw new Error(await readErrorBody(response, response.status));
        }

        const text = await response.text();
        const data: T = text ? JSON.parse(text) : (null as T);
        return {data};
    } catch (error) {
        console.error(`API PUT ${endpoint} failed:`, error);
        return {
            data: null as T,
            error: error instanceof Error ? error.message : 'Unknown error',
        };
    }
}

export async function patch<T, B = unknown>(
    endpoint: string,
    body: B,
    options?: RequestInit
): Promise<ApiResponse<T>> {
    try {
        const {headers: extraHeaders, ...restOptions} = options ?? {};
        const response = await fetch(`${API_BASE_URL}${endpoint}`, {
            method: 'PATCH',
            headers: {
                'Content-Type': 'application/json',
                ...extraHeaders,
            },
            body: JSON.stringify(body),
            credentials: 'include',
            ...restOptions,
        });

        if (!response.ok && response.status !== 204) {
            throw new Error(await readErrorBody(response, response.status));
        }

        const text = await response.text();
        const data: T = text ? JSON.parse(text) : (null as T);
        return {data};
    } catch (error) {
        console.error(`API PATCH ${endpoint} failed:`, error);
        return {
            data: null as T,
            error: error instanceof Error ? error.message : 'Unknown error',
        };
    }
}

export async function remove<T>(
    endpoint: string,
    options?: RequestInit
): Promise<ApiResponse<T>> {
    try {
        const {headers: extraHeaders, ...restOptions} = options ?? {};
        const response = await fetch(`${API_BASE_URL}${endpoint}`, {
            method: 'DELETE',
            headers: {
                'Content-Type': 'application/json',
                ...extraHeaders,
            },
            // Carries the session cookie set by /auth/login — needed for
            // `npm run dev` where the frontend (:5173) and backend (:8080)
            // are different origins; same-origin deployments send it by
            // default anyway, so this is harmless there.
            credentials: 'include',
            ...restOptions,
        });

        if (!response.ok && response.status !== 204) {
            throw new Error(await readErrorBody(response, response.status));
        }

        const data: T = response.status === 204 ? (null as T) : await response.json();
        return {data};
    } catch (error) {
        console.error(`API DELETE ${endpoint} failed:`, error);
        return {
            data: null as T,
            error: error instanceof Error ? error.message : 'Unknown error',
        };
    }
}

// ============================================================================
// Adaptive API Functions - Route to HTTP or Bindings based on current mode
// ============================================================================

import {getCurrentApiMode} from './apiMode';

// Export functions that are used in service layer
export const adaptiveApi = {
    isUsingBindings: () => getCurrentApiMode() === 'bindings',
    getApiMode: getCurrentApiMode,
};

