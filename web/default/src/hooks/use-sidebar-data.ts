/*
Copyright (C) 2023-2026 c1cada

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cada.dev
*/
import {
  LayoutDashboard,
  Activity,
  Key,
  FileText,
  Wallet,
  Box,
  Users,
  Ticket,
  User,
  Command,
  Radio,
  FlaskConical,
  MessageSquare,
  CreditCard,
  ListTodo,
  Settings,
  Calculator,
  DatabaseZap,
  ServerCog,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useAuthStore } from '@/stores/auth-store'
import {
  ADMIN_PERMISSION_RESOURCES,
  canReadAdminResource,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { WORKSPACE_IDS } from '@/components/layout/lib/workspace-registry'
import { type SidebarData } from '@/components/layout/types'
import { getAccountPoolSectionNavItems } from '@/features/account-pool/section-registry'

export function useSidebarData(): SidebarData {
  const { t } = useTranslation()
  const user = useAuthStore((state) => state.auth.user)
  const userRole = user?.role
  const canReadChannel = canReadAdminResource(
    user,
    ADMIN_PERMISSION_RESOURCES.CHANNEL
  )
  const canReadAccountPool = canReadAdminResource(
    user,
    ADMIN_PERMISSION_RESOURCES.ACCOUNT_POOL
  )
  const canReadModel = canReadAdminResource(
    user,
    ADMIN_PERMISSION_RESOURCES.MODEL
  )
  const canReadUser = canReadAdminResource(
    user,
    ADMIN_PERMISSION_RESOURCES.USER
  )
  const canReadRedemption = canReadAdminResource(
    user,
    ADMIN_PERMISSION_RESOURCES.REDEMPTION
  )
  const canReadSubscription = canReadAdminResource(
    user,
    ADMIN_PERMISSION_RESOURCES.SUBSCRIPTION
  )

  return {
    workspaces: [
      {
        id: WORKSPACE_IDS.DEFAULT,
        name: '', // 动态读取系统名称
        logo: Command,
        plan: '', // 动态读取系统版本
      },
    ],
    navGroups: [
      {
        id: 'chat',
        title: t('Third-party'),
        items: [
          {
            title: t('Playground'),
            url: '/playground',
            icon: FlaskConical,
          },
          {
            title: t('Third-party'),
            icon: MessageSquare,
            type: 'chat-presets',
          },
        ],
      },
      {
        id: 'general',
        title: t('General'),
        items: [
          {
            title: t('Overview'),
            url: '/dashboard/overview',
            icon: Activity,
          },
          {
            title: t('Dashboard'),
            url: '/dashboard/models',
            icon: LayoutDashboard,
          },
          {
            title: t('API Keys'),
            url: '/keys',
            icon: Key,
          },
          {
            title: t('Usage Logs'),
            url: '/usage-logs/common',
            icon: FileText,
          },
          {
            title: t('Task Logs'),
            url: '/usage-logs/task',
            activeUrls: ['/usage-logs/drawing'],
            configUrls: ['/usage-logs/drawing', '/usage-logs/task'],
            icon: ListTodo,
          },
        ],
      },
      {
        id: 'personal',
        title: t('Personal'),
        items: [
          {
            title: t('Wallet'),
            url: '/wallet',
            icon: Wallet,
          },
          {
            title: t('Profile'),
            url: '/profile',
            icon: User,
          },
        ],
      },
      {
        id: 'admin',
        title: t('Admin'),
        items: [
          ...(canReadChannel
            ? [
                {
                  title: t('Upstream Channels'),
                  url: '/channels',
                  icon: Radio,
                },
              ]
            : []),
          ...(canReadAccountPool
            ? [
                {
                  title: t('Account Pool'),
                  activeUrls: ['/account-pool'],
                  icon: DatabaseZap,
                  items: getAccountPoolSectionNavItems(t),
                },
              ]
            : []),
          ...(canReadModel
            ? [
                {
                  title: t('Models'),
                  url: '/models/metadata',
                  icon: Box,
                },
              ]
            : []),
          ...(userRole === ROLE.SUPER_ADMIN
            ? [
                {
                  title: t('Group & Tool Pricing'),
                  url: '/pricing-settings',
                  icon: Calculator,
                },
              ]
            : []),
          ...(canReadUser
            ? [
                {
                  title: t('Users'),
                  url: '/users',
                  icon: Users,
                },
              ]
            : []),
          ...(canReadSubscription
            ? [
                {
                  title: t('Subscription Management'),
                  url: '/subscriptions',
                  icon: CreditCard,
                },
              ]
            : []),
          ...(canReadRedemption
            ? [
                {
                  title: t('Redemption Codes'),
                  url: '/redemption-codes',
                  icon: Ticket,
                },
              ]
            : []),
          ...(userRole === ROLE.SUPER_ADMIN
            ? [
                {
                  title: t('System Settings'),
                  url: '/system-settings/site',
                  activeUrls: ['/system-settings'],
                  icon: Settings,
                },
                {
                  title: t('System Info'),
                  url: '/system-info',
                  icon: ServerCog,
                },
              ]
            : []),
        ],
      },
    ],
  }
}
