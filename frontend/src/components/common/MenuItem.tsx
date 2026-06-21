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

import type {ReactNode} from 'react';
import type {LucideIcon} from 'lucide-react';
import { cn } from '@/lib/utils';

interface MenuItemProps {
    active: boolean;
    onClick: () => void;
    children: ReactNode;
    icon?: LucideIcon;
    collapsed?: boolean;
}

export function MenuItem({active, onClick, children, icon: Icon, collapsed}: MenuItemProps) {
    const label = typeof children === 'string' ? children : '';

    return (
        <button
            onClick={onClick}
            className={cn(
                'w-full flex items-center gap-3 p-3 rounded-lg font-medium transition-all duration-200',
                active
                    ? 'shadow-md border border-sky-200 dark:border-sky-500/25 text-sky-700 dark:text-sky-400 bg-sky-100 dark:bg-sky-500/10'
                    : 'text-muted-foreground hover:bg-sidebar-accent/20 hover:text-sidebar-accent-foreground',
                collapsed && 'justify-center'
            )}
            title={collapsed ? label : ''}
        >
            {Icon && <Icon size={20} />}
            <span className={cn('transition-opacity', collapsed && 'opacity-0 hidden')}>{children}</span>
        </button>
    );
}
