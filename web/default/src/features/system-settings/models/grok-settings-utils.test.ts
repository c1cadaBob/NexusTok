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
  buildGrokFormDefaults,
  getChangedGrokSettingKeys,
  normalizeGrokFormValues,
  type FlatGrokSettings,
} from './grok-settings-utils'

describe('Grok 设置表单值转换', () => {
  const flatDefaults: FlatGrokSettings = {
    'grok.violation_deduction_enabled': true,
    'grok.violation_deduction_amount': 0.05,
  }

  test('将后端扁平 option key 转换为 RHF 嵌套默认值', () => {
    assert.deepEqual(buildGrokFormDefaults(flatDefaults), {
      grok: {
        violation_deduction_enabled: true,
        violation_deduction_amount: 0.05,
      },
    })
  })

  test('将 RHF 嵌套提交值转换回后端扁平 option key', () => {
    assert.deepEqual(
      normalizeGrokFormValues({
        grok: {
          violation_deduction_enabled: false,
          violation_deduction_amount: 0.1,
        },
      }),
      {
        'grok.violation_deduction_enabled': false,
        'grok.violation_deduction_amount': 0.1,
      }
    )
  })

  test('只返回相对基线发生变化的 option key', () => {
    assert.deepEqual(
      getChangedGrokSettingKeys(
        {
          'grok.violation_deduction_enabled': true,
          'grok.violation_deduction_amount': 0.1,
        },
        flatDefaults
      ),
      ['grok.violation_deduction_amount']
    )
  })

  test('无变化时不产生保存 key', () => {
    assert.deepEqual(getChangedGrokSettingKeys(flatDefaults, flatDefaults), [])
  })
})
