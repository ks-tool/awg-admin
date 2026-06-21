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

import { Search } from 'lucide-react'
import { useTranslation } from 'react-i18next'

interface Props {
  value: string
  onChange: (v: string) => void
}

export function SearchBar({ value, onChange }: Props) {
  const { t } = useTranslation()
  return (
    <div className="relative">
      <Search size={14} className="absolute left-3 top-1/2 -translate-y-1/2 text-zinc-500" />
      <input
        type="text"
        value={value}
        onChange={e => onChange(e.target.value)}
        placeholder={t('common.search')}
        className="h-9 w-64 rounded-lg border border-white/8 bg-white/4 pl-9 pr-3 text-sm text-zinc-200 placeholder-zinc-600 outline-none transition focus:border-white/15 focus:bg-white/6"
      />
    </div>
  )
}
