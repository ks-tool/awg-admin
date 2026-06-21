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

import {useCallback, useEffect, useState} from 'react';
import {Toaster} from 'sonner';
import Sidebar from './components/layout/Sidebar';
import MainContent from './components/layout/MainContent';
import {type PageId, allPages, pages} from './pages';
import { ThemeProvider, useTheme } from './contexts/ThemeContext';
import { NavigationProvider } from './contexts/NavigationContext';
import { AuthProvider } from './contexts/AuthContext';
import { getCurrentApiMode } from './services/apiMode';
import { getCurrentUser, logout as logoutRequest } from './services/auth';
import Login from './pages/Login';

const STORAGE_KEY = 'activeMenuItem';
const SIDEBAR_COLLAPSED_KEY = 'sidebarCollapsed';

function getStoredMenuItem(): PageId {
    const stored = sessionStorage.getItem(STORAGE_KEY);
    if (stored && allPages.some(page => page.id === stored)) {
        return stored as PageId;
    }
    return pages[0].id;
}

function getStoredSidebarState(): boolean {
    const stored = localStorage.getItem(SIDEBAR_COLLAPSED_KEY);
    return stored ? JSON.parse(stored) : false;
}

function AppToaster() {
    const {resolvedTheme} = useTheme();
    return <Toaster theme={resolvedTheme} richColors position="bottom-right" />;
}

// Login is an HTTP-mode-only concept — the Wails desktop app talks to Go
// directly via bindings, never over the network, so it has nothing to log
// in to.
const needsAuth = getCurrentApiMode() === 'http';

function App() {
    const [activeMenuItem, setActiveMenuItem] = useState<PageId>(getStoredMenuItem);
    const [sidebarCollapsed, setSidebarCollapsed] = useState(getStoredSidebarState);
    const [authChecked, setAuthChecked] = useState(!needsAuth);
    const [username, setUsername] = useState<string | null>(null);

    useEffect(() => {
        if (!needsAuth) return;
        getCurrentUser().then(user => {
            setUsername(user?.username ?? null);
            setAuthChecked(true);
        });
    }, []);

    useEffect(() => {
        sessionStorage.setItem(STORAGE_KEY, activeMenuItem);
    }, [activeMenuItem]);

    useEffect(() => {
        localStorage.setItem(SIDEBAR_COLLAPSED_KEY, JSON.stringify(sidebarCollapsed));
    }, [sidebarCollapsed]);

    const handleToggleSidebarCollapse = () => {
        setSidebarCollapsed(prev => !prev);
    };

    const handleSelectItem = useCallback((item: PageId) => {
        setActiveMenuItem(item);
    }, []);

    const handleLogout = useCallback(() => {
        void logoutRequest();
        setUsername(null);
    }, []);

    if (needsAuth && !authChecked) {
        return null;
    }

    if (needsAuth && !username) {
        return <Login onSuccess={setUsername} />;
    }

    return (
        <ThemeProvider>
            <AppToaster />
            <AuthProvider value={{enabled: needsAuth, username, logout: handleLogout}}>
                <NavigationProvider navigate={handleSelectItem}>
                    <div className="min-h-screen flex flex-col bg-background text-foreground">
                        <div className="flex flex-1 relative">
                            <Sidebar
                                activeItem={activeMenuItem}
                                onSelectItem={handleSelectItem}
                                collapsed={sidebarCollapsed}
                                onToggleSidebar={handleToggleSidebarCollapse}
                            />
                            <main className="flex-1 flex flex-col min-w-0">
                                <MainContent
                                    activeItem={activeMenuItem}
                                />
                            </main>
                        </div>
                    </div>
                </NavigationProvider>
            </AuthProvider>
        </ThemeProvider>
    );
}

export default App;
