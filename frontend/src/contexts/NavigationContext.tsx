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

import { createContext, type ReactNode, useContext } from 'react';

export type PageId = 'dashboard' | 'servers' | 'users' | 'settings' | 'add-user' | 'add-server' | 'user-detail' | 'server-detail';

interface NavigationContextType {
    navigate: (pageId: PageId) => void;
}

const NavigationContext = createContext<NavigationContextType | undefined>(undefined);

export function NavigationProvider({ children, navigate }: { children: ReactNode; navigate: (pageId: PageId) => void }) {
    return (
        <NavigationContext.Provider value={{ navigate }}>
            {children}
        </NavigationContext.Provider>
    );
}

export function useNavigation() {
    const context = useContext(NavigationContext);
    if (!context) {
        throw new Error('useNavigation must be used within a NavigationProvider');
    }
    return context;
}
