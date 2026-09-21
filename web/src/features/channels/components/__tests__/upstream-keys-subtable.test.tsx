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
import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import * as React from 'react'
import { beforeEach, expect, test, vi } from 'vitest'

import * as channelsApi from '@/features/channels/api'
import * as channelsLib from '@/features/channels/lib'
import type { Channel, UpstreamKey } from '@/features/channels/types'

import { ChannelsProvider, useChannels } from '../channels-provider'
import { ChannelTestDialog } from '../dialogs/channel-test-dialog'
import { FetchModelsDialog } from '../dialogs/fetch-models-dialog'
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
    fetchUpstreamModels: vi.fn(),
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
  vi.mocked(channelsApi.fetchUpstreamModels).mockReset()
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
  const id = overrides.id ?? 7
  const key: UpstreamKey = {
    id,
    key_id: overrides.key_id ?? id + 1000,
    channel_id: 101,
    external_id: 'prod-key',
    name: 'Production key',
    key_preview: 'sk-live...mask',
    models: ['gpt-key-only'],
    allowed_models: null,
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

test('平台站点子密钥操作列固定在表格右侧', () => {
  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel()} />)

  const actionHeader = screen.getByRole('columnheader', { name: 'Actions' })
  expect(actionHeader.className).toContain('sticky')
  expect(actionHeader.className).toContain('right-0')

  const editButton = screen.getByRole('button', { name: 'Edit' })
  const actionCell = editButton.closest('td')
  expect(actionCell).not.toBeNull()
  expect(actionCell?.className).toContain('sticky')
  expect(actionCell?.className).toContain('right-0')
})

test('平台站点子表根区域锚定到渠道表可视宽度', () => {
  const tableContainer = document.createElement('div')
  tableContainer.setAttribute('data-slot', 'data-table-scroll-container')
  Object.defineProperty(tableContainer, 'clientWidth', {
    configurable: true,
    value: 640,
  })

  const view = render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <UpstreamKeysSubTable channel={platformChannel()} />
      </ChannelsProvider>
    </QueryClientProvider>,
    { container: tableContainer }
  )

  const actionHeader = view.getByRole('columnheader', { name: 'Actions' })
  const subTableRoot = actionHeader.closest('table')?.parentElement?.parentElement

  expect(subTableRoot).not.toBeNull()
  expect(subTableRoot).toHaveClass(
    'relative',
    'max-w-none',
    'min-w-0',
    'overflow-visible'
  )
  expect(subTableRoot).not.toHaveClass('sticky', 'left-0')
  expect(subTableRoot).toHaveAttribute(
    'style',
    expect.stringContaining('width: 640px')
  )
  expect(actionHeader.closest('table')).toHaveClass('w-max', 'min-w-full')
})

test('平台站点展开子表时批量操作栏固定在内部滚动区域顶部', () => {
  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel()} />)

  const enableButton = screen.getByRole('button', {
    name: 'Enable selected keys',
  })
  const toolbar = enableButton.closest('div.sticky')

  expect(toolbar).not.toBeNull()
  expect(toolbar).toHaveClass(
    'sticky',
    'top-0',
    'left-0',
    'w-full',
    'min-w-full',
    'z-20'
  )
  expect(toolbar).not.toHaveClass('min-w-max')
})

test('平台站点桌面子表复用渠道滚动容器而不创建独立横向滚动层', () => {
  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel()} />)

  const actionHeader = screen.getByRole('columnheader', { name: 'Actions' })
  const subTableRoot = actionHeader.closest('table')?.parentElement?.parentElement

  expect(subTableRoot).not.toBeNull()
  expect(subTableRoot).toHaveClass('overflow-visible')
  expect(subTableRoot).toHaveClass('relative')
  expect(subTableRoot).not.toHaveClass('sticky', 'left-0')
  expect(subTableRoot).not.toHaveClass('overflow-auto')

  const internalContainer = actionHeader.closest('table')?.parentElement
  expect(internalContainer).not.toBeNull()
  expect(internalContainer).toHaveClass('!overflow-visible')
  expect(internalContainer).not.toHaveClass('overflow-auto')
})

