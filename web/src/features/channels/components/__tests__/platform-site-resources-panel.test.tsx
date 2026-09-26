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
import { beforeEach, expect, test, vi } from 'vitest'

import * as channelsApi from '@/features/channels/api'
import type {
  PlatformSiteResources,
  PlatformSiteResourcesResponse,
  UpstreamKey,
} from '@/features/channels/types'

import { PlatformSiteResourcesPanel } from '../platform-site-resources-panel'

vi.mock('@/features/channels/api', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/channels/api')
  >('@/features/channels/api')
  return {
    ...actual,
    getPlatformSiteResources: vi.fn(),
    syncPlatformSiteResources: vi.fn(),
  }
})

let queryClient: QueryClient

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })
  vi.mocked(channelsApi.getPlatformSiteResources).mockReset()
  vi.mocked(channelsApi.syncPlatformSiteResources).mockReset()
})

function renderPanel() {
  return render(
    <QueryClientProvider client={queryClient}>
      <PlatformSiteResourcesPanel channelId={101} />
    </QueryClientProvider>
  )
}

function response(
  data: PlatformSiteResources | undefined
): PlatformSiteResourcesResponse {
  return { success: true, data }
}

function upstreamKey(
  overrides: Partial<UpstreamKey> = {}
): UpstreamKey {
  return {
    id: 7,
    key_id: 1007,
    channel_id: 101,
    external_id: 'key-7',
    name: 'Production key',
    key_preview: 'sk-live...mask',
    models: ['gpt-4o'],
    allowed_models: null,
    models_synced: true,
    key_priority: 10,
    source_conversion_ratio: 0.8,
    conversion_ratio: 0.1,
    conversion_ratio_override: null,
    weight: 1900,
    auto_weight: 1900,
    weight_override: null,
    used_quota: 120,
    remain_quota: 880,
    expires_at: '2030-01-01T00:00:00Z',
    status: 1,
    last_sync_at: 1_700_000_000,
    routable: true,
    availability_reason: '',
    ...overrides,
  }
}

function resources(
  overrides: Partial<PlatformSiteResources> = {}
): PlatformSiteResources {
  return {
    channel_id: 101,
    platform: 'newapi',
    management_base_url: 'https://manage.example',
    relay_base_url: 'https://relay.example/v1',
    balance: 12.5,
    used_quota: 345,
    identity: {
      platform_user_id: 'user-7',
      username: 'operator',
      email: 'operator@example.com',
      display_name: 'Operator',
      role: 'admin',
      current_group: 'pro',
      status: 'active',
      quota_unit: 'quota',
      balance: 12.5,
      used_quota: 345,
      current_value_at: 1_700_000_000,
      snapshot_value_at: 1_700_000_000,
      source_endpoint: '/api/user/self',
      upstream_updated_at: 1_700_000_000,
      last_sync_at: 1_700_000_000,
    },
    groups: [
      {
        external_id: 'pro',
        name: 'Pro',
        ratio: 0.8,
        available: true,
        usable: true,
        source_endpoint: '/api/pricing',
        upstream_updated_at: 1_700_000_000,
        last_sync_at: 1_700_000_000,
      },
    ],
    endpoint: {
      management_url: 'https://manage.example',
      relay_url: 'https://relay.example/v1',
      models_url: 'https://relay.example/v1/models',
      pricing_url: 'https://manage.example/api/pricing',
      usage_url: 'https://manage.example/api/user/self',
      token_url: 'https://manage.example/api/token/',
      admin_url: 'https://manage.example/api/channel/',
      openai_url: 'https://relay.example/v1',
      claude_url: 'https://relay.example/v1',
      gemini_url: 'https://relay.example/v1',
      responses_url: 'https://relay.example/v1',
      source: 'NewAPI',
      discovery_method: 'configured_base_url',
      enabled: true,
      last_confirmed_at: 1_700_000_000,
      capabilities: [
        {
          protocol: 'openai',
          http_method: 'POST',
          path: '/v1/chat/completions',
          supported: true,
          source_data: 'supported_endpoint',
          last_confirmed_at: 1_700_000_000,
        },
      ],
    },
    resource_syncs: [
      {
        resource_type: 'keys',
        status: 'partial',
        attempted_at: 1_700_000_100,
        succeeded_at: 1_700_000_000,
        source_endpoint: '/api/token/',
        record_count: 1,
        failure_reason: '部分密钥详情读取失败',
        partial: true,
        requires_security_verification: false,
        using_snapshot: true,
      },
    ],
    keys: [upstreamKey()],
    key_count: 1,
    routable_key_count: 1,
    sync_status: 'partial',
    last_sync_at: 1_700_000_000,
    using_last_snapshot: true,
    ...overrides,
  }
}

