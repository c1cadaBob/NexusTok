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
    completePlatformSiteCapture: vi.fn(),
  }
})

type PlatformSiteFormProps = {
  isEditing?: boolean
  authType?: ChannelFormValues['platform_site_auth_type']
  captureID?: string
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
      capture_bridge_url:
        'https://nexustok.example/api/channel/platform-site/capture-session/capture-123/bridge.js?install_token=install-token',
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
      capture_bridge_url:
        'https://nexustok.example/api/channel/platform-site/capture-session/capture-123/bridge.js?install_token=install-token',
    },
  })
  vi.mocked(channelsApi.completePlatformSiteCapture).mockResolvedValue({
    success: true,
    data: {
      capture_id: 'capture-123',
      status: 'completed',
      expires_at: Math.floor(Date.now() / 1000) + 600,
      platform: 'newapi',
      base_url: 'https://upstream.example',
      auth_type: 'auto',
      origin: 'https://upstream.example',
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
      platform_site_capture_id: props.captureID ?? '',
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

test('凭证区域仅显示账号密码和自动配置，不显示高级认证输入', () => {
  render(<PlatformSiteForm />)

  expect(screen.getByLabelText('Authentication method')).toBeInTheDocument()
  expect(screen.getByLabelText('Site URL *')).toBeInTheDocument()
  expect(screen.getByLabelText('Username')).toBeInTheDocument()
  expect(screen.getByLabelText('Password')).toBeInTheDocument()
  expect(screen.queryByLabelText('Access token')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Admin Key')).not.toBeInTheDocument()
  expect(screen.queryByLabelText('Cookie')).not.toBeInTheDocument()
  expect(
    screen.queryByLabelText('Dashboard Session ID')
  ).not.toBeInTheDocument()
  expect(screen.queryByText('Advanced authentication')).not.toBeInTheDocument()
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
        return_url: window.location.href,
      })
    )
  })
  expect(pendingWindow.location.href).toBe(
    'https://upstream.example/?nexustok_capture=payload'
  )
  openSpy.mockRestore()
})

test('安装采集助手时优先打开稳定助手地址', async () => {
  const user = userEvent.setup()
  const openedWindow = {
    closed: false,
    focus: vi.fn(),
    location: { href: '' },
    opener: null,
  } as unknown as Window
  const openSpy = vi.spyOn(window, 'open').mockReturnValue(openedWindow)

  render(<PlatformSiteForm authType='auto' />)
  await user.click(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  )

  const installButton = await screen.findByRole('button', {
    name: 'Install Capture Helper',
  })
  await user.click(installButton)

  expect(openSpy).toHaveBeenLastCalledWith(
    'https://nexustok.example/api/channel/platform-site/capture-helper.user.js',
    '_blank',
    'noopener,noreferrer'
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
  expect(
    screen.getByRole('button', { name: 'Install Capture Helper' })
  ).toBeInTheDocument()
  await user.click(fallbackButton)
  expect(openSpy).toHaveBeenLastCalledWith(
    'https://upstream.example/?nexustok_capture=payload',
    '_blank'
  )
  openSpy.mockRestore()
})

test('页面桥接只接受正确窗口和来源并回传完成确认', async () => {
  const user = userEvent.setup()
  const postMessage = vi.fn()
  const upstreamWindow = {
    closed: false,
    focus: vi.fn(),
    location: { href: '' },
    opener: window,
    postMessage,
  } as unknown as Window
  const openSpy = vi.spyOn(window, 'open').mockReturnValue(upstreamWindow)
  vi.mocked(channelsApi.completePlatformSiteCapture).mockClear()

  render(<PlatformSiteForm authType='auto' />)
  await user.click(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  )
  await screen.findByRole('button', { name: 'Run page bridge' })
  await waitFor(() => {
    expect(channelsApi.getPlatformSiteCaptureStatus).toHaveBeenCalledWith(
      'capture-123'
    )
  })

  const payload = {
    type: 'nexustok-upstream-capture-bridge-result',
    capture_id: 'capture-123',
    payload: {
      capture_secret: 'capture-secret',
      capture_source: 'capture_bridge',
      helper_version: '1.7.0',
      platform: 'newapi',
      auth_type: 'auto',
      access_token: 'temporary-access-token',
      diagnostics: { auth_user_verified: true },
    },
  }
  const dispatchMessage = (
    source: Window,
    origin: string,
    data: typeof payload
  ) => {
    const event = Object.assign(new Event('message'), {
      source,
      origin,
      data,
    }) as MessageEvent
    window.dispatchEvent(event)
  }

  dispatchMessage({} as Window, 'https://upstream.example', payload)
  dispatchMessage(upstreamWindow, 'https://wrong.example', payload)
  expect(channelsApi.completePlatformSiteCapture).not.toHaveBeenCalled()

  dispatchMessage(upstreamWindow, 'https://upstream.example', payload)
  await waitFor(() => {
    expect(channelsApi.completePlatformSiteCapture).toHaveBeenCalledWith(
      'capture-123',
      expect.objectContaining({
        capture_source: 'capture_bridge',
        auth_type: 'auto',
        origin: 'https://upstream.example',
      })
    )
  })
  expect(postMessage).toHaveBeenCalledWith(
    {
      type: 'nexustok-upstream-capture-bridge-ack',
      capture_id: 'capture-123',
      success: true,
    },
    'https://upstream.example'
  )
  openSpy.mockRestore()
})

test('桥接页面丢失 URL handoff 时可从活动上游窗口请求一次性 handoff', async () => {
  const user = userEvent.setup()
  const postMessage = vi.fn()
  const upstreamWindow = {
    closed: false,
    focus: vi.fn(),
    location: { href: '' },
    opener: window,
    postMessage,
  } as unknown as Window
  const openSpy = vi.spyOn(window, 'open').mockReturnValue(upstreamWindow)

  render(<PlatformSiteForm authType='auto' />)
  await user.click(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  )
  await screen.findByRole('button', { name: 'Run page bridge' })

  const request = {
    type: 'nexustok-upstream-capture-bridge-request',
    capture_id: 'capture-123',
  }
  const event = Object.assign(new Event('message'), {
    source: upstreamWindow,
    origin: 'https://upstream.example',
    data: request,
  }) as MessageEvent
  window.dispatchEvent(event)

  await waitFor(() => {
    expect(postMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'nexustok-upstream-capture-bridge-handoff',
        capture_id: 'capture-123',
        handoff_url: 'https://upstream.example/?nexustok_capture=payload',
      }),
      'https://upstream.example'
    )
  })
  openSpy.mockRestore()
})

