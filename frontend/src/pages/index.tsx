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

import Dashboard from './Dashboard';
import Servers from './Servers';
import Settings from './Settings';
import Users from './Users';
import AddUser from './AddUser';
import AddServer from './AddServer';
import UserDetail from './UserDetail';
import ServerDetail from './ServerDetail';
import { LayoutDashboard, Server, Users as UsersIcon, Settings as SettingsIcon } from 'lucide-react';

// Pages visible in the sidebar navigation
export const navPages = [
    {
        id: 'dashboard',
        labelKey: 'nav.dashboard',
        icon: LayoutDashboard,
        component: Dashboard,
    },
    {
        id: 'servers',
        labelKey: 'nav.servers',
        icon: Server,
        component: Servers,
    },
    {
        id: 'users',
        labelKey: 'nav.users',
        icon: UsersIcon,
        component: Users,
    },
    {
        id: 'settings',
        labelKey: 'nav.settings',
        icon: SettingsIcon,
        component: Settings,
    },
] as const;

// All pages including hidden ones (not shown in sidebar)
export const allPages = [
    ...navPages,
    {
        id: 'add-user',
        labelKey: 'users.addUser',
        icon: UsersIcon,
        component: AddUser,
    },
    {
        id: 'add-server',
        labelKey: 'servers.addServer',
        icon: Server,
        component: AddServer,
    },
    {
        id: 'user-detail',
        labelKey: 'users.userDetail',
        icon: UsersIcon,
        component: UserDetail,
    },
    {
        id: 'server-detail',
        labelKey: 'servers.serverDetail',
        icon: Server,
        component: ServerDetail,
    },
] as const;

// For backwards compatibility
export const pages = navPages;

export type PageId = typeof allPages[number]['id'];
