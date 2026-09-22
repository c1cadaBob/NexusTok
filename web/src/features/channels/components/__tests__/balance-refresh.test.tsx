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
import { toast } from 'sonner'
import { beforeEach, expect, test, vi } from 'vitest'

import * as channelsApi from '@/features/channels/api'
import type { Channel } from '@/features/channels/types'

import { BalanceCell } from '../channels-columns'
import { ChannelsProvider, useChannels } from '../channels-provider'
import { BalanceQueryDialog } from '../dialogs/balance-query-dialog'

vi.mock('@/features/channels/api', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/channels/api')
  >('@/features/channels/api')
  return {
    ...actual,
    getCodexUsage: vi.fn(),
    updateChannelBalance: vi.fn(),
  }
})

let queryClient: QueryClient

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  vi.mocked(channelsApi.updateChannelBalance).mockReset()
  vi.mocked(channelsApi.getCodexUsage).mockReset()
  vi.spyOn(toast, 'success').mockReturnValue('success')
  vi.spyOn(toast, 'error').mockReturnValue('error')
})

function platformChannel(): Channel {
  return {
    id: 101,
    name: 'NewAPI upstream',
    type: 60,
    upstream_kind: 'platform_site',
    key: '',
    status: 1,
    balance: 1,
    balance_updated_time: 1_700_000_000,
    used_quota: 10,
    models: '',
    group: 'default',
    upstream_site_status: {
      channel_id: 101,
      platform: 'newapi',
      base_url: 'https://upstream.example',
      auth_type: 'password',
      recharge_amount: 1,
      credited_amount: 10,
      conversion_ratio: 0.1,
      balance: 1,
      used_quota: 10,
      balance_updated_time: 1_700_000_000,
      sync_status: 'success',
      last_sync_at: 1_700_000_000,
      consecutive_failures: 0,
      key_count: 1,
      routable_key_count: 1,
    },
  } as Channel
}

function BalanceDialogHarness(props: { channel: Channel }) {
  const { setCurrentRow } = useChannels()

  React.useEffect(() => {
    setCurrentRow(props.channel)
  }, [props.channel, setCurrentRow])

  return <BalanceQueryDialog open onOpenChange={() => undefined} />
}

function renderBalanceDialog(channel: Channel) {
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  const result = render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <BalanceDialogHarness channel={channel} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  return { ...result, invalidateSpy }
}

function renderBalanceCell(channel: Channel) {
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')
  const result = render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <BalanceCell channel={channel} />
      </ChannelsProvider>
    </QueryClientProvider>
  )
  return { ...result, invalidateSpy }
}

test('平台站点余额刷新成功后使用服务端时间并刷新站点和子密钥缓存', async () => {
  const user = userEvent.setup()
  vi.mocked(channelsApi.updateChannelBalance).mockResolvedValue({
    success: true,
    balance: 8,
    used_quota: 20,
    balance_updated_time: 1_800_000_000,
    sync_status: 'success',
    key_count: 3,
    routable_key_count: 2,
  })
  const { invalidateSpy } = renderBalanceDialog(platformChannel())

  await user.click(screen.getByRole('button', { name: 'Update Balance' }))

  await waitFor(() => {
    expect(channelsApi.updateChannelBalance).toHaveBeenCalledWith(101)
    expect(toast.success).toHaveBeenCalled()
  })
  expect(
    invalidateSpy.mock.calls.some(
      ([options]) =>
        JSON.stringify(options?.queryKey) ===
        JSON.stringify(['channels', 'list'])
    )
  ).toBe(true)
  expect(
    invalidateSpy.mock.calls.some(
      ([options]) =>
        JSON.stringify(options?.queryKey) ===
        JSON.stringify(['upstream-site-status', 101])
    )
  ).toBe(true)
  expect(
    invalidateSpy.mock.calls.some(
      ([options]) =>
        JSON.stringify(options?.queryKey) ===
        JSON.stringify(['upstream-keys', 101])
    )
  ).toBe(true)
  expect(screen.getByText(/2027-01-15/)).toBeInTheDocument()
})

test('平台站点余额单元格优先显示上游状态中的余额和已使用余额', () => {
  const channel = platformChannel()
  channel.balance = 1
  channel.used_quota = 10
  const upstreamSiteStatus = channel.upstream_site_status
  if (!upstreamSiteStatus) {
    throw new Error('platform fixture must include upstream site status')
  }
  channel.upstream_site_status = {
    ...upstreamSiteStatus,
    balance: 8,
    used_quota: 10_000_000,
  }

  renderBalanceCell(channel)

  expect(screen.getByText('$20')).toBeInTheDocument()
  expect(screen.getByText('$8')).toBeInTheDocument()
})

test('平台站点余额刷新失败时展示错误且不清空已有余额', async () => {
  const user = userEvent.setup()
  vi.mocked(channelsApi.updateChannelBalance).mockResolvedValue({
    success: false,
    message: 'upstream sync failed',
    balance: 1,
    used_quota: 10,
    balance_updated_time: 1_700_000_000,
    sync_status: 'failed',
    last_sync_at: 1_700_000_000,
    last_sync_error: 'refresh failed',
  })
  const { invalidateSpy } = renderBalanceDialog(platformChannel())

  await user.click(screen.getByRole('button', { name: 'Update Balance' }))

  await waitFor(() => {
    expect(toast.error).toHaveBeenCalled()
  })
  expect(invalidateSpy).not.toHaveBeenCalledWith(
    expect.objectContaining({ queryKey: ['upstream-keys', 101] })
  )
  expect(screen.getByText(/2023-11-14/)).toBeInTheDocument()
})

test('平台站点余额徽标支持键盘触发刷新并防止重复请求', async () => {
  const user = userEvent.setup()
  let resolveBalance: (
    value: Awaited<ReturnType<typeof channelsApi.updateChannelBalance>>
  ) => void = () => undefined
  vi.mocked(channelsApi.updateChannelBalance).mockImplementation(() => {
    return new Promise((resolve) => {
      resolveBalance = resolve
    })
  })
  const { invalidateSpy } = renderBalanceCell(platformChannel())
  const refreshButton = screen.getByRole('button', { name: 'Update Balance' })

  refreshButton.focus()
  await user.keyboard('[Enter][Enter]')

  expect(channelsApi.updateChannelBalance).toHaveBeenCalledTimes(1)
  expect(refreshButton).toHaveAttribute('aria-disabled', 'true')
  resolveBalance({
    success: true,
    balance: 8,
    used_quota: 20,
    balance_updated_time: 1_800_000_000,
    sync_status: 'success',
    key_count: 3,
    routable_key_count: 2,
  })

  await waitFor(() => {
    expect(toast.success).toHaveBeenCalled()
  })
  expect(refreshButton).toHaveAttribute('aria-disabled', 'false')
  expect(
    invalidateSpy.mock.calls.some(
      ([options]) =>
        JSON.stringify(options?.queryKey) ===
        JSON.stringify(['upstream-keys', 101])
    )
  ).toBe(true)
})
