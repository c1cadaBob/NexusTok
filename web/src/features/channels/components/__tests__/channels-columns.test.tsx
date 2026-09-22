/*
Copyright (C) 2023-2026 c1cadaBob

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@c1cadabob.dev
*/
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { expect, test } from 'vitest'

import type { Channel } from '@/features/channels/types'
import { formatTimestampToDate } from '@/lib/format'

import { UpstreamSiteSyncStatusCell } from '../channels-columns'

function renderStatusCell(
  status: NonNullable<Channel['upstream_site_status']>
) {
  const channel = {
    id: 101,
    upstream_kind: 'platform_site',
    upstream_site_status: status,
  } as Channel

  return render(<UpstreamSiteSyncStatusCell channel={channel} />)
}

test('平台状态单元格默认只显示同步时间', async () => {
  const user = userEvent.setup()

  renderStatusCell({
    channel_id: 101,
    platform: 'newapi',
    base_url: 'https://upstream.example',
    auth_type: 'password',
    recharge_amount: 1,
    credited_amount: 10,
    conversion_ratio: 0.1,
    balance: 8,
    used_quota: 20,
    balance_updated_time: 0,
    sync_status: 'failed',
    last_sync_at: 0,
    last_sync_error: 'upstream unavailable',
    consecutive_failures: 2,
    using_last_snapshot: false,
    credential_available: false,
    needs_credential_save: true,
  })

  expect(screen.getByText('Never')).toBeInTheDocument()
  expect(screen.queryByText('Failed')).not.toBeInTheDocument()

  await user.hover(screen.getByText('Never'))

  expect(await screen.findByText('Sync status: Failed')).toBeInTheDocument()
  expect(
    screen.getByText(
      'Credential cannot be decrypted; resave platform credentials.'
    )
  ).toBeInTheDocument()
  expect(screen.getByText('upstream unavailable')).toBeInTheDocument()
})

test('平台状态悬停提示保留完整同步时间和快照状态', async () => {
  const user = userEvent.setup()
  const lastSyncAt = 1_700_000_000

  renderStatusCell({
    channel_id: 101,
    platform: 'newapi',
    base_url: 'https://upstream.example',
    auth_type: 'password',
    recharge_amount: 1,
    credited_amount: 10,
    conversion_ratio: 0.1,
    balance: 8,
    used_quota: 20,
    balance_updated_time: lastSyncAt,
    sync_status: 'running',
    last_sync_at: lastSyncAt,
    consecutive_failures: 1,
    using_last_snapshot: true,
  })

  const trigger = screen.getByText(/ago/).parentElement as HTMLElement
  await user.hover(trigger)

  await waitFor(
    () => {
      expect(screen.getByText('Sync status: Running')).toBeInTheDocument()
    },
    { timeout: 2000 }
  )
  expect(
    screen.getByText('Sync is running; using the last successful snapshot.')
  ).toBeInTheDocument()
  expect(
    screen.getByText(formatTimestampToDate(lastSyncAt))
  ).toBeInTheDocument()
})
