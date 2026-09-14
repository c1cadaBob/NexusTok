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
import * as channelsLib from '@/features/channels/lib'
import type { Channel, UpstreamKey } from '@/features/channels/types'

import { ChannelsProvider, useChannels } from '../channels-provider'
import { ChannelTestDialog } from '../dialogs/channel-test-dialog'
import {
  UpstreamKeysMobileList,
  UpstreamKeysSubTable,
} from '../upstream-keys-subtable'

vi.mock('@/features/channels/api', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/channels/api')
  >('@/features/channels/api')
  return {
    ...actual,
    getUpstreamKeys: vi.fn(),
    batchUpdateUpstreamKeyStatus: vi.fn(),
    patchUpstreamKey: vi.fn(),
  }
})

vi.mock('@/features/channels/lib', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/channels/lib')
  >('@/features/channels/lib')
  return {
    ...actual,
    handleTestChannel: vi.fn(
      async (
        _id: number,
        _options: unknown,
        onTestComplete?: (success: boolean, responseTime?: number) => void
      ) => {
        onTestComplete?.(true, 88)
      }
    ),
  }
})

let queryClient: QueryClient

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.mocked(channelsApi.getUpstreamKeys).mockReset()
  vi.mocked(channelsApi.batchUpdateUpstreamKeyStatus).mockReset()
  vi.mocked(channelsApi.patchUpstreamKey).mockReset()
  vi.mocked(channelsLib.handleTestChannel).mockClear()
})

function renderWithProviders(ui: React.ReactNode) {
  return render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>{ui}</ChannelsProvider>
    </QueryClientProvider>
  )
}

function upstreamKey(overrides: Partial<UpstreamKey> = {}): UpstreamKey {
  return {
    id: 7,
    channel_id: 101,
    external_id: 'prod-key',
    name: 'Production key',
    key_preview: 'sk-live...mask',
    models: ['gpt-key-only'],
    key_priority: 9,
    conversion_ratio: 0.1,
    weight: 1900,
    weight_override: null,
    used_quota: 12345,
    remain_quota: 9000,
    expires_at: null,
    status: 1,
    last_sync_at: 1_700_000_000,
    last_used_at: 1_700_100_000,
    ...overrides,
  }
}

function platformChannel(key: UpstreamKey = upstreamKey()): Channel {
  return {
    id: 101,
    name: 'NewAPI upstream',
    type: 60,
    upstream_kind: 'platform_site',
    key: '',
    status: 1,
    models: 'gpt-parent',
    group: 'default',
    upstream_keys: [key],
  } as Channel
}

test('平台站点展开区按密钥级字段展示且不暴露明文密钥', () => {
  const key = upstreamKey()

  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel(key)} />)

  expect(screen.getByText('Key')).toBeInTheDocument()
  expect(screen.getByText('Converted ratio')).toBeInTheDocument()
  expect(screen.getByText('Key priority')).toBeInTheDocument()
  expect(screen.getByText('Key weight')).toBeInTheDocument()
  expect(screen.getByText('Upstream used')).toBeInTheDocument()
  expect(screen.getByText('Last used')).toBeInTheDocument()
  expect(screen.getByText('Production key')).toBeInTheDocument()
  expect(screen.getByText('sk-live...mask')).toBeInTheDocument()
  expect(screen.queryByText('sk-secret-real')).not.toBeInTheDocument()
})

test('平台站点卡片展开区在窄布局中保留密钥字段和操作入口', () => {
  const key = upstreamKey()

  renderWithProviders(
    <UpstreamKeysMobileList channel={platformChannel(key)} />
  )

  expect(screen.getByText('Upstream keys (1)')).toBeInTheDocument()
  expect(screen.getByText('sk-live...mask')).toBeInTheDocument()
  expect(screen.getByText('gpt-key-only')).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Test Connection' })
  ).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Disable' })
  ).toBeInTheDocument()
})

test('密钥级更多菜单不显示渠道级复制和账号池入口', async () => {
  const user = userEvent.setup()

  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel()} />)

  await user.click(screen.getByRole('button', { name: 'Open menu' }))

  expect(await screen.findByText('Test Connection')).toBeInTheDocument()
  expect(screen.getByText('Query Balance')).toBeInTheDocument()
  expect(screen.getByText('Fetch Models')).toBeInTheDocument()
  expect(screen.queryByText('Copy Channel')).not.toBeInTheDocument()
  expect(screen.queryByText('Account pool')).not.toBeInTheDocument()
})

function ChannelTestHarness(props: {
  channel: Channel
  upstreamKey: UpstreamKey
}) {
  const { setCurrentRow, setCurrentUpstreamKey } = useChannels()

  React.useEffect(() => {
    setCurrentRow(props.channel)
    setCurrentUpstreamKey(props.upstreamKey)
  }, [props.channel, props.upstreamKey, setCurrentRow, setCurrentUpstreamKey])

  return <ChannelTestDialog open onOpenChange={() => undefined} />
}

test('平台站点测试弹窗按所选密钥过滤模型并透传 upstream_key_id', async () => {
  const user = userEvent.setup()
  const key = upstreamKey()
  const channel = platformChannel(key)
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [key], total: 1 },
  })

  renderWithProviders(
    <ChannelTestHarness channel={channel} upstreamKey={key} />
  )

  expect(await screen.findByText('Test key')).toBeInTheDocument()
  expect(await screen.findByText('gpt-key-only')).toBeInTheDocument()
  expect(screen.queryByText('gpt-parent')).not.toBeInTheDocument()

  await user.keyboard('{Escape}')

  const testButtons = screen.getAllByRole('button', {
    name: 'Test Connection',
  })
  const testButton = testButtons.at(-1)
  if (!testButton) {
    throw new Error('未找到模型测试按钮')
  }
  await user.click(testButton)

  await waitFor(() => {
    expect(channelsLib.handleTestChannel).toHaveBeenCalledWith(
      101,
      expect.objectContaining({
        testModel: 'gpt-key-only',
        upstreamKeyId: 7,
      }),
      expect.any(Function)
    )
  })
})
