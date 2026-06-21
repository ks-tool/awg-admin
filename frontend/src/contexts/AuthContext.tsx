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

interface AuthContextType {
    /** False in Wails desktop (bindings) mode — there's no login at all there. */
    enabled: boolean;
    username: string | null;
    logout: () => void;
}

const AuthContext = createContext<AuthContextType>({
    enabled: false,
    username: null,
    logout: () => {},
});

export function AuthProvider({ children, value }: { children: ReactNode; value: AuthContextType }) {
    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
    return useContext(AuthContext);
}
