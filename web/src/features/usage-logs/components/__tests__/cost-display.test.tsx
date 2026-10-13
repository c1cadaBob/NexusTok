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
      label: 'Upstream',
      tone: 'green',
    },
    {
      source: 'estimated' as const,
      label: 'Estimated',
      tone: 'yellow',
    },
  ])(
    'labels $source cost with the $tone source treatment',
    ({ source, label, tone }) => {
      render(
        <UpstreamCostDisplay
          other={{
            admin_info: {
              upstream_cost: 12500,
              upstream_cost_source: source,
            },
          }}
        />
      )

      expect(screen.getByText(label)).toBeVisible()
      expect(screen.getByText(formatLogQuota(12500))).toBeVisible()
      expect(
        screen.getByLabelText(`${formatLogQuota(12500)} ${label}`)
      ).toHaveAttribute('data-cost-tone', tone)
    }
  )

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
