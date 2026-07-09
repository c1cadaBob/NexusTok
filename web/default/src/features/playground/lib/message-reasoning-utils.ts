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
export type ParsedThinkTags = {
  visibleContent: string
  reasoning: string
  hasUnclosedTag: boolean
}

/**
 * 解析 `<think>` 内容，把推理过程从可见正文中拆出来。
 *
 * 兼容完整和未闭合的 `<think>` 标签，流式过程中未闭合部分会进入
 * reasoning buffer，而不是提前显示到正文里。
 */
export function parseThinkTags(content: string): ParsedThinkTags {
  if (!content.includes('<think>')) {
    return { visibleContent: content, reasoning: '', hasUnclosedTag: false }
  }

  const visibleParts: string[] = []
  const reasoningParts: string[] = []
  let currentPos = 0
  let hasUnclosedTag = false

  while (true) {
    const openPos = content.indexOf('<think>', currentPos)

    if (openPos === -1) {
      if (currentPos < content.length) {
        visibleParts.push(content.slice(currentPos))
      }
      break
    }

    if (openPos > currentPos) {
      visibleParts.push(content.slice(currentPos, openPos))
    }

    const closePos = content.indexOf('</think>', openPos + 7)

    if (closePos === -1) {
      reasoningParts.push(content.slice(openPos + 7))
      hasUnclosedTag = true
      break
    }

    reasoningParts.push(content.slice(openPos + 7, closePos))
    currentPos = closePos + 8
  }

  return {
    visibleContent: visibleParts.join('').trim(),
    reasoning: reasoningParts.join('\n\n').trim(),
    hasUnclosedTag,
  }
}
