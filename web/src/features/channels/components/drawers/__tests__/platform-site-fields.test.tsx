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
import i18next from 'i18next'
import { useForm } from 'react-hook-form'
import { beforeEach, expect, test, vi } from 'vitest'

import { Form } from '@/components/ui/form'
import * as channelsApi from '@/features/channels/api'
import { ChannelsProvider } from '@/features/channels/components/channels-provider'
import { CHANNEL_TYPE_NEW_API } from '@/features/channels/constants'
import {
  CHANNEL_FORM_DEFAULT_VALUES,
  transformFormDataToCreatePayload,
  type ChannelFormValues,
} from '@/features/channels/lib'
import { ROLE } from '@/lib/roles'
import { useAuthStore } from '@/stores/auth-store'

import { ChannelMutateDrawer } from '../channel-mutate-drawer'
import { PlatformSiteFields } from '../platform-site-fields'

vi.mock('@/features/channels/api', async () => {
  const actual = await vi.importActual<
    typeof import('@/features/channels/api')
  >('@/features/channels/api')
  return {
    ...actual,
    getAllModels: vi.fn(),
    getGroups: vi.fn(),
    getPrefillGroups: vi.fn(),
    getTaskPluginOptions: vi.fn(),
    startPlatformSiteCapture: vi.fn(),
    getPlatformSiteCaptureStatus: vi.fn(),
    submitPlatformSiteVerification: vi.fn(),
    syncUpstreamSite: vi.fn(),
  }
})

type PlatformSiteFormProps = {
  isEditing?: boolean
  authType?: ChannelFormValues['platform_site_auth_type']
  channelId?: number
  syncStatus?: import('@/features/channels/types').UpstreamSiteStatus
  onSubmit?: (values: ChannelFormValues) => void
}

let queryClient: QueryClient

beforeEach(() => {
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  })
  useAuthStore.getState().auth.setUser({
    id: 1,
    username: 'root',
    role: ROLE.SUPER_ADMIN,
  })
  vi.mocked(channelsApi.getAllModels).mockResolvedValue({
    success: true,
    data: [],
  })
  vi.mocked(channelsApi.getGroups).mockResolvedValue({
    success: true,
    data: ['default'],
  })
  vi.mocked(channelsApi.getPrefillGroups).mockResolvedValue({
    success: true,
    data: [],
  })
  vi.mocked(channelsApi.getTaskPluginOptions).mockResolvedValue([])
  vi.mocked(channelsApi.startPlatformSiteCapture).mockResolvedValue({
    success: true,
    data: {
      capture_id: 'capture-123',
      expires_at: Math.floor(Date.now() / 1000) + 600,
      platform: 'newapi',
      base_url: 'https://upstream.example',
      auth_type: 'auto',
      origin: 'https://upstream.example',
      userscript_url:
        'https://nexustok.example/api/channel/platform-site/capture-session/capture-123/userscript.user.js',
      helper_install_url:
        'https://nexustok.example/api/channel/platform-site/capture-helper.user.js',
      handoff_url: 'https://upstream.example/?nexustok_capture=payload',
      login_url: 'https://upstream.example',
    },
  })
  vi.mocked(channelsApi.getPlatformSiteCaptureStatus).mockResolvedValue({
    success: true,
    data: {
      capture_id: 'capture-123',
      status: 'pending',
      expires_at: Math.floor(Date.now() / 1000) + 600,
      platform: 'newapi',
      base_url: 'https://upstream.example',
      auth_type: 'auto',
      origin: 'https://upstream.example',
      userscript_url:
        'https://nexustok.example/api/channel/platform-site/capture-session/capture-123/userscript.user.js',
      helper_install_url:
        'https://nexustok.example/api/channel/platform-site/capture-helper.user.js',
      handoff_url: 'https://upstream.example/?nexustok_capture=payload',
      login_url: 'https://upstream.example',
    },
  })
})

function PlatformSiteForm(props: PlatformSiteFormProps) {
  const form = useForm<ChannelFormValues>({
    defaultValues: {
      ...CHANNEL_FORM_DEFAULT_VALUES,
      name: 'Platform site',
      type: CHANNEL_TYPE_NEW_API,
      upstream_kind: 'platform_site',
      base_url: 'https://upstream.example',
      models: 'gpt-4o',
      platform_site_auth_type: props.authType ?? 'password',
    },
  })

  return (
    <QueryClientProvider client={queryClient}>
      <Form {...form}>
        <form
          onSubmit={form.handleSubmit((values) => props.onSubmit?.(values))}
        >
          <PlatformSiteFields
            disabled={false}
            isEditing={props.isEditing === true}
            channelId={props.channelId}
            syncStatus={props.syncStatus}
          />
          <button type='submit'>Save</button>
        </form>
      </Form>
    </QueryClientProvider>
  )
}

