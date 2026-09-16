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
  const key: UpstreamKey = {
    id: 7,
    channel_id: 101,
    external_id: 'prod-key',
    name: 'Production key',
    key_preview: 'sk-live...mask',
    models: ['gpt-key-only'],
    models_synced: true,
    key_priority: 9,
    source_conversion_ratio: 1,
    conversion_ratio: 0.1,
    conversion_ratio_override: null,
    weight: 1900,
    auto_weight: 1900,
    weight_override: null,
    status: 1,
    last_sync_at: 1_700_000_000,
    routable: true,
  }
  return { ...key, ...overrides }
}

function platformChannel(
  key: UpstreamKey = upstreamKey(),
  status: Channel['upstream_site_status'] = {
    channel_id: 101,
    platform: 'newapi',
    base_url: 'https://upstream.example',
    auth_type: 'password',
    recharge_amount: 1,
    credited_amount: 10,
    conversion_ratio: 0.1,
    balance: 10,
    used_quota: 0,
    balance_updated_time: 1_700_000_000,
    sync_status: 'success',
    last_sync_at: 1_700_000_000,
    consecutive_failures: 0,
  }
): Channel {
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
    upstream_site_status: status,
  } as Channel
}

test('平台站点展开区按密钥级字段展示且不暴露明文密钥', () => {
  const key = upstreamKey()

  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel(key)} />)

  expect(screen.getByText('Key')).toBeInTheDocument()
  expect(screen.getByText('Converted ratio')).toBeInTheDocument()
  expect(screen.getByText('Key priority')).toBeInTheDocument()
  expect(screen.getByText('Key weight')).toBeInTheDocument()
  expect(screen.getByText('Last sync')).toBeInTheDocument()
  expect(screen.queryByText('Upstream used')).not.toBeInTheDocument()
  expect(screen.getByText('Production key')).toBeInTheDocument()
  expect(screen.getByText('sk-live...mask')).toBeInTheDocument()
  expect(screen.queryByText('sk-secret-real')).not.toBeInTheDocument()
})

test('平台站点卡片展开区在窄布局中保留密钥字段和操作入口', () => {
  const key = upstreamKey()

  renderWithProviders(<UpstreamKeysMobileList channel={platformChannel(key)} />)

  expect(screen.getByText('Upstream keys (1)')).toBeInTheDocument()
  expect(screen.getByText('sk-live...mask')).toBeInTheDocument()
  expect(screen.getByText('gpt-key-only')).toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Test Connection' })
  ).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Disable' })).toBeInTheDocument()
})

test('未开启倍率覆盖时保存子密钥会清除覆盖而不是写入有效倍率', async () => {
  const user = userEvent.setup()
  vi.mocked(channelsApi.patchUpstreamKey).mockResolvedValue({
    success: true,
    data: upstreamKey(),
  })

  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel()} />)

  await user.click(screen.getByRole('button', { name: 'Edit' }))
  await user.click(await screen.findByRole('button', { name: 'Save' }))

  await waitFor(() => {
    expect(channelsApi.patchUpstreamKey).toHaveBeenCalledWith(
      101,
      7,
      expect.objectContaining({
        key_priority: 9,
        clear_conversion_ratio: true,
      })
    )
  })
  const payload = vi.mocked(channelsApi.patchUpstreamKey).mock.calls[0]?.[2]
  expect(payload).not.toHaveProperty('conversion_ratio')
})

