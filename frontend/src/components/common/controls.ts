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

export const inputs = {
    primary: "w-full rounded-lg border border-input bg-input px-4 py-2 text-foreground placeholder-muted-foreground/50 focus:border-sky-500/50 focus:outline-none transition-colors dark:border-white/10 dark:bg-white/5 dark:text-zinc-100 dark:placeholder-zinc-600/50 dark:focus:bg-white/10",
};

export const buttons = {
    primary: "rounded-lg px-6 py-2 text-sm font-medium bg-sky-100 text-sky-700 border border-sky-200 hover:bg-sky-200 dark:bg-sky-500/15 dark:text-sky-400 dark:border-sky-500/25 dark:hover:bg-sky-500/20 disabled:opacity-50 disabled:cursor-not-allowed",
    secondary: "rounded-lg px-6 py-2 text-sm font-medium transition-colors bg-muted text-muted-foreground border border-border hover:bg-secondary dark:bg-zinc-500/15 dark:text-zinc-400 dark:border-zinc-500/25 dark:hover:bg-zinc-500/20 disabled:opacity-50 disabled:cursor-not-allowed",
    danger: "rounded-lg px-6 py-2 text-sm font-medium transition-colors bg-red-100 text-red-700 border border-red-200 hover:bg-red-200 dark:bg-red-500/15 dark:text-red-400 dark:border-red-500/25 disabled:opacity-50 disabled:cursor-not-allowed",
};
