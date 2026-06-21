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

import { useTranslation } from 'react-i18next';
import { pages, type PageId } from '@/pages';
import { MenuItem as MenuItemComponent } from '@/components/common/MenuItem';
import { ThemeSwitcher } from '@/components/common/ThemeSwitcher';
import { ChevronLeft, LogOut } from 'lucide-react';
import { cn } from '@/lib/utils';
import { useAuth } from '@/contexts/AuthContext';

interface SidebarProps {
    activeItem: PageId;
    onSelectItem: (item: PageId) => void;
    collapsed: boolean;
    onToggleSidebar: () => void;
}

export default function Sidebar({ activeItem, onSelectItem, collapsed, onToggleSidebar }: SidebarProps) {
    const { t } = useTranslation();
    const { enabled: authEnabled, username, logout } = useAuth();

    return (
        <aside id="sidebar" className={cn('bg-sidebar text-sidebar-foreground shadow-xl flex flex-col border-r border-sidebar-border transition-all duration-300 ease-in-out', collapsed ? 'w-16' : 'w-64')}>
            <nav className="py-2 flex-1">
                {pages.map((page) => (
                    <MenuItemComponent
                        key={page.id}
                        active={activeItem === page.id}
                        onClick={() => onSelectItem(page.id)}
                        icon={page.icon}
                        collapsed={collapsed}
                    >
                        {t(page.labelKey)}
                    </MenuItemComponent>
                ))}
            </nav>
            {authEnabled && (
                <div className={cn('px-4 py-2 border-b border-border', collapsed ? 'flex justify-center' : 'flex items-center justify-between gap-2')}>
                    {!collapsed && (
                        <span className="text-xs text-muted-foreground truncate" title={username ?? undefined}>
                            {username}
                        </span>
                    )}
                    <button
                        onClick={logout}
                        className="p-1.5 hover:bg-muted rounded-md transition-colors text-foreground shrink-0"
                        title={t('auth.logout')}
                    >
                        <LogOut size={16} />
                    </button>
                </div>
            )}
            <div className="flex items-center justify-between p-4 border-b border-border">
                <button
                    onClick={onToggleSidebar}
                    className="p-1.5 hover:bg-muted rounded-md transition-colors text-foreground"
                >
                    <ChevronLeft
                        size={20}
                        className={cn('transition-transform duration-300', collapsed && 'rotate-180')}
                    />
                </button>
                {!collapsed && <ThemeSwitcher collapsed={collapsed} />}
            </div>
        </aside>
    );
}
