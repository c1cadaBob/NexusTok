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
import { resolveCodeBlockDisplayState } from './code-block'

function makeCode(lines: number) {
  return Array.from({ length: lines }, (_, index) => `line ${index + 1}`).join(
    '\n'
  )
}

describe('resolveCodeBlockDisplayState', () => {
  test('长代码按预览行数默认折叠', () => {
    const state = resolveCodeBlockDisplayState({
      code: makeCode(20),
      collapsedLines: 14,
      isCollapsed: true,
      language: 'ts',
      maxExpandedLines: 44,
    })

    assert.equal(state.lineCount, 20)
    assert.equal(state.previewLines, 14)
    assert.equal(state.canCollapse, true)
    assert.equal(state.isCodeCollapsed, true)
    assert.equal(state.bodyMaxHeight, '23rem')
    assert.equal(state.displayLanguage, 'typescript')
    assert.equal(state.downloadFilename, 'code.typescript')
  })

  test('展开长代码后使用最大展开高度', () => {
    const state = resolveCodeBlockDisplayState({
      code: makeCode(20),
      collapsedLines: 14,
      isCollapsed: false,
      language: 'javascript',
      maxExpandedLines: 44,
    })

    assert.equal(state.canCollapse, true)
    assert.equal(state.isCodeCollapsed, false)
    assert.equal(state.bodyMaxHeight, '68rem')
  })

  test('短代码不会出现折叠状态', () => {
    const state = resolveCodeBlockDisplayState({
      code: makeCode(4),
      collapsedLines: 14,
      isCollapsed: true,
      language: 'plaintext',
    })

    assert.equal(state.canCollapse, false)
    assert.equal(state.isCodeCollapsed, false)
    assert.equal(state.bodyMaxHeight, undefined)
  })

  test('可显式关闭折叠并使用自定义下载文件名', () => {
    const state = resolveCodeBlockDisplayState({
      code: makeCode(80),
      enableCollapse: false,
      filename: 'trace.json',
      isCollapsed: true,
      language: 'json',
      maxExpandedLines: 30,
    })

    assert.equal(state.canCollapse, false)
    assert.equal(state.isCodeCollapsed, false)
    assert.equal(state.bodyMaxHeight, '47rem')
    assert.equal(state.downloadFilename, 'trace.json')
  })
})
