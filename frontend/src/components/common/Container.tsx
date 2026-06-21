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

import * as React from "react";

interface Props {
    title: React.ReactNode
    action?: React.ReactNode
    children: React.ReactNode
}

export function Container({title, action, children}: Props) {
    return (
        <div className="rounded-xl border border-border bg-card p-6 dark:border-white/5 dark:bg-white/3">
            <div className="flex items-center justify-between mb-4">
                <h2 className="text-lg font-semibold text-foreground dark:text-zinc-100">{title}</h2>
                {action}
            </div>
            {children}
        </div>
    );
}