test('运行页面桥接前获取内联脚本并保留可复制降级代码', async () => {
  const user = userEvent.setup()
  const bridgeSource =
    'window.postMessage({type:"nexustok-upstream-capture-bridge-result"},"*")'
  const clipboardWriteText = vi.fn().mockResolvedValue(undefined)
  const originalClipboard = Object.getOwnPropertyDescriptor(
    navigator,
    'clipboard'
  )
  Object.defineProperty(navigator, 'clipboard', {
    configurable: true,
    value: { writeText: clipboardWriteText },
  })
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    text: async () => bridgeSource,
  })
  vi.stubGlobal('fetch', fetchMock)
  const upstreamWindow = {
    closed: false,
    focus: vi.fn(),
    location: { href: '' },
    opener: window,
    postMessage: vi.fn(),
  } as unknown as Window
  const openSpy = vi.spyOn(window, 'open').mockReturnValue(upstreamWindow)

  render(<PlatformSiteForm authType='auto' />)
  await user.click(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  )
  const runBridgeButton = await screen.findByRole('button', {
    name: 'Run page bridge',
  })
  await user.click(runBridgeButton)

  await waitFor(() => {
    expect(fetchMock).toHaveBeenCalledWith(
      expect.stringContaining('/bridge.js?install_token='),
      expect.objectContaining({
        credentials: 'include',
        cache: 'no-store',
      })
    )
  })
  expect(upstreamWindow.location.href).toBe(
    'https://upstream.example/?nexustok_capture=payload'
  )
  expect(
    await screen.findByRole('button', { name: 'Copy page bridge code' })
  ).toBeInTheDocument()
  await waitFor(() => {
    expect(clipboardWriteText).toHaveBeenCalled()
  })
  const bootstrap = String(clipboardWriteText.mock.calls.at(-1)?.[0] || '')
  expect(bootstrap).not.toContain('javascript:')
  expect(bootstrap).toContain('(()=>{')
  expect(bootstrap).toContain(JSON.stringify(window.location.origin))
  expect(bootstrap).not.toContain(JSON.stringify('https://upstream.example'))
  expect(bootstrap).toContain('document.scripts')
  expect(bootstrap).toContain('script[nonce]')
  expect(bootstrap).toContain('getAttribute')
  expect(bootstrap).toContain('s.nonce')
  expect(bootstrap).toContain('DOMContentLoaded')
  expect(bootstrap).toContain('MutationObserver')
  expect(bootstrap).toContain('nexustok-upstream-capture-bridge-failed')
  expect(bootstrap.indexOf('if(!n)return false')).toBeLessThan(
    bootstrap.indexOf('document.createElement')
  )

  const request = {
    type: 'nexustok-upstream-capture-bridge-script-request',
    capture_id: 'capture-123',
  }
  const event = Object.assign(new Event('message'), {
    source: upstreamWindow,
    origin: 'https://upstream.example',
    data: request,
  }) as MessageEvent
  window.dispatchEvent(event)

  await waitFor(() => {
    expect(upstreamWindow.postMessage).toHaveBeenCalledWith(
      expect.objectContaining({
        type: 'nexustok-upstream-capture-bridge-script',
        capture_id: 'capture-123',
        script: bridgeSource,
      }),
      'https://upstream.example'
    )
  })

  openSpy.mockRestore()
  vi.unstubAllGlobals()
  if (originalClipboard) {
    Object.defineProperty(navigator, 'clipboard', originalClipboard)
  } else {
    Reflect.deleteProperty(navigator, 'clipboard')
  }
})

