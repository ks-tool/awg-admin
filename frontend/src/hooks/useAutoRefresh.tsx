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

import {useEffect, useRef} from "react";

export function useAutoRefresh(fn: () => void | Promise<void>, ms = 30000) {
    const running = useRef(false);

    useEffect(() => {
        let cancelled = false;

        const tick = async () => {
            if (running.current) return;
            running.current = true;
            try {
                await fn();
            } finally {
                running.current = false;
            }
        };

        void tick();
        const id = setInterval(() => {
            if (!cancelled) void tick();
        }, ms);
        return () => {
            cancelled = true;
            clearInterval(id);
        };
    }, [fn, ms]);
}
