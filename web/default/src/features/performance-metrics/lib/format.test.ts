import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import {
  getSuccessRateDotClass,
  getSuccessRateLevel,
  getSuccessRateTextClass,
} from './format'

describe('性能成功率等级', () => {
  test('按共享阈值划分健康等级', () => {
    assert.equal(getSuccessRateLevel(100), 'excellent')
    assert.equal(getSuccessRateLevel(99.99), 'good')
    assert.equal(getSuccessRateLevel(90), 'good')
    assert.equal(getSuccessRateLevel(89.99), 'warning')
    assert.equal(getSuccessRateLevel(70), 'warning')
    assert.equal(getSuccessRateLevel(69.99), 'critical')
    assert.equal(getSuccessRateLevel(Number.NaN), 'unknown')
  })

  test('文本和状态点使用同一等级映射', () => {
    assert.equal(getSuccessRateTextClass(100), 'text-success')
    assert.equal(getSuccessRateDotClass(100), 'bg-success')
    assert.equal(getSuccessRateTextClass(95), 'text-success/80')
    assert.equal(getSuccessRateDotClass(95), 'bg-success/80')
    assert.equal(getSuccessRateTextClass(80), 'text-warning')
    assert.equal(getSuccessRateDotClass(80), 'bg-warning')
    assert.equal(getSuccessRateTextClass(50), 'text-destructive')
    assert.equal(getSuccessRateDotClass(50), 'bg-destructive')
  })
})