test('平台站点空子密钥列表不显示批量操作栏', () => {
  renderWithProviders(
    <UpstreamKeysSubTable
      channel={{ ...platformChannel(), upstream_keys: [] }}
    />
  )

  expect(
    screen.queryByRole('button', { name: 'Enable selected keys' })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Disable selected keys' })
  ).not.toBeInTheDocument()
})

test('平台站点子表使用分体表头的渠道滚动容器同步批量工具栏', async () => {
  const tableContainer = document.createElement('div')
  tableContainer.setAttribute('data-slot', 'data-table-scroll-container')
  Object.defineProperty(tableContainer, 'clientWidth', {
    configurable: true,
    value: 640,
  })
  const addEventListener = vi.spyOn(tableContainer, 'addEventListener')
  tableContainer.getBoundingClientRect = () =>
    ({
      left: 100,
      right: 740,
      width: 640,
      top: 0,
      bottom: 800,
      height: 800,
      x: 100,
      y: 0,
      toJSON: () => ({}),
    }) as DOMRect

  const view = render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <UpstreamKeysSubTable channel={platformChannel()} />
      </ChannelsProvider>
    </QueryClientProvider>,
    { container: tableContainer }
  )

  const enableButton = view.getByRole('button', {
    name: 'Enable selected keys',
  })
  const toolbar = enableButton.closest('div.sticky')

  expect(toolbar).not.toBeNull()
  expect(toolbar).toHaveClass('sticky', 'top-0', 'left-0', 'w-full')
  expect(toolbar).not.toHaveClass('min-w-max')

  const toolbarViewport = toolbar?.parentElement
  expect(toolbarViewport).not.toBeNull()
  if (!toolbarViewport) {
    return
  }
  expect(addEventListener).toHaveBeenCalledWith(
    'scroll',
    expect.any(Function),
    expect.objectContaining({ passive: true })
  )

  const subTableRoot = toolbarViewport.parentElement
  expect(subTableRoot).not.toBeNull()
  subTableRoot?.style.setProperty('padding-left', '0px')
  subTableRoot?.style.setProperty('border-left-width', '0px')

  toolbarViewport.getBoundingClientRect = () =>
    ({
      left: 0,
      right: 640,
      width: 640,
      top: 0,
      bottom: 44,
      height: 44,
      x: 0,
      y: 0,
      toJSON: () => ({}),
    }) as DOMRect

  const scrollCall = addEventListener.mock.calls.find(
    ([eventName]) => eventName === 'scroll'
  )
  const scrollListener = scrollCall?.[1]
  expect(typeof scrollListener).toBe('function')

  await act(async () => {
    if (typeof scrollListener === 'function') {
      scrollListener(new Event('scroll'))
    }
  })

  expect(toolbarViewport.style.transform).toBe('translateX(100px)')
})

test('平台站点卡片展开区在窄布局中保留密钥字段和操作入口', () => {
  const key = upstreamKey()

  renderWithProviders(<UpstreamKeysMobileList channel={platformChannel(key)} />)

  expect(screen.getByText('Upstream keys (1)')).toBeInTheDocument()
  const enableButton = screen.getByRole('button', {
    name: 'Enable selected keys',
  })
  const toolbar = enableButton.closest('div.sticky')
  expect(toolbar).not.toBeNull()
  expect(toolbar).toHaveClass('sticky', 'top-0', 'left-0', 'w-full')
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
        clear_allowed_models: true,
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

function UpstreamKeysWithTestDialogHarness(props: { channel: Channel }) {
  const { open, setOpen } = useChannels()

  return (
    <>
      <UpstreamKeysSubTable channel={props.channel} />
      <ChannelTestDialog
        open={open === 'test-channel'}
        onOpenChange={(nextOpen) => {
          if (!nextOpen) {
            setOpen(null)
          }
        }}
      />
    </>
  )
}

test('从子密钥直接测试按钮进入时测试弹窗默认选择该子密钥', async () => {
  const user = userEvent.setup()
  const key = upstreamKey()
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [key], total: 1 },
  })

  renderWithProviders(
    <UpstreamKeysWithTestDialogHarness channel={platformChannel(key)} />
  )

  await user.click(screen.getByRole('button', { name: 'Test Connection' }))

  expect(await screen.findByDisplayValue('Production key')).toBeInTheDocument()
  expect(await screen.findAllByText('gpt-key-only')).not.toHaveLength(0)
})

