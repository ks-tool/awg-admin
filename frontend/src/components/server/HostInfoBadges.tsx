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

import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import type { HostInfo } from '@/types'

const tones = {
    sky:     'bg-sky-100 border-sky-200 text-sky-700 dark:bg-sky-400/10 dark:border-sky-400/20 dark:text-sky-400',
    violet:  'bg-violet-100 border-violet-200 text-violet-700 dark:bg-violet-400/10 dark:border-violet-400/20 dark:text-violet-400',
    emerald: 'bg-emerald-100 border-emerald-200 text-emerald-700 dark:bg-emerald-400/10 dark:border-emerald-400/20 dark:text-emerald-400',
    zinc:    'bg-gray-100 border-gray-200 text-gray-600 dark:bg-zinc-500/10 dark:border-zinc-500/20 dark:text-zinc-400',
} as const

function Pill({ tone, children }: { tone: keyof typeof tones; children: React.ReactNode }) {
    return (
        <span className={cn('inline-flex items-center rounded-full border px-1.5 py-0.5 text-[10px] font-mono font-medium tracking-wide', tones[tone])}>
            {children}
        </span>
    )
}

// kindLabel shortens the agent's interface-kind identifiers for the compact
// dashboard cell: "amneziawg" → "awg", "wireguard" → "wg" (anything else shown
// verbatim).
function kindLabel(kind: string): string {
    if (kind === 'amneziawg') return 'awg'
    if (kind === 'wireguard') return 'wg'
    return kind
}

/**
 * HostInfoBadges renders a server agent's HostInfo (see agent.HostInfo, fetched
 * via getServerHostInfo) as a compact row of pills for the dashboard: the
 * backend, a "docker" pill when the agent runs in a container, and one pill per
 * creatable interface kind (AmneziaWG highlighted). Full detail — host Docker
 * availability, kernel-module presence, version — is in the cell's hover title.
 * Renders "—" until the first successful fetch (agent unreachable / no data).
 */
export function HostInfoBadges({ info }: { info: HostInfo | null | undefined }) {
    const { t } = useTranslation()

    if (!info) {
        return <span className="text-zinc-600 dark:text-zinc-500">—</span>
    }

    const yn = (b: boolean) => (b ? t('common.yes') : t('common.no'))
    const title = [
        `${t('servers.hostInfo.backend')}: ${info.backend}`,
        info.version ? `${t('servers.hostInfo.version')}: ${info.version}` : null,
        `${t('servers.hostInfo.dockerHost')}: ${yn(info.docker)}`,
        `${t('servers.hostInfo.container')}: ${yn(info.inDocker)}`,
        info.backend === 'kernel' ? `${t('servers.hostInfo.kernelModule')}: ${yn(info.kernelModule)}` : null,
        `${t('servers.hostInfo.interfaceKinds')}: ${(info.interfaceKinds ?? []).join(', ') || '—'}`,
    ].filter(Boolean).join('\n')

    return (
        <div className="flex items-center gap-1 whitespace-nowrap" title={title}>
            <Pill tone="sky">{info.backend}</Pill>
            {info.inDocker && <Pill tone="violet">docker</Pill>}
            {(info.interfaceKinds ?? []).map((kind) => (
                <Pill key={kind} tone={kind === 'amneziawg' ? 'emerald' : 'zinc'}>
                    {kindLabel(kind)}
                </Pill>
            ))}
        </div>
    )
}