test('开启倍率覆盖时保存子密钥会写入最终倍率覆盖', async () => {
  const user = userEvent.setup()
  vi.mocked(channelsApi.patchUpstreamKey).mockResolvedValue({
    success: true,
    data: upstreamKey({ conversion_ratio: 0.05 }),
  })

  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel()} />)

  await user.click(screen.getByRole('button', { name: 'Edit' }))
  await user.click(
    await screen.findByRole('switch', { name: 'Override conversion ratio' })
  )
  const ratioInput = screen.getByLabelText('Conversion ratio')
  await user.clear(ratioInput)
  await user.type(ratioInput, '0.05')
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => {
    expect(channelsApi.patchUpstreamKey).toHaveBeenCalledWith(
      101,
      7,
      expect.objectContaining({
        key_priority: 9,
        conversion_ratio: 0.05,
      })
    )
  })
  const payload = vi.mocked(channelsApi.patchUpstreamKey).mock.calls[0]?.[2]
  expect(payload).not.toHaveProperty('clear_conversion_ratio')
})

test('子密钥编辑弹窗展示上游原始倍率和覆盖状态', async () => {
  const user = userEvent.setup()
  const key = upstreamKey({
    source_conversion_ratio: 0.5,
    conversion_ratio: 0.2,
    conversion_ratio_override: 0.2,
  })

  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel(key)} />)

  await user.click(screen.getByRole('button', { name: 'Edit' }))

  expect(await screen.findByText('Source ratio')).toBeInTheDocument()
  expect(screen.getByText('0.5')).toBeInTheDocument()
  expect(
    screen.getByRole('switch', { name: 'Override conversion ratio' })
  ).toBeChecked()
})

test('平台站点子密钥支持批量禁用选中项', async () => {
  const user = userEvent.setup()
  vi.mocked(channelsApi.batchUpdateUpstreamKeyStatus).mockResolvedValue({
    success: true,
    data: { updated: 1 },
  })

  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel()} />)

  await user.click(
    screen.getByRole('checkbox', { name: 'Select upstream key' })
  )
  await user.click(
    screen.getByRole('button', { name: 'Disable selected keys' })
  )

  await waitFor(() => {
    expect(channelsApi.batchUpdateUpstreamKeyStatus).toHaveBeenCalledWith(
      101,
      [7],
      2
    )
  })
})

test('平台站点子密钥部分选中时全选框展示半选状态', async () => {
  const user = userEvent.setup()
  const firstKey = upstreamKey()
  const secondKey = upstreamKey({ id: 8, external_id: 'backup-key' })
  const channel = {
    ...platformChannel(firstKey),
    upstream_keys: [firstKey, secondKey],
  }

  renderWithProviders(<UpstreamKeysSubTable channel={channel} />)

  const rowCheckboxes = screen.getAllByRole('checkbox', {
    name: 'Select upstream key',
  })
  const firstCheckbox = rowCheckboxes[0]
  if (!firstCheckbox) {
    throw new Error('未找到子密钥选择框')
  }
  await user.click(firstCheckbox)

  const selectAllCheckboxes = screen.getAllByRole('checkbox', {
    name: 'Select all upstream keys',
  })
  for (const checkbox of selectAllCheckboxes) {
    expect(checkbox).toHaveAttribute('aria-checked', 'mixed')
  }
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
  upstreamKey?: UpstreamKey | null
}) {
  const { setCurrentRow, setCurrentUpstreamKey } = useChannels()

  React.useEffect(() => {
    setCurrentRow(props.channel)
    setCurrentUpstreamKey(props.upstreamKey ?? null)
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

test('平台站点首次同步失败时不展示旧子密钥模型', async () => {
  const key = upstreamKey()
  const channel = platformChannel(key, {
    channel_id: 101,
    platform: 'newapi',
    base_url: 'https://upstream.example',
    auth_type: 'password',
    recharge_amount: 1,
    credited_amount: 10,
    conversion_ratio: 0.1,
    balance: 10,
    used_quota: 0,
    balance_updated_time: 1_700_000_000,
    sync_status: 'failed',
    last_sync_at: 0,
    consecutive_failures: 1,
  })
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [key], total: 1 },
  })

  renderWithProviders(<ChannelTestHarness channel={channel} />)

  expect(
    await screen.findByText(
      'Please sync this platform site successfully before testing.'
    )
  ).toBeInTheDocument()
  expect(screen.queryByText('gpt-key-only')).not.toBeInTheDocument()
})