test('从子密钥更多菜单进入时测试弹窗默认选择该子密钥', async () => {
  const user = userEvent.setup()
  const key = upstreamKey()
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [key], total: 1 },
  })

  renderWithProviders(
    <UpstreamKeysWithTestDialogHarness channel={platformChannel(key)} />
  )

  await user.click(screen.getByRole('button', { name: 'Open menu' }))
  await user.click(
    await screen.findByRole('menuitem', { name: 'Test Connection' })
  )

  expect(await screen.findByDisplayValue('Production key')).toBeInTheDocument()
  expect(await screen.findAllByText('gpt-key-only')).not.toHaveLength(0)
})

test('平台站点测试弹窗按所选密钥过滤模型并透传全局 key_id', async () => {
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
  expect(screen.queryByRole('listbox')).not.toBeInTheDocument()
  expect(await screen.findByText('gpt-key-only')).toBeInTheDocument()
  expect(screen.queryByText('gpt-parent')).not.toBeInTheDocument()

  await user.click(screen.getByRole('combobox', { name: 'Test key' }))
  expect(await screen.findByRole('listbox')).toBeInTheDocument()

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
        keyId: key.key_id,
      }),
      expect.any(Function)
    )
  })
})

test('渠道级获取模型时不传递子密钥 key_id', async () => {
  vi.mocked(channelsApi.fetchUpstreamModels).mockResolvedValue({
    success: true,
    data: ['gpt-parent'],
  })

  renderWithProviders(
    <FetchModelsHarness channel={platformChannel()} upstreamKey={null} />
  )

  await waitFor(() => {
    expect(channelsApi.fetchUpstreamModels).toHaveBeenCalledWith(101, undefined)
  })
})

test('平台站点子密钥编辑支持保存允许模型列表', async () => {
  const user = userEvent.setup()
  const key = upstreamKey({
    models: ['gpt-key-only', 'gpt-secondary'],
  })
  vi.mocked(channelsApi.patchUpstreamKey).mockResolvedValue({
    success: true,
    data: key,
  })

  renderWithProviders(<UpstreamKeysSubTable channel={platformChannel(key)} />)

  await user.click(screen.getByRole('button', { name: 'Edit' }))
  await user.click(
    await screen.findByRole('switch', {
      name: 'Limit which models can be used with this key',
    })
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => {
    expect(channelsApi.patchUpstreamKey).toHaveBeenCalledWith(
      101,
      7,
      expect.objectContaining({
        allowed_models: ['gpt-key-only', 'gpt-secondary'],
      })
    )
  })
  const payload = vi.mocked(channelsApi.patchUpstreamKey).mock.calls[0]?.[2]
  expect(payload).not.toHaveProperty('clear_allowed_models')
})

test('平台站点测试弹窗只展示子密钥允许使用的模型', async () => {
  const key = upstreamKey({
    models: ['gpt-key-only', 'gpt-restricted'],
    allowed_models: ['gpt-restricted'],
  })
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [key], total: 1 },
  })

  renderWithProviders(
    <ChannelTestHarness channel={platformChannel(key)} upstreamKey={key} />
  )

  expect(await screen.findByText('gpt-restricted')).toBeInTheDocument()
  expect(screen.queryByText('gpt-key-only')).not.toBeInTheDocument()
})

function FetchModelsHarness(props: {
  channel: Channel
  upstreamKey?: UpstreamKey | null
}) {
  const { setCurrentRow, setCurrentUpstreamKey } = useChannels()

  React.useEffect(() => {
    setCurrentRow(props.channel)
    setCurrentUpstreamKey(props.upstreamKey ?? null)
  }, [props.channel, props.upstreamKey, setCurrentRow, setCurrentUpstreamKey])

  return <FetchModelsDialog open onOpenChange={() => undefined} />
}

