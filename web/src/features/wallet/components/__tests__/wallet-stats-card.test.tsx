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
*/
import { render, screen } from '@testing-library/react'
import { createInstance } from 'i18next'
import { I18nextProvider, initReactI18next } from 'react-i18next'
import { describe, expect, test } from 'vitest'

import type { UserWalletData } from '../../types'
import { WalletStatsCard } from '../wallet-stats-card'

const i18n = createInstance()
await i18n.use(initReactI18next).init({
  lng: 'en',
  resources: {
    en: {
      translation: {
        'Current Balance': 'Current Balance',
        'Remaining quota': 'Remaining quota',
        'Total Usage': 'Total Usage',
        'Total consumed quota': 'Total consumed quota',
        'API Requests': 'API Requests',
        'Total requests made': 'Total requests made',
        'Upstream platform total balance': 'Upstream platform total balance',
        'Configured upstream accounts: {{count}}':
          'Configured upstream accounts: {{count}}',
        'Last synced: {{time}}': 'Last synced: {{time}}',
        'Upstream balance sync pending': 'Upstream balance sync pending',
      },
    },
  },
})

const user: UserWalletData = {
  id: 1,
  username: 'admin',
  quota: 500_000,
  used_quota: 100_000,
  request_count: 12,
  aff_quota: 0,
  aff_history_quota: 0,
  aff_count: 0,
  group: 'default',
  upstream_platform_balance: {
    quota: 1_500_000,
    account_count: 3,
    last_sync_at: 1_700_000_000,
  },
}

function renderCard(isAdmin: boolean) {
  return render(
    <I18nextProvider i18n={i18n}>
      <WalletStatsCard user={user} isAdmin={isAdmin} />
    </I18nextProvider>
  )
}

describe('wallet statistics card', () => {
  test('shows the upstream platform balance only to administrators', () => {
    renderCard(true)

    expect(screen.getByText('Upstream platform total balance')).toBeInTheDocument()
    expect(
      screen.getByText(/Configured upstream accounts: 3/)
    ).toBeInTheDocument()
    expect(screen.getByText(/Last synced:/)).toBeInTheDocument()
  })

  test('does not show the upstream platform balance to regular users', () => {
    renderCard(false)

    expect(
      screen.queryByText('Upstream platform total balance')
    ).not.toBeInTheDocument()
    expect(
      screen.queryByText(/Configured upstream accounts: 3/)
    ).not.toBeInTheDocument()
  })
})
