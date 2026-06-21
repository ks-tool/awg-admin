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

import { allPages, type PageId } from '@/pages';

interface MainContentProps {
    activeItem: PageId;
}

export default function MainContent({ activeItem }: MainContentProps) {
    const activePage = allPages.find(page => page.id === activeItem);
    if (!activePage) return null;

    const PageComponent = activePage.component;

    return (
        <div className="flex-1 flex flex-col bg-background">
            <div className="flex-1 p-4 sm:p-6 overflow-auto">
                <div className={'max-w-7xl mx-auto text-foreground'}>
                    <PageComponent />
                </div>
            </div>
        </div>
    );
}
