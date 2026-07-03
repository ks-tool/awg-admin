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

import {useState, type ReactNode} from 'react';
import {ChevronRight} from 'lucide-react';
import {cn} from '@/lib/utils';

/**
 * A labelled disclosure: a toggle row with a chevron that reveals/hides its
 * children. Used in the interface and peer forms to keep only the required
 * fields visible by default and tuck every optional/advanced field behind a
 * single "expand" affordance. The children stay mounted only while open, but
 * form state lives in the parent, so collapsing never resets the values.
 */
export function CollapsibleSection({
    label,
    defaultOpen = false,
    children,
}: {
    label: string;
    defaultOpen?: boolean;
    children: ReactNode;
}) {
    const [open, setOpen] = useState(defaultOpen);
    return (
        <div className="border-t border-border pt-2 dark:border-white/10">
            <button
                type="button"
                onClick={() => setOpen(o => !o)}
                aria-expanded={open}
                className="flex w-full items-center gap-1.5 py-1 text-sm font-medium text-muted-foreground transition-colors hover:text-foreground dark:text-zinc-400 dark:hover:text-zinc-200"
            >
                <ChevronRight size={16} className={cn('transition-transform', open && 'rotate-90')}/>
                {label}
            </button>
            {open && <div className="space-y-4 pt-3">{children}</div>}
        </div>
    );
}
