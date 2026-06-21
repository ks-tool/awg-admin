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

export type ApiMode = 'http' | 'bindings';

// Check if Wails runtime is available
export const isWailsAvailable = (): boolean => {
    return typeof window !== 'undefined' && !!(window as any)['go'];
};

/**
 * Get hardcoded API mode based on build environment
 * - If Wails runtime is available → use bindings (wails build)
 * - Otherwise → use HTTP API (go build or development)
 */
export const getApiMode = (): ApiMode => {
    return isWailsAvailable() ? 'bindings' : 'http';
};

// Get available API modes based on environment (for UI purposes only)
export const getAvailableApiModes = (): ApiMode[] => {
    return isWailsAvailable() ? ['bindings', 'http'] : ['http'];
};

// Re-checked on every call rather than cached at module-load time: in
// `wails dev`, the Wails JS runtime injects `window.go` asynchronously after
// the page loads, so a value cached when this module first evaluated could
// permanently miss it and get stuck on 'http' for the whole session.
export const getCurrentApiMode = (): ApiMode => getApiMode();
