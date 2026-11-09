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
import {ThemeProvider, useTheme} from './contexts/ThemeContext';
import {AuthProvider} from './contexts/AuthContext';
import {getCurrentApiMode} from './services/apiMode';
import {getCurrentUser, logout as logoutRequest} from './services/auth';
import Login from './pages/Login';
import Topology from './pages/Topology';

function AppToaster() {
    const {resolvedTheme} = useTheme();
    return <Toaster theme={resolvedTheme} richColors position="bottom-right" />;
}

// Login is an HTTP-mode-only concept — the Wails desktop app talks to Go
// directly via bindings, never over the network, so it has nothing to log
// in to.
const needsAuth = getCurrentApiMode() === 'http';

function App() {
    const [authChecked, setAuthChecked] = useState(!needsAuth);
    const [username, setUsername] = useState<string | null>(null);

    useEffect(() => {
        if (!needsAuth) return;
        getCurrentUser().then(user => {
            setUsername(user?.username ?? null);
            setAuthChecked(true);
        });
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
                <main className="min-h-screen bg-background text-foreground">
                    <Topology />
                </main>
            </AuthProvider>
        </ThemeProvider>
    );
}

export default App;