test('页面桥接失败消息只接受活动窗口、来源和 Capture ID 匹配的回传', async () => {
  const user = userEvent.setup()
  const upstreamWindow = {
    closed: false,
    focus: vi.fn(),
    location: { href: '' },
    opener: window,
    postMessage: vi.fn(),
  } as unknown as Window
  const openSpy = vi.spyOn(window, 'open').mockReturnValue(upstreamWindow)
  vi.mocked(channelsApi.completePlatformSiteCapture).mockClear()

  render(<PlatformSiteForm authType='auto' />)
  await user.click(
    screen.getByRole('button', { name: 'Capture upstream login state' })
  )
  await screen.findByRole('button', { name: 'Run page bridge' })

  const failure = {
    type: 'nexustok-upstream-capture-bridge-failed',
    capture_id: 'capture-123',
    reason: 'csp_nonce_unavailable',
  }
  const dispatchMessage = (
    source: Window,
    origin: string,
    data: typeof failure
  ) => {
    const event = Object.assign(new Event('message'), {
      source,
      origin,
      data,
    }) as MessageEvent
    window.dispatchEvent(event)
  }

  dispatchMessage({} as Window, 'https://upstream.example', failure)
  dispatchMessage(upstreamWindow, 'https://wrong.example', failure)
  dispatchMessage(upstreamWindow, 'https://upstream.example', {
    ...failure,
    capture_id: 'other-capture',
  })
  expect(channelsApi.completePlatformSiteCapture).not.toHaveBeenCalled()

  dispatchMessage(upstreamWindow, 'https://upstream.example', failure)
  await waitFor(() => {
    expect(channelsApi.completePlatformSiteCapture).not.toHaveBeenCalled()
  })
  openSpy.mockRestore()
})

test('采集会话过期后停止轮询并显示重新创建入口', async () => {
  vi.mocked(channelsApi.getPlatformSiteCaptureStatus).mockResolvedValue({
    success: false,
    message: '采集会话不存在或已过期',
  })

  render(<PlatformSiteForm authType='auto' captureID='expired-capture' />)

  await waitFor(() => {
    expect(
      screen.getByText(
        'Capture session expired. Create a new session to continue.'
      )
    ).toBeInTheDocument()
  })
  expect(
    screen.getByRole('button', { name: 'Create a new capture session' })
  ).toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Refresh capture status' })
  ).not.toBeInTheDocument()
})

test('从 Capture Helper 回跳参数恢复采集会话并清理地址栏参数', async () => {
  const originalURL = window.location.href
  window.history.pushState(
    {},
    '',
    `${originalURL.split('?')[0]}?platform_site_capture_id=returned-capture`
  )
  vi.mocked(channelsApi.getPlatformSiteCaptureStatus).mockResolvedValue({
    success: true,
    data: {
      capture_id: 'returned-capture',
      status: 'pending',
      expires_at: Math.floor(Date.now() / 1000) + 600,
      platform: 'newapi',
      base_url: 'https://upstream.example',
      auth_type: 'auto',
      origin: 'https://upstream.example',
    },
  })

  try {
    render(<PlatformSiteForm authType='auto' />)

    await waitFor(() => {
      expect(channelsApi.getPlatformSiteCaptureStatus).toHaveBeenCalledWith(
        'returned-capture'
      )
    })
    expect(window.location.search).toBe('')
  } finally {
    window.history.replaceState({}, '', originalURL)
  }
})