test('平台站点子密钥获取模型时传递全局 key_id', async () => {
  const key = upstreamKey()
  vi.mocked(channelsApi.fetchUpstreamModels).mockResolvedValue({
    success: true,
    data: ['gpt-key-only'],
  })

  renderWithProviders(
    <FetchModelsHarness channel={platformChannel(key)} upstreamKey={key} />
  )

  await waitFor(() => {
    expect(channelsApi.fetchUpstreamModels).toHaveBeenCalledWith(
      101,
      key.key_id
    )
  })
})

test('平台站点从渠道行进入测试时默认使用自动路由', async () => {
  const user = userEvent.setup()
  const key = upstreamKey()
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [key], total: 1 },
  })

  renderWithProviders(<ChannelTestHarness channel={platformChannel(key)} />)

  expect(await screen.findByDisplayValue('Auto route')).toBeInTheDocument()

  await user.keyboard('{Escape}')
  const testButton = screen
    .getAllByRole('button', { name: 'Test Connection' })
    .at(-1)
  if (!testButton) {
    throw new Error('未找到模型测试按钮')
  }
  await user.click(testButton)

  await waitFor(() => {
    expect(channelsLib.handleTestChannel).toHaveBeenCalledWith(
      101,
      expect.objectContaining({
        testModel: 'gpt-key-only',
        keyId: undefined,
      }),
      expect.any(Function)
    )
  })
})

test('平台站点密钥行入口在异步结果缺少该密钥时仍保留预选', async () => {
  const key = upstreamKey()
  const channel = platformChannel(key)
  channel.upstream_keys = []
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [], total: 0 },
  })

  renderWithProviders(
    <ChannelTestHarness channel={channel} upstreamKey={key} />
  )

  expect(await screen.findByDisplayValue('Production key')).toBeInTheDocument()
  expect(await screen.findByText('gpt-key-only')).toBeInTheDocument()
})

test('平台站点密钥行入口在异步查询返回前后不覆盖入口选择', async () => {
  const key = upstreamKey()
  const refreshedKey = upstreamKey({
    models: ['gpt-key-only', 'gpt-key-refreshed'],
  })
  let resolveKeys: (value: {
    success: true
    data: { items: UpstreamKey[]; total: number }
  }) => void = () => undefined
  vi.mocked(channelsApi.getUpstreamKeys).mockImplementation(
    () =>
      new Promise((resolve) => {
        resolveKeys = resolve
      })
  )

  renderWithProviders(
    <ChannelTestHarness channel={platformChannel(key)} upstreamKey={key} />
  )

  expect(await screen.findByDisplayValue('Production key')).toBeInTheDocument()
  expect(screen.getByText('gpt-key-only')).toBeInTheDocument()

  await act(async () => {
    resolveKeys({
      success: true,
      data: { items: [refreshedKey], total: 1 },
    })
  })

  expect(await screen.findByDisplayValue('Production key')).toBeInTheDocument()
  expect(await screen.findByText('gpt-key-refreshed')).toBeInTheDocument()
})

test('平台站点密钥行入口在子密钥暂缺路由字段时不回退自动路由', async () => {
  const user = userEvent.setup()
  const key = upstreamKey({ routable: undefined })
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [key], total: 1 },
  })

  renderWithProviders(
    <ChannelTestHarness channel={platformChannel(key)} upstreamKey={key} />
  )

  expect(await screen.findByDisplayValue('Production key')).toBeInTheDocument()
  expect(await screen.findByText('gpt-key-only')).toBeInTheDocument()

  const testButton = screen
    .getAllByRole('button', { name: 'Test Connection' })
    .at(-1)
  if (!testButton) {
    throw new Error('未找到模型测试按钮')
  }
  await user.click(testButton)

  await waitFor(() => {
    expect(channelsLib.handleTestChannel).toHaveBeenCalledWith(
      101,
      expect.objectContaining({
        testModel: 'gpt-key-only',
        keyId: key.key_id,
      }),
      expect.any(Function)
    )
  })
})

