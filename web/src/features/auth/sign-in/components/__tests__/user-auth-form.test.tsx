/*
Copyright (C) 2023-2026 c1cadaBob

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
at your option any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.
*/
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import type { ReactNode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import { UserAuthForm } from '../user-auth-form'

const { login, handleLoginResult } = vi.hoisted(() => ({
  login: vi.fn(),
  handleLoginResult: vi.fn(),
}))

vi.mock('@/features/auth/api', () => ({
  login,
  wechatLoginByCode: vi.fn(),
}))

vi.mock('@/hooks/use-status', () => ({
  useStatus: () => ({ status: null }),
}))

vi.mock('@/features/auth/hooks/use-turnstile', () => ({
  useTurnstile: () => ({
    isTurnstileEnabled: false,
    turnstileSiteKey: '',
    turnstileToken: '',
    setTurnstileToken: vi.fn(),
    validateTurnstile: () => true,
  }),
}))

vi.mock('@/features/auth/hooks/use-auth-redirect', () => ({
  useAuthRedirect: () => ({ handleLoginResult }),
}))

vi.mock('@/lib/passkey', () => ({
  isPasskeySupported: vi.fn().mockResolvedValue(false),
  buildAssertionResult: vi.fn(),
  prepareCredentialRequestOptions: vi.fn(),
}))

vi.mock('@/features/auth/components/legal-consent', () => ({
  LegalConsent: () => null,
}))

vi.mock('@/features/auth/components/oauth-providers', () => ({
  OAuthProviders: () => null,
}))

vi.mock('@/components/dialog', () => ({
  Dialog: () => null,
}))

vi.mock('@/components/turnstile', () => ({
  Turnstile: () => null,
}))

vi.mock('@tanstack/react-router', () => ({
  Link: ({ children }: { children: ReactNode }) => <a>{children}</a>,
}))

vi.mock('@/lib/handle-server-error', () => ({
  handleServerError: vi.fn(),
}))

vi.mock('@/lib/secure-verification', () => ({
  AuthOperationError: {
    from: (error: unknown) => error,
  },
}))

vi.mock('@/lib/server-error-message', () => ({
  createServerError: (response: unknown) => response,
}))

describe('UserAuthForm', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    handleLoginResult.mockResolvedValue(true)
  })

  it('快速连续提交时只发送一次登录请求', async () => {
    let resolveLogin: (value: { success: boolean; data: object }) => void =
      () => undefined
    login.mockReturnValue(
      new Promise((resolve) => {
        resolveLogin = resolve
      })
    )

    render(<UserAuthForm />)
    fireEvent.change(screen.getByLabelText('Username or Email'), {
      target: { value: 'admin' },
    })
    fireEvent.change(screen.getByLabelText('Password'), {
      target: { value: 'password' },
    })

    const form = document.querySelector('form')
    if (!form) throw new Error('登录表单未渲染')
    fireEvent.submit(form)
    fireEvent.submit(form)

    await waitFor(() => expect(login).toHaveBeenCalledTimes(1))
    resolveLogin({ success: true, data: {} })
    await waitFor(() => expect(handleLoginResult).toHaveBeenCalledTimes(1))
  })
})