function renderChannelMutateDrawer() {
  return render(
    <QueryClientProvider client={queryClient}>
      <ChannelsProvider>
        <ChannelMutateDrawer
          open
          onOpenChange={() => undefined}
          currentRow={null}
        />
      </ChannelsProvider>
    </QueryClientProvider>
  )
}

async function enterNumber(
  user: ReturnType<typeof userEvent.setup>,
  input: HTMLElement,
  value: string
): Promise<void> {
  await user.clear(input)
  await user.type(input, value)
}

test('基础信息三项字段在桌面端使用三列等宽且顶部对齐', async () => {
  renderChannelMutateDrawer()

  const upstreamKindLabel = await screen.findByText('Upstream channel type')
  const fieldset = upstreamKindLabel.closest('fieldset')
  const basicGrid = fieldset?.parentElement
  expect(basicGrid).not.toBeNull()
  expect(fieldset).not.toBeNull()

  expect(basicGrid?.className).toContain(
    'lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1fr)]'
  )
  expect(basicGrid?.className).toContain('lg:items-start')
  expect(fieldset?.className).toContain('items-start')
  expect(fieldset?.className).toContain(
    'lg:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]'
  )

  const upstreamKindTrigger = screen.getByRole('combobox', {
    name: 'Upstream channel type',
  })
  expect(upstreamKindTrigger.className).toContain('w-full')
})

test('平台站点输入充值和到账金额后稳定预览自动倍率', async () => {
  const user = userEvent.setup()
  render(<PlatformSiteForm />)

  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Recharge amount' }),
    '1'
  )
  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Credited amount' }),
    '10'
  )

  await waitFor(() => {
    expect(
      screen.getByRole('spinbutton', { name: 'Conversion ratio' })
    ).toHaveValue(0.1)
  })
  expect(
    screen.getByRole('switch', { name: 'Override conversion ratio' })
  ).not.toBeChecked()
  expect(
    screen.queryByText('Automatic ratio: 0.100. Edit this field to override.')
  ).not.toBeInTheDocument()
  expect(
    screen.queryByText('Leave blank when editing to keep the saved credential.')
  ).not.toBeInTheDocument()
})

test('连续输入到账金额字符不会产生递归渲染并保持有限倍率', async () => {
  const user = userEvent.setup()
  render(<PlatformSiteForm />)

  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Recharge amount' }),
    '1'
  )
  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Credited amount' }),
    '100000'
  )

  await waitFor(() => {
    expect(
      screen.getByRole('spinbutton', { name: 'Conversion ratio' })
    ).toHaveValue(0.00001)
  })
})

test('编辑平台站点时修改充值和到账金额也会刷新自动倍率', async () => {
  const user = userEvent.setup()
  render(<PlatformSiteForm isEditing />)

  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Recharge amount' }),
    '1'
  )
  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Credited amount' }),
    '10'
  )

  await waitFor(() => {
    expect(
      screen.getByRole('spinbutton', { name: 'Conversion ratio' })
    ).toHaveValue(0.1)
  })
})

test('到账金额清空或为零时不写入 NaN', async () => {
  const user = userEvent.setup()
  render(<PlatformSiteForm />)
  const ratioInput = screen.getByRole('spinbutton', {
    name: 'Conversion ratio',
  })

  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Recharge amount' }),
    '1'
  )
  await user.clear(screen.getByRole('spinbutton', { name: 'Credited amount' }))

  expect(ratioInput).toHaveValue(1)
  expect(ratioInput).not.toHaveValue(Number.NaN)
  expect(
    screen.queryByText(
      'Enter recharge and credited amounts to calculate the ratio.'
    )
  ).not.toBeInTheDocument()
})

test('手动编辑转换倍率后金额变化不会覆盖手动倍率', async () => {
  const user = userEvent.setup()
  render(<PlatformSiteForm />)
  const ratioInput = screen.getByRole('spinbutton', {
    name: 'Conversion ratio',
  })
  const overrideSwitch = screen.getByRole('switch', {
    name: 'Override conversion ratio',
  })

  await enterNumber(user, ratioInput, '0.25')

  await waitFor(() => {
    expect(overrideSwitch).toBeChecked()
  })
  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Recharge amount' }),
    '1'
  )
  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Credited amount' }),
    '10'
  )

  expect(ratioInput).toHaveValue(0.25)
  expect(
    screen.queryByText('Manual ratio override: 0.250.')
  ).not.toBeInTheDocument()
})

