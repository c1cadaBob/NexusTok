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

For commercial licensing, please contact support@c1cadabob.dev
*/
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as React from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import * as channelsApi from '@/features/channels/api'
import type { Channel } from '@/features/channels/types'

import { ChannelsProvider, useChannels } from '../channels-provider'
import { MultiKeyManageDialog } from '../dialogs/multi-key-manage-dialog'

vi.mock('@/features/channels/api', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/channels/api')
  >('@/features/channels/api')
  return {
    ...actual,
    getMultiKeyStatus: vi.fn(),
    updateMultiKeySettings: vi.fn(),
  }
})

let queryClient: QueryClient

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.mocked(channelsApi.getMultiKeyStatus).mockReset()
  vi.mocked(channelsApi.updateMultiKeySettings).mockReset()
})

function multiKeyChannel(): Channel {
  return {
    id: 101,
    name: 'Multi key channel',
    type: 1,
    upstream_kind: 'key_channel',
    key: '',
    status: 1,
    models: 'gpt-test',
    group: 'default',
    channel_info: {
      is_multi_key: true,
      multi_key_size: 1,
      multi_key_polling_index: 0,
      multi_key_mode: 'random',
    },
  } as Channel
}

function MultiKeyHarness(props: { channel: Channel }) {
  const { setCurrentRow } = useChannels()

  React.useEffect(() => {
    setCurrentRow(props.channel)
  }, [props.channel, setCurrentRow])

  return <MultiKeyManageDialog open onOpenChange={() => undefined} />
}

function renderWithProviders(ui: React.ReactNode) {
  return render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>{ui}</ChannelsProvider>
    </QueryClientProvider>
  )
}

test('多密钥管理表展示全局 key_id 并用 key_id 保存编辑', async () => {
  const user = userEvent.setup()
  vi.mocked(channelsApi.getMultiKeyStatus).mockResolvedValue({
    success: true,
    data: {
      keys: [
        {
          index: 0,
          key_id: 1201,
          status: 1,
          key_preview: 'sk-live...',
          key_priority: 7,
          weight: 300,
          auto_weight: 200,
          health_status: 'degraded',
        },
      ],
      total: 1,
      page: 1,
      page_size: 10,
      total_pages: 1,
      enabled_count: 1,
      manual_disabled_count: 0,
      auto_disabled_count: 0,
    },
  })
  vi.mocked(channelsApi.updateMultiKeySettings).mockResolvedValue({
    success: true,
  })

  renderWithProviders(<MultiKeyHarness channel={multiKeyChannel()} />)

  expect(await screen.findByText('#1201')).toBeInTheDocument()
  expect(screen.getByText('7')).toBeInTheDocument()
  expect(screen.getByText('Degraded')).toBeInTheDocument()
  expect(screen.getByText('300')).toBeInTheDocument()
  expect(screen.getByText('Override')).toBeInTheDocument()
  expect(screen.queryByText('300 (200)')).not.toBeInTheDocument()

  await user.click(screen.getByRole('button', { name: 'Edit' }))
  const priorityInput = await screen.findByLabelText('Key priority')
  const weightInput = screen.getByLabelText('Weight override')
  await user.clear(priorityInput)
  await user.type(priorityInput, '12')
  await user.clear(weightInput)
  await user.type(weightInput, '450')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => {
    expect(channelsApi.updateMultiKeySettings).toHaveBeenCalledWith(
      101,
      0,
      1201,
      {
        key_priority: 12,
        weight_override: 450,
      }
    )
  })
})
