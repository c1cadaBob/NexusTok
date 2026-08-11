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
import { renderAuditContent } from './format'
import type { LogOtherData } from '../types'

const t = (key: string, opts?: Record<string, unknown>) =>
  key.replace(/{{\s*([A-Za-z0-9_]+)\s*}}/g, (_match, name: string) => {
    const value = opts?.[name]
    return value == null ? '' : String(value)
  })

describe('renderAuditContent', () => {
  test('登录日志会使用结构化 method 渲染摘要', () => {
    const text = renderAuditContent(
      {
        op: {
          action: 'login',
          params: { method: 'password' },
        },
      },
      t
    )

    assert.equal(text, 'Logged in successfully via password')
  })

  test('渠道更新日志在参数完整时使用模板摘要', () => {
    const text = renderAuditContent(
      {
        op: {
          action: 'channel.update',
          params: { name: 'OpenAI', id: 12, changed_fields: ['models'] },
        },
      },
      t
    )

    assert.equal(text, 'Updated channel OpenAI (ID: 12)')
  })

  test('上游渠道手动同步日志使用本地化模板摘要', () => {
    const text = renderAuditContent(
      {
        op: {
          action: 'channel.upstream_account_sync_refresh',
          params: { name: 'Sub2API', id: 12 },
        },
      },
      t
    )

    assert.equal(
      text,
      'Refreshed upstream account sync for channel Sub2API (ID: 12)'
    )
  })

  test('上游账号系统任务日志支持 task_id 别名', () => {
    const text = renderAuditContent(
      {
        op: {
          action: 'system_task.upstream_account_sync',
          params: { task_id: 'task-1' },
        },
      },
      t
    )

    assert.equal(text, 'Ran upstream account sync system task task-1')
  })

  test('兜底 generic 审计会渲染请求方法和路由', () => {
    const text = renderAuditContent(
      {
        op: {
          action: 'generic',
          params: { method: 'PUT', route: '/api/channel/' },
        },
      },
      t
    )

    assert.equal(text, 'PUT /api/channel/')
  })

  test('模板缺少必要参数时退回到稳定 action 摘要和审计参数', () => {
    const text = renderAuditContent(
      {
        op: {
          action: 'channel.update',
          params: {},
        },
        audit_info: {
          method: 'PUT',
          route: '/api/channel/',
          status: 200,
        },
      } satisfies LogOtherData,
      t
    )

    assert.equal(
      text,
      'Audit operation Channel Update · method: PUT, route: /api/channel/, status: 200'
    )
  })

  test('未知 action 不会抛错且保留关键参数', () => {
    const text = renderAuditContent(
      {
        op: {
          action: 'account_pool.future_action',
          params: { id: 9, name: 'primary' },
        },
      },
      t
    )

    assert.equal(
      text,
      'Audit operation Account Pool Future Action · name: primary, id: 9'
    )
  })

  test('缺少结构化 op 时返回 null 让调用方继续使用 raw content', () => {
    assert.equal(renderAuditContent(null, t), null)
    assert.equal(renderAuditContent({}, t), null)
  })
})
