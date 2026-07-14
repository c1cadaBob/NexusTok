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
  advancedCustomConfigUsesRelativeUpstreamPath,
  buildAdvancedCustomConfigSummary,
  getAdvancedCustomExportFilename,
  getAdvancedCustomStats,
  getAdvancedCustomTemplateConfig,
  normalizeAdvancedCustomConfig,
  parseAdvancedCustomConfig,
  stringifyAdvancedCustomConfig,
  validateAdvancedCustomConfig,
} from './advanced-custom'
import type { AdvancedCustomConfig } from '../types'

describe('Advanced Custom 前端配置工具', () => {
  test('模板可以被克隆、序列化并重新解析', () => {
    const template = getAdvancedCustomTemplateConfig('official_openai_chat')
    const text = stringifyAdvancedCustomConfig(template)
    const parsed = parseAdvancedCustomConfig(text)

    assert.deepEqual(parsed, normalizeAdvancedCustomConfig(template))
    assert.equal(validateAdvancedCustomConfig(parsed), null)
  })

  test('重复 incoming path 会被拒绝', () => {
    const error = validateAdvancedCustomConfig({
      advanced_routes: [
        {
          incoming_path: '/v1/chat/completions',
          upstream_path: '/v1/chat/completions',
          converter: 'none',
        },
        {
          incoming_path: '/v1/chat/completions',
          upstream_path: '/v1/responses',
          converter: 'none',
        },
      ],
    })

    assert.equal(error?.message, 'Incoming path must be unique')
    assert.equal(error?.routeIndex, 1)
  })

  test('converter 与入口路径不匹配时会被拒绝', () => {
    const error = validateAdvancedCustomConfig({
      advanced_routes: [
        {
          incoming_path: '/v1/responses',
          upstream_path: '/v1/messages',
          converter: 'openai_chat_completions_to_anthropic_messages',
        },
      ],
    })

    assert.equal(error?.message, 'Converter does not match incoming path')
  })

  test('统计和相对 upstream path 检测可用于抽屉摘要', () => {
    const config: AdvancedCustomConfig = {
      advanced_routes: [
        {
          incoming_path: '/v1/chat/completions',
          upstream_path: '/v1beta/models/{model}:generateContent',
          converter: 'openai_chat_completions_to_gemini_generate_content',
        },
      ],
    }
    const text = stringifyAdvancedCustomConfig(config)

    assert.equal(advancedCustomConfigUsesRelativeUpstreamPath(config), true)
    assert.deepEqual(getAdvancedCustomStats(text), {
      routeCount: 1,
      valid: true,
      routeTypeLabels: ['OpenAI Chat'],
    })
  })

  test('配置摘要隐藏 auth value 并统计迁移预览所需信息', () => {
    const config: AdvancedCustomConfig = {
      advanced_routes: [
        {
          incoming_path: '/v1/chat/completions',
          upstream_path: '/v1/chat/completions',
          converter: 'none',
          auth: {
            type: 'header',
            name: 'Authorization',
            value: 'Bearer secret-value',
          },
        },
        {
          incoming_path: '/v1/responses',
          upstream_path: 'https://api.example.com/v1/responses',
          converter: 'openai_responses_to_openai_chat_completions',
        },
      ],
    }

    assert.deepEqual(buildAdvancedCustomConfigSummary(config), {
      routeCount: 2,
      valid: true,
      incomingPaths: ['/v1/chat/completions', '/v1/responses'],
      converterLabels: ['Native forwarding', 'OpenAI Responses to OpenAI Chat'],
      authModeLabels: ['Header', 'Default Bearer'],
      relativeUpstreamPathCount: 1,
      fullUrlUpstreamPathCount: 1,
    })
  })

  test('导出文件名使用固定前缀和时间戳且不包含渠道信息', () => {
    const filename = getAdvancedCustomExportFilename(
      new Date('2026-07-14T08:09:10')
    )

    assert.equal(filename, 'advanced-custom-routes-20260714-080910.json')
  })
})