test('关闭转换倍率覆盖后恢复自动倍率', async () => {
  const user = userEvent.setup()
  render(<PlatformSiteForm />)
  const ratioInput = screen.getByRole('spinbutton', {
    name: 'Conversion ratio',
  })
  const overrideSwitch = screen.getByRole('switch', {
    name: 'Override conversion ratio',
  })

  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Recharge amount' }),
    '1'
  )
  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Credited amount' }),
    '10'
  )
  await enterNumber(user, ratioInput, '0.25')
  await user.click(overrideSwitch)

  await waitFor(() => {
    expect(overrideSwitch).not.toBeChecked()
    expect(ratioInput).toHaveValue(0.1)
  })
})

test('提交时仅在手动覆盖后发送平台转换倍率', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn<(values: ChannelFormValues) => void>()
  render(<PlatformSiteForm onSubmit={onSubmit} />)

  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Recharge amount' }),
    '1'
  )
  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Credited amount' }),
    '10'
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => {
    expect(onSubmit).toHaveBeenCalledOnce()
  })
  let payload = transformFormDataToCreatePayload(onSubmit.mock.calls[0][0])
  expect(payload.platform_site?.conversion_ratio).toBeUndefined()

  onSubmit.mockClear()
  await enterNumber(
    user,
    screen.getByRole('spinbutton', { name: 'Conversion ratio' }),
    '0.25'
  )
  await user.click(screen.getByRole('button', { name: 'Save' }))

  await waitFor(() => {
    expect(onSubmit).toHaveBeenCalledOnce()
  })
  payload = transformFormDataToCreatePayload(onSubmit.mock.calls[0][0])
  expect(payload.platform_site?.conversion_ratio).toBe(0.25)
})

test('自动配置显示脚本采集入口且不显示手动凭据输入框', () => {
  render(<PlatformSiteForm authType='access_token' />)

  expect(screen.getByText('Browser login state capture')).toBeInTheDocument()
  expect(screen.getAllByText('Automatic configuration').length).toBeGreaterThan(
    0
  )
  expect(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  ).toBeInTheDocument()
  expect(screen.queryByLabelText('Access token')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Admin Key')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Cookie')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Username')).not.toBeInTheDocument()
})

test('点击自动配置时同步预开窗口并仅提交自动认证方式', async () => {
  const user = userEvent.setup()
  const pendingWindow = {
    closed: false,
    focus: vi.fn(),
    location: { href: '' },
    opener: null,
  } as unknown as Window
  const openSpy = vi
    .spyOn(window, 'open')
    .mockReturnValueOnce(pendingWindow)
    .mockReturnValueOnce(pendingWindow)

  render(<PlatformSiteForm authType='auto' />)
  await user.click(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  )

  expect(openSpy).toHaveBeenCalledWith('about:blank', '_blank')
  await waitFor(() => {
    expect(channelsApi.startPlatformSiteCapture).toHaveBeenCalledWith(
      expect.objectContaining({
        auth_type: 'auto',
      })
    )
  })
  expect(pendingWindow.location.href).toBe(
    'https://upstream.example/?nexustok_capture=payload'
  )
  openSpy.mockRestore()
})

test('预开窗口被拦截时保留会话并提供手动打开按钮', async () => {
  const user = userEvent.setup()
  const openedWindow = {
    closed: false,
    focus: vi.fn(),
    location: { href: '' },
    opener: null,
  } as unknown as Window
  const openSpy = vi
    .spyOn(window, 'open')
    .mockReturnValueOnce(null)
    .mockReturnValueOnce(openedWindow)

  render(<PlatformSiteForm authType='auto' />)
  await user.click(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  )

  const fallbackButton = await screen.findByRole('button', {
    name: 'Open upstream capture page',
  })
  expect(fallbackButton).toBeInTheDocument()
  await user.click(fallbackButton)
  expect(openSpy).toHaveBeenLastCalledWith(
    'https://upstream.example/?nexustok_capture=payload',
    '_blank'
  )
  openSpy.mockRestore()
})

test('账号密码模式显示采集入口并提交缓存登录态 capture_id', async () => {
  const user = userEvent.setup()
  const onSubmit = vi.fn()
  const pendingWindow = {
    closed: false,
    focus: vi.fn(),
    location: { href: '' },
    opener: null,
  } as unknown as Window
  const openSpy = vi
    .spyOn(window, 'open')
    .mockReturnValueOnce(pendingWindow)
    .mockReturnValueOnce(pendingWindow)

  render(<PlatformSiteForm authType='password' onSubmit={onSubmit} />)

  expect(screen.getByText('Browser login state capture')).toBeInTheDocument()
  expect(screen.getByLabelText('Username')).toBeInTheDocument()
  expect(screen.getByLabelText('Password')).toBeInTheDocument()

  await user.click(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  )

  await waitFor(() => {
    expect(channelsApi.startPlatformSiteCapture).toHaveBeenCalledWith(
      expect.objectContaining({
        auth_type: 'password',
      })
    )
  })
  expect(pendingWindow.location.href).toBe(
    'https://upstream.example/?nexustok_capture=payload'
  )

  await user.click(screen.getByRole('button', { name: 'Save' }))
  await waitFor(() => {
    expect(onSubmit).toHaveBeenCalledOnce()
  })
  const payload = transformFormDataToCreatePayload(onSubmit.mock.calls[0][0])
  expect(payload.platform_site).toMatchObject({
    auth_type: 'password',
    capture_id: 'capture-123',
  })
  openSpy.mockRestore()
})

