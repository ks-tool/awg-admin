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

import { useTheme, type Theme } from '@/contexts/ThemeContext';
import { Sun, Moon, Monitor } from 'lucide-react';
import { useState, useRef, useEffect } from 'react';
import * as React from "react";
import { cn } from '@/lib/utils';

interface ThemeSwitcherProps {
    collapsed: boolean;
}

export function ThemeSwitcher({ collapsed }: ThemeSwitcherProps) {
    const { theme, setTheme, resolvedTheme } = useTheme();
    const [isColapsed, setIsColapsed] = useState(false);
    const dropdownRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        function handleClickOutside(event: MouseEvent) {
            if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
                setIsColapsed(false);
            }
        }

        if (isColapsed) {
            document.addEventListener('mousedown', handleClickOutside);
            return () => document.removeEventListener('mousedown', handleClickOutside);
        }
    }, [isColapsed]);

    if (collapsed) {
        return null;
    }

    const themes: { value: Theme; icon: React.ReactNode }[] = [
        { value: 'light', icon: <Sun size={16} /> },
        { value: 'dark', icon: <Moon size={16} /> },
        { value: 'system', icon: <Monitor size={16} /> },
    ];

    const currentResolvedIcon = resolvedTheme === 'dark' ? <Moon size={16} /> : <Sun size={16} />;

    return (
        <div className="relative" ref={dropdownRef}>
            <button
                onClick={() => setIsColapsed(!isColapsed)}
                className="flex-1 flex items-center gap-2 px-3 py-2 hover:bg-muted rounded-md transition-colors text-foreground text-sm justify-center"
            >
                {currentResolvedIcon}
            </button>

            {isColapsed && (
                <div className="absolute bottom-full left-0 right-0 mb-2 bg-popover border border-border rounded-md shadow-lg z-50">
                    {themes.map((t) => (
                        <button
                            key={t.value}
                            onClick={() => {
                                setTheme(t.value);
                                setIsColapsed(false);
                            }}
                            className={cn(
                                'w-full flex items-center gap-2 px-3 py-2 text-sm transition-colors justify-center',
                                theme === t.value
                                    ? 'bg-primary text-primary-foreground'
                                    : 'hover:bg-muted text-foreground'
                            )}
                        >
                            {t.icon}
                        </button>
                    ))}
                </div>
            )}
        </div>
    );
}
