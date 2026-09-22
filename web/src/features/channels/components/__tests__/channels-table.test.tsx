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
import type { ColumnDef } from '@tanstack/react-table'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, expect, test, vi } from 'vitest'

import * as channelsApi from '@/features/channels/api'
import type { Channel, UpstreamKey } from '@/features/channels/types'

import { ChannelsProvider } from '../channels-provider'
import { ChannelsTable } from '../channels-table'

const routeSearch: Record<string, unknown> = {
  page: 1,
  pageSize: 20,
  filter: '',
  status: [],
  type: [],
  group: [],
  model: '',
}
const navigate = vi.fn()

vi.mock('@tanstack/react-router', async () => {
  const actual = await vi.importActual<typeof import('@tanstack/react-router')>(
    '@tanstack/react-router'
  )
  return {
    ...actual,
    getRouteApi: vi.fn(() => ({
      useSearch: () => routeSearch,
      useNavigate: () => navigate,
    })),
  }
})

vi.mock('@/features/channels/api', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/channels/api')
  >('@/features/channels/api')
  return {
    ...actual,
    getChannels: vi.fn(),
    getEnabledModels: vi.fn(),
    getGroups: vi.fn(),
    getUpstreamKeys: vi.fn(),
    getUpstreamSiteStatus: vi.fn(),
    searchChannels: vi.fn(),
  }
})

vi.mock('../channel-card', () => ({
  ChannelCard: () => null,
}))

vi.mock('../channels-columns', () => ({
  useChannelsColumns: ({
    modelRatioSortActive,
  }: {
    modelRatioSortActive: boolean
  }): ColumnDef<Channel>[] => [
    {
      accessorKey: 'name',
      header: 'Name',
      cell: ({ row }) => (
        <button
          type='button'
          aria-label='Expand upstream keys'
          aria-expanded={row.getIsExpanded()}
          onClick={row.getToggleExpandedHandler()}
        >
          {row.original.name}
        </button>
      ),
    },
    {
      id: 'model_ratio',
      accessorKey: 'model_ratio',
      header: 'Minimum ratio',
      enableSorting: true,
      cell: () => (
        <span data-testid='ratio-mode'>
          {modelRatioSortActive ? 'ratio-sort' : 'normal-sort'}
        </span>
      ),
    },
    {
      id: 'actions',
      header: 'Actions',
      cell: () => null,
    },
  ],
}))

vi.mock('../upstream-keys-subtable', () => ({
  UpstreamKeysSubTable: ({ channel }: { channel: Channel }) => (
    <div data-testid='upstream-keys'>
      {(channel.upstream_keys || []).map((key) => key.name).join(',')}
    </div>
  ),
}))

let queryClient: QueryClient

function channel(overrides: Partial<Channel> = {}): Channel {
  return {
    id: 1,
    name: 'ratio channel',
    type: 1,
    upstream_kind: 'platform_site',
    key: '',
    status: 1,
    models: 'gpt-4o',
    group: 'default',
    priority: 0,
    balance: 0,
    used_quota: 0,
    response_time: 0,
    test_time: 0,
    created_time: 0,
    balance_updated_time: 0,
    ...overrides,
  } as Channel
}

function upstreamKey(name: string, models: string[]): UpstreamKey {
  return {
    id: name === 'key-a' ? 11 : 12,
    key_id: name === 'key-a' ? 101 : 102,
    channel_id: 1,
    external_id: name,
    name,
    key_preview: `${name}-preview`,
    models,
    allowed_models: null,
    models_synced: true,
    key_priority: 0,
    source_conversion_ratio: 1,
    conversion_ratio: name === 'key-a' ? 0.4 : 0.8,
    conversion_ratio_override: null,
    weight: 1000,
    auto_weight: 1000,
    weight_override: null,
    status: 1,
    last_sync_at: 1,
    last_used_at: 1,
    routable: true,
    health_status: 'normal',
    health_sample_count: 5,
    health_success_count: 5,
    health_first_latency_ms: 100,
  }
}

