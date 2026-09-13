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
import { describe, expect, test } from 'vitest'

import type { Channel, UpstreamKey } from '../../types'
import {
  filterUpstreamKeys,
  formatConversionRatio,
  getChannelTableRowId,
  type TagRow,
} from '../channel-utils'

function channel(id: number): Channel {
  return { id } as Channel
}

describe('channel table row identity', () => {
  test('keeps each channel identity when priority updates reorder the rows', () => {
    const first = channel(101)
    const updated = channel(202)
    const third = channel(303)

    const beforeUpdate = [first, updated, third].map(getChannelTableRowId)
    const afterUpdate = [updated, first, third].map(getChannelTableRowId)

    expect(beforeUpdate).toEqual(['channel:101', 'channel:202', 'channel:303'])
    expect(afterUpdate).toEqual(['channel:202', 'channel:101', 'channel:303'])
  })

  test('uses separate namespaces for tag and channel rows', () => {
    const tagRow = {
      id: '202' as unknown as number,
      tag: '202',
      children: [channel(202)],
    } as TagRow

    expect(getChannelTableRowId(tagRow)).toBe('tag:202')
    expect(getChannelTableRowId(channel(202))).toBe('channel:202')
  })

  test('keeps platform site parent rows in the channel namespace', () => {
    const platformSite = {
      id: 404,
      upstream_kind: 'platform_site',
      children: [
        {
          id: 1,
          is_upstream_key: true,
          parent_channel_id: 404,
        } as Channel,
      ],
    } as Channel

    expect(getChannelTableRowId(platformSite)).toBe('channel:404')
  })

  test('uses a parent-scoped namespace for upstream key rows', () => {
    const upstreamKey = {
      id: 202,
      is_upstream_key: true,
      parent_channel_id: 7,
    } as Channel

    expect(getChannelTableRowId(upstreamKey)).toBe('upstream-key:7:202')
    expect(getChannelTableRowId(upstreamKey)).not.toBe('channel:202')
  })

  test('filters upstream keys by keyword, model, and status', () => {
    const keys = [
      {
        id: 1,
        name: 'Production',
        external_id: 'prod-key',
        models: ['gpt-4o', 'claude-3-7'],
        status: 1,
      },
      {
        id: 2,
        name: 'Disabled backup',
        external_id: 'backup-key',
        models: ['gpt-4o-mini'],
        status: 2,
      },
    ] as UpstreamKey[]

    expect(
      filterUpstreamKeys(keys, {
        keyword: 'prod',
        model: 'claude',
        status: ['enabled'],
      }).map((key) => key.id)
    ).toEqual([1])
    expect(
      filterUpstreamKeys(keys, { status: ['disabled'] }).map((key) => key.id)
    ).toEqual([2])
  })

  test('formats conversion ratio with bounded readable precision', () => {
    expect(formatConversionRatio(0)).toBe('0')
    expect(formatConversionRatio(0.1)).toBe('0.1')
    expect(formatConversionRatio(1.23456)).toBe('1.235')
    expect(formatConversionRatio(null)).toBe('-')
    expect(formatConversionRatio(Number.POSITIVE_INFINITY)).toBe('-')
  })
})