test('平台站点测试弹窗遇到禁用当前密钥时回退自动路由并仅显示可路由模型', async () => {
  const user = userEvent.setup()
  const disabledKey = upstreamKey({
    id: 7,
    name: 'Disabled key',
    models: ['gpt-disabled'],
    status: 3,
    disabled_reason: '上游密钥模型能力读取失败',
    routable: false,
  })
  const routableKey = upstreamKey({
    id: 8,
    name: 'Routable key',
    models: ['gpt-live'],
    status: 1,
    routable: true,
  })
  const channel = {
    ...platformChannel(disabledKey),
    models: 'gpt-parent,gpt-disabled',
    upstream_keys: [disabledKey, routableKey],
  } as Channel
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [disabledKey, routableKey], total: 2 },
  })

  renderWithProviders(
    <ChannelTestHarness channel={channel} upstreamKey={disabledKey} />
  )

  expect(await screen.findByDisplayValue('Auto route')).toBeInTheDocument()
  expect(await screen.findByText('gpt-live')).toBeInTheDocument()
  expect(screen.queryByText('gpt-parent')).not.toBeInTheDocument()
  expect(screen.queryByText('gpt-disabled')).not.toBeInTheDocument()

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
        testModel: 'gpt-live',
        upstreamKeyId: undefined,
      }),
      expect.any(Function)
    )
  })
})

test('平台站点测试弹窗在从未成功同步时提示先同步成功', async () => {
  const disabledKey = upstreamKey({
    models: ['gpt-disabled'],
    status: 3,
    disabled_reason: '上游密钥读取失败',
    routable: false,
  })
  const channel = {
    ...platformChannel(disabledKey),
    models: 'gpt-parent',
    upstream_site_status: {
      channel_id: 101,
      platform: 'newapi',
      base_url: 'https://upstream.example',
      auth_type: 'password',
      recharge_amount: 1,
      credited_amount: 10,
      conversion_ratio: 0.1,
      balance: 0,
      used_quota: 0,
      balance_updated_time: 0,
      sync_status: 'failed',
      last_sync_at: 0,
      consecutive_failures: 1,
      key_count: 1,
      routable_key_count: 0,
    },
    upstream_keys: [disabledKey],
  } as Channel
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [disabledKey], total: 1 },
  })

  renderWithProviders(<ChannelTestHarness channel={channel} />)

  expect(
    await screen.findAllByText(
      'Please sync this platform site successfully before testing.'
    )
  ).not.toHaveLength(0)
  expect(screen.queryByText('gpt-parent')).not.toBeInTheDocument()
})

test('平台站点测试弹窗在刷新失败时继续使用最近一次成功快照', async () => {
  const routableKey = upstreamKey({
    models: ['gpt-live'],
    status: 1,
    routable: true,
  })
  const channel = {
    ...platformChannel(routableKey),
    models: 'gpt-live',
    upstream_site_status: {
      channel_id: 101,
      platform: 'newapi',
      base_url: 'https://upstream.example',
      auth_type: 'password',
      recharge_amount: 1,
      credited_amount: 10,
      conversion_ratio: 0.1,
      balance: 1,
      used_quota: 2,
      balance_updated_time: 1_700_000_000,
      sync_status: 'failed',
      last_sync_at: 1_700_000_000,
      consecutive_failures: 1,
      key_count: 1,
      routable_key_count: 1,
    },
    upstream_keys: [routableKey],
  } as Channel
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [routableKey], total: 1 },
  })

  renderWithProviders(<ChannelTestHarness channel={channel} />)

  expect(
    await screen.findByText(
      'Current refresh failed; using the last successful snapshot.'
    )
  ).toBeInTheDocument()
  expect(screen.getByText('gpt-live')).toBeInTheDocument()
  expect(
    screen.queryByText(
      'Please sync this platform site successfully before testing.'
    )
  ).not.toBeInTheDocument()
})
