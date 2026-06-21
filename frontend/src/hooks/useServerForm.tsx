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

import {useCallback, useState} from 'react';
import type {ChangeEvent} from 'react';
import type {Server} from '@/types';
import type {ServerInput} from '@/services/servers';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface ServerFormData {
    name: string;
    info: {
        description: string;
        location: string;
        tags: string[];
    };
    ssh: {
        host: string;
        port: number;
        user: string;
        /** Путь к файлу приватного SSH-ключа на машине, где запущен awg-admin. */
        key: string;
        /** Содержимое загруженного файла приватного ключа (если выбран upload, а не путь). */
        keyData: string;
        password: string;
    };
    agent: {
        addr: string;
        tlsEnabled: boolean;
    };
}

export type AuthType = 'key' | 'password';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

export const INITIAL_FORM: ServerFormData = {
    name: '',
    info: {
        description: '',
        location: '',
        tags: [],
    },
    ssh: {
        host: '',
        port: 22,
        user: 'root',
        key: '',
        keyData: '',
        password: '',
    },
    agent: {
        addr: '127.0.0.1:8080',
        tlsEnabled: false,
    },
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Рекурсивно устанавливает значение по точечному пути в объекте */
function setNestedValue<T extends object>(obj: T, path: string, value: unknown): T {
    const keys = path.split('.');
    const root = {...obj} as Record<string, unknown>;
    let current = root;
    for (let i = 0; i < keys.length - 1; i++) {
        const key = keys[i];
        current[key] = current[key] && typeof current[key] === 'object'
            ? {...(current[key] as object)}
            : {};
        current = current[key] as Record<string, unknown>;
    }
    current[keys[keys.length - 1]] = value;
    return root as T;
}

/** Создаёт состояние формы из существующего сервера */
export function serverToFormData(server: Server): ServerFormData {
    return {
        name: server.name ?? '',
        info: {
            description: server.info?.description ?? '',
            location: server.info?.location ?? '',
            tags: server.info?.tags ?? [],
        },
        ssh: {
            host: server.ssh.host ?? '',
            port: server.ssh.port ?? 22,
            user: server.ssh.user ?? 'root',
            key: server.ssh.key ?? '',
            // Never pre-filled, like password — avoids re-sending a
            // multi-KB key on every unrelated save and avoids displaying
            // secret material. Presence is shown separately (see
            // ServerFormFields' hasStoredKey prop) without exposing content.
            keyData: '',
            password: '',
        },
        agent: {
            addr: server.agent?.addr || '127.0.0.1:8080',
            tlsEnabled: Boolean(server.agent?.tls),
        },
    };
}

/**
 * Собирает ServerInput из данных формы.
 *
 * @param formData  — текущее состояние формы
 * @param authType  — тип аутентификации; передаётся только при создании,
 *                    при обновлении можно опустить (будет использован `key` если заполнен)
 * @param existing  — исходный сервер при обновлении (сохраняет TLS-сертификаты и прочие поля)
 */
export function formDataToServerInput(
    formData: ServerFormData,
    authType: AuthType = 'key',
    existing?: Server,
): ServerInput {
    const sshAuth = authType === 'key'
        ? (formData.ssh.keyData.trim()
            ? {keyData: formData.ssh.keyData}
            : formData.ssh.key.trim()
                ? {key: formData.ssh.key}
                : {})
        : formData.ssh.password.trim()
            ? {password: formData.ssh.password}
            : {};

    return {
        name: formData.name,
        info: {
            description: formData.info.description || undefined,
            location: formData.info.location || undefined,
            tags: formData.info.tags.length ? formData.info.tags : (existing?.info?.tags ?? []),
        },
        ssh: {
            host: formData.ssh.host,
            port: formData.ssh.port,
            user: formData.ssh.user || undefined,
            ...sshAuth,
        },
        agent: {
            addr: formData.agent.addr || existing?.agent?.addr || '',
            ...(formData.agent.tlsEnabled
                ? {
                    tls: existing?.agent?.tls ?? {
                        ca: {cert: '', pk: ''},
                        server: {cert: '', pk: ''},
                        client: {cert: '', pk: ''},
                    },
                }
                : {}),
        },
    };
}

// ---------------------------------------------------------------------------
// Hook
// ---------------------------------------------------------------------------

export function useServerForm(initial: ServerFormData = INITIAL_FORM) {
    const [formData, setFormData] = useState<ServerFormData>(initial);

    const updateField = useCallback(
        (e: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
            const {name, value, type} = e.target;
            // DOM input values are always strings regardless of `type` —
            // ssh.port is typed as `number` (sent to the backend as
            // uint16), so a plain string here would round-trip back as
            // e.g. "22567" and fail Go's JSON unmarshal. Number('') is 0,
            // so an emptied field falls back to 0 rather than NaN.
            const val = type === 'checkbox'
                ? (e.target as HTMLInputElement).checked
                : type === 'number'
                    ? Number(value)
                    : value;

            setFormData(prev =>
                name.includes('.')
                    ? setNestedValue(prev, name, val)
                    : {...prev, [name]: val},
            );
        },
        [],
    );

    const resetForm = useCallback((data: ServerFormData = INITIAL_FORM) => {
        setFormData(data);
    }, []);

    return {formData, setFormData, updateField, resetForm};
}
