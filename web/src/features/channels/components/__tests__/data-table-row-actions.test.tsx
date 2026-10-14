/*
Copyright (C) 2023-2026 c1cadaBob

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
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { afterEach, describe, expect, test, vi } from 'vitest'

import type { Channel } from '@/features/channels/types'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { DataTableRowActions } from '../data-table-row-actions'

vi.mock('../channels-provider', () => ({
  useChannels: () => ({
    setOpen: vi.fn(),
    setCurrentRow: vi.fn(),
    setCurrentUpstreamKey: vi.fn(),
    upstream: {
      openModal: vi.fn(),
      detectChannelUpdates: vi.fn(),
    },
  }),
}))

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Open menu': 'Open menu',
        'Open upstream platform': 'Open upstream platform',
        Edit: 'Edit',
        'Test Connection': 'Test Connection',
        'Test Channel Connection': 'Test Channel Connection',
        Disable: 'Disable',
        Enable: 'Enable',
        'Query Balance': 'Query Balance',
        'Fetch Models': 'Fetch Models',
        'Copy Channel': 'Copy Channel',
        Delete: 'Delete',
        'Delete Channel': 'Delete Channel',
        'Are you sure you want to delete channel "{{name}}"? This action cannot be undone.':
          'Are you sure you want to delete channel "{{name}}"? This action cannot be undone.',
        'No permission to perform this action':
          'No permission to perform this action',
      },
    },
  },
})

const channel = {
  id: 42,
  type: 1,
  upstream_kind: 'platform_site',
  key_priority: null,
  conversion_ratio: null,
  key_weight_override: null,
  key: '',
  openai_organization: null,
  test_model: null,
  status: 1,
  name: 'platform-site',
  weight: 1,
  created_time: 0,
  test_time: 0,
  response_time: 0,
  base_url: null,
  other: '',
  balance: 0,
  balance_updated_time: 0,
  models: '',
  model_ratio: null,
  group: 'default',
  used_quota: 0,
  model_mapping: null,
  status_code_mapping: null,
  priority: 0,
  auto_ban: 0,
  other_info: '',
  tag: null,
  setting: null,
  param_override: null,
  header_override: null,
  remark: '',
  max_input_tokens: 0,
  channel_info: {
    is_multi_key: false,
    multi_key_size: 0,
    multi_key_polling_index: 0,
    multi_key_mode: 'random',
  },
  settings: '{}',
  upstream_site_status: {
    channel_id: 42,
    platform: 'sub2api',
    base_url: 'https://upstream.example',
    management_url: 'https://upstream.example/admin/dashboard',
    auth_type: 'access_token',
    recharge_amount: 0,
    credited_amount: 0,
    conversion_ratio: 1,
    balance: 0,
    used_quota: 0,
    balance_updated_time: 0,
    sync_status: 'success',
    last_sync_at: 1,
    consecutive_failures: 0,
  },
} satisfies Channel

function renderActions(value: Channel = channel) {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <I18nextProvider i18n={i18n}>
        <DataTableRowActions
          row={{ original: value } as never}
        />
      </I18nextProvider>
    </QueryClientProvider>
  )
}

afterEach(() => {
  useAuthStore.getState().auth.setUser(null)
  vi.restoreAllMocks()
})

describe('channel row upstream platform action', () => {
  test('lets an administrator open the validated management URL in a new tab', async () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'admin',
      role: ROLE.ADMIN,
    })
    const open = vi.spyOn(window, 'open').mockImplementation(() => null)
    const user = userEvent.setup()

    renderActions()
    await user.click(screen.getByRole('button', { name: 'Open menu' }))
    await user.click(screen.getByText('Open upstream platform'))

    expect(open).toHaveBeenCalledWith(
      'https://upstream.example/admin/dashboard',
      '_blank',
      'noopener,noreferrer'
    )
  })

  test('hides the upstream platform action from non-administrators', async () => {
    useAuthStore.getState().auth.setUser({
      id: 2,
      username: 'user',
      role: ROLE.USER,
    })
    const user = userEvent.setup()

    renderActions()
    await user.click(screen.getByRole('button', { name: 'Open menu' }))

    expect(screen.queryByText('Open upstream platform')).not.toBeInTheDocument()
  })

  test('hides the action when the server has no safe management URL', async () => {
    useAuthStore.getState().auth.setUser({
      id: 1,
      username: 'admin',
      role: ROLE.ADMIN,
    })
    const user = userEvent.setup()

    renderActions({
      ...channel,
      upstream_site_status: {
        ...channel.upstream_site_status,
        management_url: undefined,
      },
    })
    await user.click(screen.getByRole('button', { name: 'Open menu' }))

    expect(screen.queryByText('Open upstream platform')).not.toBeInTheDocument()
  })
})