function renderTable() {
  localStorage.setItem('channels:view-mode', 'table')

  return render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <ChannelsTable />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  vi.clearAllMocks()
  localStorage.removeItem('channels:upstream-expanded:v1')
  Object.assign(routeSearch, {
    page: 1,
    pageSize: 20,
    filter: '',
    status: [],
    type: [],
    group: [],
    model: '',
  })
  vi.mocked(channelsApi.getEnabledModels).mockResolvedValue({
    success: true,
    data: ['gpt-4o', 'claude-3'],
  })
  vi.mocked(channelsApi.getGroups).mockResolvedValue({
    success: true,
    data: [],
  })
  vi.mocked(channelsApi.getChannels).mockResolvedValue({
    success: true,
    data: {
      items: [channel()],
      total: 1,
      page: 1,
      page_size: 20,
      type_counts: { '1': 1 },
    },
  })
  vi.mocked(channelsApi.searchChannels).mockResolvedValue({
    success: true,
    data: {
      items: [channel()],
      total: 1,
      type_counts: { '1': 1 },
    },
  })
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: {
      items: [
        upstreamKey('key-a', ['gpt-4o']),
        upstreamKey('key-b', ['claude-3']),
      ],
      total: 2,
    },
  })
  vi.mocked(channelsApi.getUpstreamSiteStatus).mockResolvedValue({
    success: true,
    data: undefined,
  })
})

test('选择模型后开启最低倍率排序会把模型传给渠道列表接口', async () => {
  const user = userEvent.setup()
  renderTable()

  const modelInput = await screen.findByRole('combobox', {
    name: 'Filter by model...',
  })
  await user.click(modelInput)
  await user.click(screen.getByRole('option', { name: 'gpt-4o' }))

  await waitFor(() => {
    expect(channelsApi.searchChannels).toHaveBeenLastCalledWith(
      expect.objectContaining({ model: 'gpt-4o' })
    )
  })

  const ratioHeader = screen.getByRole('button', { name: /Minimum ratio/ })
  await user.click(ratioHeader)
  await user.click(screen.getByRole('menuitem', { name: 'Asc' }))

  await waitFor(() => {
    expect(channelsApi.getChannels).toHaveBeenLastCalledWith(
      expect.objectContaining({
        model: 'gpt-4o',
        sort_by: 'model_ratio',
        sort_order: 'asc',
      })
    )
  })
})

test('模型选择器允许输入候选列表之外的模型并在清空后恢复未指定模型', async () => {
  const user = userEvent.setup()
  renderTable()

  const modelInput = await screen.findByRole('combobox', {
    name: 'Filter by model...',
  })
  await user.type(modelInput, 'custom-model')

  await waitFor(() => {
    expect(channelsApi.searchChannels).toHaveBeenLastCalledWith(
      expect.objectContaining({ model: 'custom-model' })
    )
  })

  await user.clear(modelInput)

  await waitFor(() => {
    expect(channelsApi.getChannels).toHaveBeenLastCalledWith(
      expect.not.objectContaining({ model: expect.anything() })
    )
  })
})

test('最低倍率排序激活时不会用指定模型过滤子密钥列表', async () => {
  const user = userEvent.setup()
  renderTable()

  await screen.findByRole('combobox', { name: 'Filter by model...' })
  await waitFor(() =>
    expect(channelsApi.getUpstreamKeys).toHaveBeenCalledWith(1)
  )
  await user.click(
    await screen.findByRole('button', { name: 'Expand upstream keys' })
  )
  await waitFor(() =>
    expect(screen.getByTestId('upstream-keys')).toHaveTextContent('key-a,key-b')
  )

  const modelInput = screen.getByRole('combobox', {
    name: 'Filter by model...',
  })
  await user.click(modelInput)
  await user.click(screen.getByRole('option', { name: 'gpt-4o' }))

  const ratioHeader = screen.getByRole('button', { name: /Minimum ratio/ })
  await user.click(ratioHeader)
  await user.click(screen.getByRole('menuitem', { name: 'Asc' }))

  await waitFor(() =>
    expect(screen.getByTestId('upstream-keys')).toHaveTextContent('key-a,key-b')
  )
})

test('进入渠道页时忽略历史展开状态并默认折叠', async () => {
  localStorage.setItem('channels:upstream-expanded:v1', JSON.stringify([1]))
  renderTable()

  expect(
    await screen.findByRole('button', { name: 'Expand upstream keys' })
  ).toHaveAttribute('aria-expanded', 'false')
  expect(screen.queryByTestId('upstream-keys')).not.toBeInTheDocument()
})
