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
import {
  buildModelRows,
  buildModelSnapshots,
  getPriceDetail,
  getPriceSummary,
  getSnapshotSignature,
  type ModelPricingSnapshotInput,
} from './model-pricing-snapshots'

const t = (key: string) => key

const emptyInput: ModelPricingSnapshotInput = {
  modelPrice: '{}',
  modelRatio: '{}',
  cacheRatio: '{}',
  createCacheRatio: '{}',
  completionRatio: '{}',
  imageRatio: '{}',
  audioRatio: '{}',
  audioCompletionRatio: '{}',
  billingMode: '{}',
  billingExpr: '{}',
}

describe('模型定价快照', () => {
  test('从倍率 JSON 构造 per-token 快照和价格摘要', () => {
    const [snapshot] = buildModelSnapshots({
      ...emptyInput,
      modelRatio: '{"gpt-test":1.5}',
      completionRatio: '{"gpt-test":2}',
      cacheRatio: '{"gpt-test":0.2}',
    })

    assert.equal(snapshot.name, 'gpt-test')
    assert.equal(snapshot.billingMode, 'per-token')
    assert.equal(snapshot.hasConflict, false)
    assert.equal(getPriceSummary(snapshot, t), 'Input $3 · 2 extras')
    assert.equal(getPriceDetail(snapshot, t), 'Output $6 · Cache $0.6')
  })

  test('固定请求价格和倍率同时存在时标记冲突', () => {
    const [snapshot] = buildModelSnapshots({
      ...emptyInput,
      modelPrice: '{"gpt-conflict":0.01}',
      modelRatio: '{"gpt-conflict":1}',
    })

    assert.equal(snapshot.billingMode, 'per-request')
    assert.equal(snapshot.hasConflict, true)
    assert.equal(getPriceSummary(snapshot, t), '$0.01 / request')
  })

  test('tiered_expr 快照会拆分请求规则并保留兜底倍率', () => {
    const [snapshot] = buildModelSnapshots({
      ...emptyInput,
      modelRatio: '{"gpt-expr":1.25}',
      billingMode: '{"gpt-expr":"tiered_expr"}',
      billingExpr:
        '{"gpt-expr":"(tier(\\"base\\", p * 2 + c * 8)) * (has(header(\\"x-plan\\"), \\"pro\\") ? 2 : 1)"}',
    })

    assert.equal(snapshot.billingMode, 'tiered_expr')
    assert.equal(snapshot.billingExpr, 'tier("base", p * 2 + c * 8)')
    assert.equal(
      snapshot.requestRuleExpr,
      '(has(header("x-plan"), "pro") ? 2 : 1)'
    )
    assert.equal(snapshot.ratio, '1.25')
    assert.equal(snapshot.hasConflict, false)
    assert.equal(getPriceSummary(snapshot, t), 'Tiered pricing · 1 tiers')
    assert.equal(getPriceDetail(snapshot, t), 'Includes request rules')
  })

  test('合并 saved 与 draft 行并识别新增和变更草稿', () => {
    const saved = {
      ...emptyInput,
      modelRatio: '{"gpt-old":1,"gpt-change":1}',
    }
    const draft = {
      ...emptyInput,
      modelRatio: '{"gpt-change":1.5,"gpt-new":2}',
    }

    const rows = buildModelRows({ saved, draft })
    const names = rows.map((row) => row.name)

    assert.deepEqual(names, ['gpt-change', 'gpt-new'])
    assert.equal(
      rows.find((row) => row.name === 'gpt-old'),
      undefined
    )

    const changed = rows.find((row) => row.name === 'gpt-change')
    assert.equal(changed?.ratio, '1.5')
    assert.equal(changed?.saved?.ratio, '1')
    assert.equal(changed?.draft?.ratio, '1.5')
    assert.equal(changed?.isDraftChanged, true)
    assert.equal(changed?.isDraftNew, false)
    assert.equal(changed?.isDraftDeleted, false)

    const added = rows.find((row) => row.name === 'gpt-new')
    assert.equal(added?.isDraftChanged, true)
    assert.equal(added?.isDraftNew, true)
    assert.equal(added?.isDraftDeleted, false)
  })

  test('相同快照签名稳定，字段变化会改变签名', () => {
    const base = buildModelSnapshots({
      ...emptyInput,
      modelRatio: '{"gpt-same":1}',
    })[0]
    const same = buildModelSnapshots({
      ...emptyInput,
      modelRatio: '{"gpt-same":1}',
    })[0]
    const changed = buildModelSnapshots({
      ...emptyInput,
      modelRatio: '{"gpt-same":1.1}',
    })[0]

    assert.equal(getSnapshotSignature(base), getSnapshotSignature(same))
    assert.notEqual(getSnapshotSignature(base), getSnapshotSignature(changed))
  })
})
