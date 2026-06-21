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

type Status = string | 'enabled' | 'disabled'

interface Props {
  status: Status
  size?: 'sm' | 'md'
}

const config: Record<Status, { dot: string; text: string; bg: string }> = {
  online:   { dot: 'bg-emerald-400 animate-pulse', text: 'text-emerald-600 dark:text-emerald-400', bg: 'bg-emerald-100 border-emerald-200 dark:bg-emerald-400/10 dark:border-emerald-400/20' },
  offline:  { dot: 'bg-gray-500 dark:bg-zinc-500',  text: 'text-gray-600 dark:text-zinc-400',    bg: 'bg-gray-100 border-gray-200 dark:bg-zinc-500/10 dark:border-zinc-500/20' },
  degraded: { dot: 'bg-amber-400 animate-pulse',   text: 'text-amber-700 dark:text-amber-400',   bg: 'bg-amber-100 border-amber-200 dark:bg-amber-400/10 dark:border-amber-400/20' },
  unknown:  { dot: 'bg-gray-500 dark:bg-zinc-600', text: 'text-gray-600 dark:text-zinc-500',    bg: 'bg-gray-100 border-gray-200 dark:bg-zinc-600/10 dark:border-zinc-600/20' },
  active:   { dot: 'bg-emerald-400 animate-pulse', text: 'text-emerald-600 dark:text-emerald-400', bg: 'bg-emerald-100 border-emerald-200 dark:bg-emerald-400/10 dark:border-emerald-400/20' },
  inactive: { dot: 'bg-gray-500 dark:bg-zinc-500', text: 'text-gray-600 dark:text-zinc-400',    bg: 'bg-gray-100 border-gray-200 dark:bg-zinc-500/10 dark:border-zinc-500/20' },
  error:    { dot: 'bg-red-500 animate-pulse',     text: 'text-red-700 dark:text-red-400',     bg: 'bg-red-100 border-red-200 dark:bg-red-500/10 dark:border-red-500/20' },
  enabled:  { dot: 'bg-sky-400 dark:bg-sky-400',   text: 'text-sky-700 dark:text-sky-400',     bg: 'bg-sky-100 border-sky-200 dark:bg-sky-400/10 dark:border-sky-400/20' },
  disabled: { dot: 'bg-gray-500 dark:bg-zinc-600', text: 'text-gray-600 dark:text-zinc-500',    bg: 'bg-gray-100 border-gray-200 dark:bg-zinc-600/10 dark:border-zinc-600/20' },
}

export function StatusBadge({ status, size = 'md' }: Props) {
  const { t } = useTranslation()
  const c = config[status] || config.unknown

  const label = (() => {
    if (status === 'enabled') return t('common.enabled')
    if (status === 'disabled') return t('common.disabled')
    return t(`status.${status}`)
  })()

  return (
    <span className={cn(
      'inline-flex items-center gap-1.5 rounded-full border font-mono font-medium tracking-wide',
      c.bg,
      size === 'sm' ? 'px-1.5 py-0.5 text-[10px]' : 'px-2.5 py-1 text-xs',
    )}>
      <span className={cn('rounded-full', c.dot, size === 'sm' ? 'size-1.5' : 'size-2')} />
      <span className={c.text}>{label}</span>
    </span>
  )
}
