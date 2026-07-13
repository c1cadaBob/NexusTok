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
import { renderMarkdownForTest } from './markdown'

describe('renderMarkdownForTest', () => {
  test('渲染 KaTeX 块级公式', () => {
    const html = renderMarkdownForTest('$$x^2 + y^2 = z^2$$')

    assert.match(html, /katex/)
    assert.match(html, /x/)
    assert.match(html, /z/)
  })

  test('渲染 flow 代码块为安全 SVG 图表', () => {
    const html = renderMarkdownForTest(
      [
        '```flow',
        'start=>start: Start',
        'check=>condition: Ready?',
        'done=>end: Done',
        'start->check(yes)->done',
        '```',
      ].join('\n')
    )

    assert.match(html, /data-diagram="flow"/)
    assert.match(html, /markdown-diagram-node/)
    assert.match(html, /Ready\?/)
  })

  test('清理危险 HTML 与 javascript URL', () => {
    const html = renderMarkdownForTest(
      '<img src=x onerror="alert(1)"><script>alert(1)</script><a href="javascript:alert(1)">bad</a>'
    )

    assert.doesNotMatch(html, /script/i)
    assert.doesNotMatch(html, /onerror/i)
    assert.doesNotMatch(html, /javascript:/i)
    assert.match(html, />bad</)
  })

  test('保留 breaks 换行语义', () => {
    const html = renderMarkdownForTest('line one\nline two', true)

    assert.match(html, /line one<br>line two/)
  })
})
