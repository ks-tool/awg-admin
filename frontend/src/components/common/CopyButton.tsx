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

import { useState } from 'react'
import { Copy, Check } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'

interface Props {
  value: string
  className?: string
}

export function CopyButton({ value, className }: Props) {
  const { t } = useTranslation()
  const [copied, setCopied] = useState(false)

  const handleCopy = async (e: React.MouseEvent) => {
    e.stopPropagation()
    await navigator.clipboard.writeText(value)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  return (
    <button
      onClick={handleCopy}
      title={copied ? t('common.copied') : t('common.copy')}
      className={cn(
        'rounded p-1 transition-colors',
        copied
          ? 'text-emerald-600 dark:text-emerald-400'
          : 'text-muted-foreground hover:text-foreground dark:text-zinc-600 dark:hover:text-zinc-300',
        className,
      )}
    >
      {copied ? <Check size={12} /> : <Copy size={12} />}
    </button>
  )
}
