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
import { type ReactNode } from 'react'

interface Props {
    title: string
    icon: LucideIcon
    actions?: ReactNode
    description?: string
}

export function PageHeader({ title, icon: Icon, actions, description }: Props) {
    return (
        <div className="flex items-start justify-between gap-4 border-b border-white/5 px-8 py-6">
            <div className="flex items-center gap-3">
                <div className="rounded-lg px-4 py-2 text-sm font-medium transition-colors bg-sky-100 text-sky-700 border border-sky-200 dark:bg-sky-500/15 dark:text-sky-400 dark:border-sky-500/25 dark:hover:bg-sky-500/20">
                    <Icon size={18} />
                </div>
                <div>
                    <h1 className="text-lg font-semibold text-muted-foreground">{title}</h1>
                    {description && <p className="text-xs text-zinc-500">{description}</p>}
                </div>
            </div>
            {actions && <div className="flex items-center gap-2">{actions}</div>}
        </div>
    )
}