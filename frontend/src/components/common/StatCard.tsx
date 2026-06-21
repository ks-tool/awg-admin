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

import { type LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

interface Props {
  label: string
  value: string | number
  sub?: string
  icon: LucideIcon
  accent?: 'emerald' | 'sky' | 'violet' | 'amber' | 'rose'
}

const accents = {
  emerald: 'text-emerald-700 dark:text-emerald-400 bg-emerald-100 dark:bg-emerald-400/10 border-emerald-200 dark:border-emerald-400/20',
  sky:     'text-sky-700 dark:text-sky-400 bg-sky-100 dark:bg-sky-400/10 border-sky-200 dark:border-sky-400/20',
  violet:  'text-violet-700 dark:text-violet-400 bg-violet-100 dark:bg-violet-400/10 border-violet-200 dark:border-violet-400/20',
  amber:   'text-amber-700 dark:text-amber-400 bg-amber-100 dark:bg-amber-400/10 border-amber-200 dark:border-amber-400/20',
  rose:    'text-rose-700 dark:text-rose-400 bg-rose-100 dark:bg-rose-400/10 border-rose-200 dark:border-rose-400/20',
}

export function StatCard({ label, value, sub, icon: Icon, accent = 'sky' }: Props) {
  return (
    <div className="group relative overflow-hidden rounded-xl border border-border bg-card p-5 backdrop-blur-sm transition-all hover:border-primary/30 hover:bg-card dark:border-white/5 dark:bg-white/3 dark:hover:border-white/10 dark:hover:bg-white/5">
      <div className="absolute inset-0 from-white/1 to-transparent dark:from-white/1 dark:to-transparent" />
      <div className="relative flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <p className="mb-2 text-xs font-medium uppercase tracking-widest text-muted-foreground">{label}</p>
          <p className="font-mono text-3xl font-bold text-foreground tabular-nums dark:text-zinc-100">{value}</p>
          {sub && <p className="mt-1 text-xs text-muted-foreground">{sub}</p>}
        </div>
        <div className={cn('shrink-0 rounded-lg border p-2.5', accents[accent])}>
          <Icon size={18} />
        </div>
      </div>
    </div>
  )
}
