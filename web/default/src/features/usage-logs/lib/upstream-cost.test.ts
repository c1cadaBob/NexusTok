/*
Copyright (C) 2023-2026 c1cada

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

For commercial licensing, please contact support@c1cada.dev
*/
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import type { UsageLog } from '../data/schema'
import { getUpstreamCost } from './upstream-cost'

function usageLog(overrides: Partial<UsageLog>): UsageLog {
  return {
    id: 1,
    type: 2,
    user_id: 1,
    channel: 1,
    model_name: 'gpt-test',
    prompt_tokens: 0,
    completion_tokens: 0,
    quota: 0,
    use_time: 0,
    is_stream: false,
    other: '{}',
    ...overrides,
  }
}

describe('getUpstreamCost', () => {
  test('新日志优先使用标准计费基准乘以上游倍率', () => {
    const cost = getUpstreamCost(
      usageLog({
        quota: 400,
        other: JSON.stringify({
          group_ratio: 4,
          admin_info: {
            ratio_conversion: 0.5,
            standard_billing_quota: 100,
          },
        }),
      })
    )

    assert.equal(cost, 50)
  })

  test('旧日志优先按用户组特殊倍率回退', () => {
    const cost = getUpstreamCost(
      usageLog({
        quota: 600,
        other: JSON.stringify({
          group_ratio: 3,
          user_group_ratio: 2,
          admin_info: {
            ratio_conversion: 0.5,
          },
        }),
      })
    )

    assert.equal(cost, 150)
  })

  test('旧日志没有用户组特殊倍率时按普通分组倍率回退', () => {
    const cost = getUpstreamCost(
      usageLog({
        quota: 600,
        other: JSON.stringify({
          group_ratio: 3,
          user_group_ratio: -1,
          admin_info: {
            ratio_conversion: 0.5,
          },
        }),
      })
    )

    assert.equal(cost, 100)
  })

  test('旧日志缺失分组倍率时按 1 回退', () => {
    const cost = getUpstreamCost(
      usageLog({
        quota: 600,
        other: JSON.stringify({
          admin_info: {
            ratio_conversion: 0.5,
          },
        }),
      })
    )

    assert.equal(cost, 300)
  })

  test('缺少有效上游倍率时不展示成本', () => {
    assert.equal(
      getUpstreamCost(
        usageLog({
          quota: 600,
          other: JSON.stringify({
            group_ratio: 2,
            admin_info: {
              ratio_conversion: 0,
            },
          }),
        })
      ),
      null
    )
  })

  test('旧日志分组倍率为 0 或负数时不展示近似成本', () => {
    assert.equal(
      getUpstreamCost(
        usageLog({
          quota: 600,
          other: JSON.stringify({
            group_ratio: 0,
            admin_info: {
              ratio_conversion: 0.5,
            },
          }),
        })
      ),
      null
    )
    assert.equal(
      getUpstreamCost(
        usageLog({
          quota: 600,
          other: JSON.stringify({
            user_group_ratio: -0.5,
            group_ratio: 2,
            admin_info: {
              ratio_conversion: 0.5,
            },
          }),
        })
      ),
      null
    )
  })
})