test('平台站点密钥行入口在同步结果明确缺失当前子密钥时才回退自动路由', async () => {
  const missingKey = upstreamKey({
    availability_reason: 'missing',
    models: ['gpt-missing'],
    routable: false,
  })
  const routableKey = upstreamKey({
    id: 8,
    name: 'Routable key',
    external_id: 'routable-key',
    models: ['gpt-live'],
    routable: true,
  })
  const channel = {
    ...platformChannel(missingKey),
    upstream_keys: [missingKey, routableKey],
  } as Channel
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [routableKey], total: 1 },
  })

  renderWithProviders(
    <ChannelTestHarness channel={channel} upstreamKey={missingKey} />
  )

  expect(await screen.findByDisplayValue('Auto route')).toBeInTheDocument()
  expect(await screen.findByText('Missing')).toBeInTheDocument()
  expect(await screen.findByText('gpt-live')).toBeInTheDocument()
  expect(screen.queryByText('gpt-missing')).not.toBeInTheDocument()
})

test('平台站点测试弹窗手动切换其他密钥后刷新不会恢复入口默认值', async () => {
  const user = userEvent.setup()
  const entryKey = upstreamKey()
  const backupKey = upstreamKey({
    id: 8,
    name: 'Backup key',
    external_id: 'backup-key',
    models: ['gpt-backup'],
    routable: true,
  })
  const channel = {
    ...platformChannel(entryKey),
    upstream_keys: [entryKey, backupKey],
  } as Channel
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [entryKey, backupKey], total: 2 },
  })

  renderWithProviders(
    <ChannelTestHarness channel={channel} upstreamKey={entryKey} />
  )

  await user.click(await screen.findByRole('combobox', { name: 'Test key' }))
  await user.click(await screen.findByRole('option', { name: /Backup key/ }))

  expect(await screen.findByDisplayValue('Backup key')).toBeInTheDocument()
  expect(await screen.findByText('gpt-backup')).toBeInTheDocument()

  act(() => {
    queryClient.setQueryData(['upstream-keys', 101], {
      success: true,
      data: {
        items: [
          entryKey,
          {
            ...backupKey,
            models: ['gpt-backup-refreshed'],
          },
        ],
        total: 2,
      },
    })
  })

  expect(await screen.findByDisplayValue('Backup key')).toBeInTheDocument()
  expect(await screen.findByText('gpt-backup-refreshed')).toBeInTheDocument()
  expect(screen.queryByText('gpt-key-only')).not.toBeInTheDocument()
})

test('平台站点测试弹窗手动切换自动路由后刷新不会恢复入口默认值', async () => {
  const user = userEvent.setup()
  const entryKey = upstreamKey()
  vi.mocked(channelsApi.getUpstreamKeys).mockResolvedValue({
    success: true,
    data: { items: [entryKey], total: 1 },
  })

  renderWithProviders(
    <ChannelTestHarness
      channel={platformChannel(entryKey)}
      upstreamKey={entryKey}
    />
  )

  await user.click(await screen.findByRole('combobox', { name: 'Test key' }))
  await user.click(await screen.findByRole('option', { name: /Auto route/ }))

  expect(await screen.findByDisplayValue('Auto route')).toBeInTheDocument()

  act(() => {
    queryClient.setQueryData(['upstream-keys', 101], {
      success: true,
      data: { items: [entryKey], total: 1 },
    })
  })

  expect(await screen.findByDisplayValue('Auto route')).toBeInTheDocument()
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

test('平台站点测试弹窗遇到禁用当前密钥时保留入口选择并透传全局 key_id', async () => {
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

  expect(await screen.findByDisplayValue('Disabled key')).toBeInTheDocument()
  expect(
    await screen.findByText('上游密钥模型能力读取失败')
  ).toBeInTheDocument()
  expect(await screen.findByText('gpt-disabled')).toBeInTheDocument()
  expect(screen.queryByText('gpt-parent')).not.toBeInTheDocument()
  expect(screen.queryByText('gpt-live')).not.toBeInTheDocument()

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
        testModel: 'gpt-disabled',
        keyId: disabledKey.key_id,
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
