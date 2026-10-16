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
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import i18next from 'i18next'
import type React from 'react'
import { beforeAll, describe, expect, test } from 'vitest'

import { formatLogQuota } from '@/lib/format'

import { LogCostDisplay, UpstreamCostDisplay } from '../log-cost-display'

function renderCost(
  props: React.ComponentProps<typeof LogCostDisplay>
): ReturnType<typeof render> {
  return render(<LogCostDisplay {...props} />)
}

function normalizedText(value: string | null): string {
  return (value ?? '').replaceAll(/\s/g, '')
}

describe('log cost display', () => {
  beforeAll(() => {
    i18next.addResourceBundle('en', 'translation', {
      Subscription: 'Subscription',
      'Deducted by subscription': 'Deducted by subscription',
      'Includes tool-call surcharge': 'Includes tool-call surcharge',
      'Cost lookup was skipped because the request failed':
        'Cost lookup was skipped because the request failed',
      'Platform authentication failed': 'Platform authentication failed',
      'Platform denied access to usage logs':
        'Platform denied access to usage logs',
      'Platform usage-log request failed': 'Platform usage-log request failed',
      'No usage records were returned': 'No usage records were returned',
      'No usage record matched the request ID':
        'No usage record matched the request ID',
      'No usage record matched the model and time':
        'No usage record matched the model and time',
      'Multiple usage records matched this request':
        'Multiple usage records matched this request',
      'Usage record did not contain a valid cost':
        'Usage record did not contain a valid cost',
      'Historical estimate; fallback reason was not recorded':
        'Historical estimate; fallback reason was not recorded',
      'HTTP status: {{status}}': 'HTTP status: {{status}}',
      'Platform error code: {{code}}': 'Platform error code: {{code}}',
    })
  })

  test('keeps the regular cost visible and adds an accessible surcharge marker', () => {
    const rendered = renderCost({
      quota: 12500,
      other: {
        tool_surcharges: [{ name: 'lookup_customer', count: 1, price: 5 }],
      },
    })

    expect(
      normalizedText(rendered.container.textContent).includes(
        normalizedText(formatLogQuota(12500))
      )
    ).toBe(true)
    const marker = screen.getByRole('img', {
      name: 'Includes tool-call surcharge',
    })
    expect(marker).toHaveAttribute('data-tool-surcharge-indicator', 'true')
    expect(marker).toHaveAttribute('tabindex', '0')
  })

  test('preserves the subscription badge and adds the same legacy surcharge marker', () => {
    renderCost({
      quota: 5000,
      other: {
        billing_source: 'subscription',
        web_search: true,
        web_search_call_count: 1,
        web_search_price: 10,
      },
    })

    expect(screen.getByText('Subscription')).toBeInTheDocument()
    expect(
      screen.getByRole('img', { name: 'Includes tool-call surcharge' })
    ).toHaveAttribute('data-tool-surcharge-indicator', 'true')
  })

  test.each([
    {
      source: 'upstream' as const,
      tone: 'green',
    },
    {
      source: 'estimated' as const,
      tone: 'yellow',
    },
  ])(
    'shows only the numeric $source cost with the $tone source treatment',
    async ({ source, tone }) => {
      const formattedCost = formatLogQuota(12500)
      const rendered = render(
        <UpstreamCostDisplay
          other={{
            admin_info: {
              upstream_cost: 12500,
              upstream_cost_source: source,
            },
          }}
        />
      )

      const cost =
        source === 'upstream'
          ? screen.getByLabelText(formattedCost)
          : rendered.container.querySelector('[data-cost-source="estimated"]')
      if (!cost) throw new Error('Expected an upstream cost badge')
      expect(cost).toHaveAttribute('data-cost-tone', tone)
      expect(cost).toHaveTextContent(formattedCost)
      expect(screen.queryByText('Upstream')).not.toBeInTheDocument()
      expect(screen.queryByText('Estimated')).not.toBeInTheDocument()
      if (source === 'estimated') {
        await userEvent.setup().hover(cost)
        expect(
          await screen.findByText(
            'Historical estimate; fallback reason was not recorded'
          )
        ).toBeInTheDocument()
      }
    }
  )

  test('renders zero estimated cost without a source label', () => {
    const formattedCost = formatLogQuota(0)
    const rendered = render(
      <UpstreamCostDisplay
        other={{
          admin_info: {
            upstream_cost: 0,
            upstream_cost_source: 'estimated',
          },
        }}
      />
    )

    const cost = rendered.container.querySelector(
      '[data-cost-source="estimated"]'
    )
    expect(cost).toHaveAttribute('data-cost-tone', 'yellow')
    expect(cost).toHaveTextContent(formattedCost)
    expect(screen.queryByText('Estimated')).not.toBeInTheDocument()
  })

  test.each([
    ['request_error', 'Cost lookup was skipped because the request failed'],
    ['authentication_failed', 'Platform authentication failed'],
    ['permission_denied', 'Platform denied access to usage logs'],
    ['http_request_failed', 'Platform usage-log request failed'],
    ['empty_result', 'No usage records were returned'],
    ['request_id_no_match', 'No usage record matched the request ID'],
    ['model_or_time_no_match', 'No usage record matched the model and time'],
    ['ambiguous_match', 'Multiple usage records matched this request'],
    ['invalid_cost_record', 'Usage record did not contain a valid cost'],
  ])(
    'shows the %s fallback reason on keyboard focus',
    async (reason, message) => {
      render(
        <UpstreamCostDisplay
          other={{
            admin_info: {
              upstream_cost: 12,
              upstream_cost_source: 'estimated',
              upstream_cost_fallback_reason: reason,
              upstream_cost_fallback_status: 429,
              upstream_cost_fallback_code: 'AUTH_SESSION_ISSUANCE_LIMIT',
            },
          }}
        />
      )

      await userEvent.setup().tab()
      expect(await screen.findByText(message)).toBeInTheDocument()
      expect(screen.getByText('HTTP status: 429')).toBeInTheDocument()
      expect(
        screen.getByText('Platform error code: AUTH_SESSION_ISSUANCE_LIMIT')
      ).toBeInTheDocument()
    }
  )

  test('opens the fallback reason when touched', async () => {
    const user = userEvent.setup()
    render(
      <UpstreamCostDisplay
        other={{
          admin_info: {
            upstream_cost: 12,
            upstream_cost_source: 'estimated',
            upstream_cost_fallback_reason: 'empty_result',
          },
        }}
      />
    )

    await user.click(screen.getByRole('button'))
    expect(
      await screen.findByText('No usage records were returned')
    ).toBeInTheDocument()
  })

  test('does not show a fallback tooltip for verified upstream cost', () => {
    render(
      <UpstreamCostDisplay
        other={{
          admin_info: {
            upstream_cost: 12,
            upstream_cost_source: 'upstream',
            upstream_cost_fallback_reason: 'permission_denied',
          },
        }}
      />
    )

    expect(screen.queryByRole('button')).not.toBeInTheDocument()
    expect(screen.queryByRole('tooltip')).not.toBeInTheDocument()
  })

  test('does not render a cost without a recognized source', () => {
    const rendered = render(
      <UpstreamCostDisplay
        other={{
          admin_info: {
            upstream_cost: 12500,
          },
        }}
      />
    )

    expect(rendered.container).toBeEmptyDOMElement()
  })
})