test('等待验证状态显示平台信息和剩余次数并提交验证码', async () => {
  const user = userEvent.setup()
  i18next.addResource(
    'en',
    'translation',
    'Attempts remaining',
    'Attempts remaining: {{count}}'
  )
  vi.mocked(channelsApi.submitPlatformSiteVerification).mockResolvedValue({
    success: false,
    code: 'UPSTREAM_CHALLENGE_CODE_INVALID',
    message: 'Verification failed. Please try again.',
    challenge_id: 'challenge-123',
    expires_at: Math.floor(Date.now() / 1000) + 240,
    attempts_remaining: 2,
  })

  render(
    <PlatformSiteForm
      channelId={123}
      syncStatus={{
        channel_id: 123,
        platform: 'newapi',
        base_url: 'https://upstream.example',
        auth_type: 'password',
        recharge_amount: 0,
        credited_amount: 0,
        conversion_ratio: 1,
        balance: 0,
        used_quota: 0,
        balance_updated_time: 0,
        sync_status: 'waiting_verification',
        last_sync_at: 0,
        consecutive_failures: 0,
        challenge_id: 'challenge-123',
        challenge_expires_at: Math.floor(Date.now() / 1000) + 240,
        attempts_remaining: 3,
      }}
    />
  )

  expect(screen.getByText('Security verification')).toBeInTheDocument()
  expect(screen.getByText('NewAPI')).toBeInTheDocument()
  expect(
    screen.getByText('Attempts remaining: 3')
  ).toBeInTheDocument()

  await user.type(
    screen.getByRole('textbox', { name: 'Verification code' }),
    '123456'
  )
  await user.click(screen.getByRole('button', { name: 'Submit' }))

  await waitFor(() => {
    expect(channelsApi.submitPlatformSiteVerification).toHaveBeenCalledWith(
      123,
      'challenge-123',
      '123456'
    )
  })
  expect(
    screen.getByRole('textbox', { name: 'Verification code' })
  ).toBeInTheDocument()
})

test('缺少 Challenge 上下文时提示真实浏览器采集登录态', () => {
  render(
    <PlatformSiteForm
      channelId={123}
      syncStatus={{
        channel_id: 123,
        platform: 'newapi',
        base_url: 'https://upstream.example',
        auth_type: 'password',
        recharge_amount: 0,
        credited_amount: 0,
        conversion_ratio: 1,
        balance: 0,
        used_quota: 0,
        balance_updated_time: 0,
        sync_status: 'waiting_verification',
        last_sync_at: 0,
        consecutive_failures: 0,
      }}
    />
  )

  expect(
    screen.getByText(
      'Complete verification in a real browser and capture the login state before syncing again.'
    )
  ).toBeInTheDocument()
  expect(
    screen.queryByRole('textbox', { name: 'Verification code' })
  ).not.toBeInTheDocument()
  expect(
    screen.getByRole('button', { name: 'Sync upstream site' })
  ).toBeInTheDocument()
})

test('重新同步业务失败时显示错误且不误报成功', async () => {
  const user = userEvent.setup()
  vi.mocked(channelsApi.syncUpstreamSite).mockResolvedValue({
    success: false,
    message: 'Failed to sync upstream site',
  })

  render(
    <PlatformSiteForm
      channelId={123}
      syncStatus={{
        channel_id: 123,
        platform: 'sub2api',
        base_url: 'https://upstream.example',
        auth_type: 'password',
        recharge_amount: 0,
        credited_amount: 0,
        conversion_ratio: 1,
        balance: 0,
        used_quota: 0,
        balance_updated_time: 0,
        sync_status: 'waiting_verification',
        last_sync_at: 0,
        consecutive_failures: 0,
        challenge_id: 'challenge-123',
        challenge_expires_at: Math.floor(Date.now() / 1000) + 240,
        attempts_remaining: 3,
      }}
    />
  )

  await user.click(screen.getByRole('button', { name: 'Sync upstream site' }))

  await waitFor(() => {
    expect(channelsApi.syncUpstreamSite).toHaveBeenCalledWith(123)
  })
})