test('资源查询加载期间显示加载状态', async () => {
  let resolve: (value: PlatformSiteResourcesResponse) => void = () => undefined
  vi.mocked(channelsApi.getPlatformSiteResources).mockReturnValue(
    new Promise((promiseResolve) => {
      resolve = promiseResolve
    })
  )

  renderPanel()

  expect(screen.getByText('Loading...')).toBeInTheDocument()
  resolve(response(resources()))
  await screen.findByText('Platform resources')
})

test('资源查询返回空数据时显示空状态', async () => {
  vi.mocked(channelsApi.getPlatformSiteResources).mockResolvedValue(
    response(undefined)
  )

  renderPanel()

  expect(await screen.findByText('No platform resources')).toBeInTheDocument()
})

test('资源查询失败后可以通过可访问的重试按钮重新加载', async () => {
  const user = userEvent.setup()
  vi.mocked(channelsApi.getPlatformSiteResources)
    .mockRejectedValueOnce(new Error('network failure'))
    .mockResolvedValueOnce(response(resources()))

  renderPanel()

  const retry = await screen.findByRole('button', { name: 'Retry' })
  expect(retry).toHaveAccessibleName('Retry')
  await user.click(retry)

  await screen.findByText('Platform resources')
  expect(channelsApi.getPlatformSiteResources).toHaveBeenCalledTimes(2)
})

test('资源面板展示部分成功、快照、分组倍率、端点能力和密钥额度', async () => {
  vi.mocked(channelsApi.getPlatformSiteResources).mockResolvedValue(
    response(resources())
  )

  renderPanel()

  await screen.findByText('Platform resources')
  expect(screen.getByText('Using last successful snapshot')).toBeInTheDocument()
  expect(screen.getByText('部分密钥详情读取失败')).toBeInTheDocument()
  expect(screen.getByText('0.8')).toBeInTheDocument()
  expect(screen.getByText('/v1/chat/completions')).toBeInTheDocument()
  expect(screen.getByText('supported_endpoint')).toBeInTheDocument()
  expect(screen.getByText('120')).toBeInTheDocument()
  expect(screen.getByText('880')).toBeInTheDocument()
  expect(screen.getByText(/2030/)).toBeInTheDocument()
  expect(screen.getByText('sk-live...mask')).toBeInTheDocument()
  expect(screen.queryByText('sk-secret-real')).not.toBeInTheDocument()
})

test('安全验证状态显示管理员提示且同步按钮在 pending 时禁用', async () => {
  const user = userEvent.setup()
  let resolve: (value: PlatformSiteResourcesResponse) => void = () => undefined
  vi.mocked(channelsApi.syncPlatformSiteResources).mockReturnValue(
    new Promise((promiseResolve) => {
      resolve = promiseResolve
    })
  )
  vi.mocked(channelsApi.getPlatformSiteResources).mockResolvedValue(
    response(
      resources({
        auth_status: 'secure_verification_required',
        auth_status_reason: '读取管理员密钥需要完成安全验证',
      })
    )
  )

  renderPanel()

  const syncButton = await screen.findByRole('button', { name: 'Sync resources' })
  expect(
    screen.getByText('secure_verification_required')
  ).toBeInTheDocument()
  await user.click(syncButton)
  expect(syncButton).toBeDisabled()

  resolve(response(resources()))
  await waitFor(() => expect(syncButton).not.toBeDisabled())
})

test('资源为空列表时仍保留表格可访问表头和空内容', async () => {
  const baseResources = resources()
  if (!baseResources.endpoint) {
    throw new Error('endpoint fixture is required')
  }
  vi.mocked(channelsApi.getPlatformSiteResources).mockResolvedValue(
    response(
      {
        ...baseResources,
        groups: [],
        keys: [],
        endpoint: {
          ...baseResources.endpoint,
          capabilities: [],
        },
      }
    )
  )

  renderPanel()

  await screen.findByText('Platform resources')
  expect(screen.getByText('No groups found')).toBeInTheDocument()
  expect(screen.getByText('No upstream keys found')).toBeInTheDocument()
  expect(screen.queryByText('No endpoint capabilities')).not.toBeInTheDocument()
})
