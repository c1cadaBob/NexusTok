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
  canCreateMultiSelectValue,
  dedupeMultiSelectValues,
  filterMultiSelectItems,
  getNewMultiSelectValues,
  getVisibleMultiSelectItems,
  shouldClearMultiSelectSearchAfterChange,
  shouldPreventMultiSelectEnterFormSubmit,
  shouldPreventEmptyInputChipRemoval,
  shouldRestoreMultiSelectSearchAfterSelection,
  shouldSubmitMultiSelectSearchOnEnter,
} from './multi-select'

describe('MultiSelect 搜索过滤', () => {
  test('空搜索保持原候选顺序', () => {
    const items = ['gpt-4o', 'gpt-5.6-terra', 'claude-sonnet']

    assert.deepEqual(filterMultiSelectItems(items, ''), items)
  })

  test('按模型名过滤候选', () => {
    const items = [
      'gpt-5.6-terra',
      'gpt-5.6-luna',
      'gpt-5.6-sol',
      'gpt-5.4',
      'gpt-5.5',
    ]

    assert.deepEqual(filterMultiSelectItems(items, 'gpt-5.6'), [
      'gpt-5.6-terra',
      'gpt-5.6-luna',
      'gpt-5.6-sol',
    ])
  })

  test('兼容 label 命中', () => {
    const items = ['model-a', 'model-b']
    const labels = new Map([
      ['model-a', 'GPT-5.6 Terra'],
      ['model-b', 'Claude Sonnet'],
    ])

    assert.deepEqual(filterMultiSelectItems(items, 'terra', labels), [
      'model-a',
    ])
  })

  test('搜索时可隐藏已选项但保留可新增候选', () => {
    const items = ['gpt-5.6-terra', 'gpt-5.6-luna', 'gpt-5.6-sol', 'gpt-5.4']

    assert.deepEqual(
      getVisibleMultiSelectItems({
        items,
        inputValue: 'gpt-5.6',
        hideSelectedOptionsWhenSearching: true,
        selected: ['gpt-5.6-sol'],
      }),
      ['gpt-5.6-terra', 'gpt-5.6-luna']
    )
  })

  test('空搜索时仍显示已选项方便管理', () => {
    const items = ['gpt-5.6-terra', 'gpt-5.6-sol']

    assert.deepEqual(
      getVisibleMultiSelectItems({
        items,
        inputValue: '',
        hideSelectedOptionsWhenSearching: true,
        selected: ['gpt-5.6-sol'],
      }),
      items
    )
  })

  test('模型仓库模式空搜索时排除已选项', () => {
    const items = ['gpt-5.5', 'gpt-5.6-terra', 'claude-sonnet']

    assert.deepEqual(
      getVisibleMultiSelectItems({
        items,
        inputValue: '',
        hideSelectedOptionsWhenSearching: false,
        excludeSelectedOptions: true,
        selected: ['GPT-5.5'],
      }),
      ['gpt-5.6-terra', 'claude-sonnet']
    )
  })

  test('模型仓库模式可使用自定义过滤器并继续排除已选项', () => {
    const items = ['gpt-5.5', 'gpt-5.6-terra', 'GPT-4.1', 'claude-sonnet']
    const textFilter = (values: string[], inputValue: string) =>
      values.filter((value) =>
        value.toLowerCase().includes(inputValue.trim().toLowerCase())
      )

    assert.deepEqual(
      getVisibleMultiSelectItems({
        items,
        inputValue: 'GPT',
        hideSelectedOptionsWhenSearching: false,
        excludeSelectedOptions: true,
        selected: ['gpt-5.5'],
        filterItems: textFilter,
      }),
      ['gpt-5.6-terra', 'GPT-4.1']
    )
  })
})

describe('MultiSelect 值去重', () => {
  test('按 trim + 大小写不敏感方式去重并保留首次展示形式', () => {
    assert.deepEqual(
      dedupeMultiSelectValues([
        ' gpt-5.6-sol ',
        'GPT-5.6-SOL',
        '',
        'gpt-5.6-luna',
      ]),
      ['gpt-5.6-sol', 'gpt-5.6-luna']
    )
  })

  test('追加候选时不会把大小写不同的已选模型当作新增项', () => {
    assert.deepEqual(
      getNewMultiSelectValues({
        selected: ['gpt-5.6-sol'],
        incoming: ['GPT-5.6-SOL', 'gpt-5.6-terra', ' gpt-5.6-luna '],
      }),
      ['gpt-5.6-terra', 'gpt-5.6-luna']
    )
  })
})

describe('MultiSelect 选中后搜索词清理', () => {
  test('默认在新增选中项后清空搜索词', () => {
    assert.equal(
      shouldClearMultiSelectSearchAfterChange({
        clearSearchOnSelect: true,
        previousSelectedLength: 1,
        nextSelectedLength: 2,
      }),
      true
    )
  })

  test('调用方可关闭自动清空，便于连续选择同系列候选', () => {
    assert.equal(
      shouldClearMultiSelectSearchAfterChange({
        clearSearchOnSelect: false,
        previousSelectedLength: 1,
        nextSelectedLength: 2,
      }),
      false
    )
  })

  test('没有新增选中项时不触发清空', () => {
    assert.equal(
      shouldClearMultiSelectSearchAfterChange({
        clearSearchOnSelect: true,
        previousSelectedLength: 2,
        nextSelectedLength: 2,
      }),
      false
    )
  })

  test('关闭自动清空时新增选中项会恢复搜索词', () => {
    assert.equal(
      shouldRestoreMultiSelectSearchAfterSelection({
        clearSearchOnSelect: false,
        inputValue: 'gpt-5.6',
        previousSelectedLength: 1,
        nextSelectedLength: 2,
      }),
      true
    )
  })

  test('默认自动清空或空搜索不会恢复搜索词', () => {
    assert.equal(
      shouldRestoreMultiSelectSearchAfterSelection({
        clearSearchOnSelect: true,
        inputValue: 'gpt-5.6',
        previousSelectedLength: 1,
        nextSelectedLength: 2,
      }),
      false
    )
    assert.equal(
      shouldRestoreMultiSelectSearchAfterSelection({
        clearSearchOnSelect: false,
        inputValue: '   ',
        previousSelectedLength: 1,
        nextSelectedLength: 2,
      }),
      false
    )
  })
})

describe('MultiSelect 空搜索删除键保护', () => {
  test('启用保护时空输入 Backspace 不删除已选项', () => {
    assert.equal(
      shouldPreventEmptyInputChipRemoval({
        preserveSelectedOnEmptyRemovalKey: true,
        inputValue: '',
        key: 'Backspace',
        selectedLength: 3,
      }),
      true
    )
  })

  test('启用保护时空输入 Delete 不删除已选项', () => {
    assert.equal(
      shouldPreventEmptyInputChipRemoval({
        preserveSelectedOnEmptyRemovalKey: true,
        inputValue: '',
        key: 'Delete',
        selectedLength: 3,
      }),
      true
    )
  })

  test('存在搜索词时仍允许删除键清空输入内容', () => {
    assert.equal(
      shouldPreventEmptyInputChipRemoval({
        preserveSelectedOnEmptyRemovalKey: true,
        inputValue: 'gpt-5.6',
        key: 'Backspace',
        selectedLength: 3,
      }),
      false
    )
  })

  test('默认关闭保护以保持其它 MultiSelect 的键盘行为', () => {
    assert.equal(
      shouldPreventEmptyInputChipRemoval({
        preserveSelectedOnEmptyRemovalKey: false,
        inputValue: '',
        key: 'Backspace',
        selectedLength: 3,
      }),
      false
    )
  })
})

describe('MultiSelect 搜索提交键盘行为', () => {
  test('已有候选但没有高亮时 Enter 不批量提交搜索结果', () => {
    assert.equal(
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches: true,
        hasSearchSubmit: true,
        key: 'Enter',
        inputValue: 'gpt-5.6',
        isLoading: false,
      }),
      false
    )
  })

  test('已有高亮候选时 Enter 保留给候选选择', () => {
    assert.equal(
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches: true,
        hasSearchSubmit: true,
        key: 'Enter',
        inputValue: 'gpt-5.6',
        isLoading: false,
        hasHighlightedOption: true,
      }),
      false
    )
  })

  test('搜索仍在加载且没有可创建值时 Enter 交给搜索提交', () => {
    assert.equal(
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches: true,
        hasSearchSubmit: true,
        key: 'Enter',
        inputValue: 'gpt-5.6',
        isLoading: true,
      }),
      true
    )
  })

  test('搜索候选存在但输入可创建时，Enter 保留给自定义创建', () => {
    assert.equal(
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches: true,
        hasSearchSubmit: true,
        key: 'Enter',
        inputValue: 'gpt-5.6',
        isLoading: false,
        canCreateValue: true,
      }),
      false
    )
  })

  test('默认关闭，避免影响其它 MultiSelect 的键盘选择', () => {
    assert.equal(
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches: false,
        hasSearchSubmit: true,
        key: 'Enter',
        inputValue: 'gpt-5.6',
        isLoading: false,
      }),
      false
    )
  })

  test('没有搜索提交处理器、空输入或非 Enter 键时不接管', () => {
    assert.equal(
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches: true,
        hasSearchSubmit: false,
        key: 'Enter',
        inputValue: 'gpt-5.6',
        isLoading: false,
      }),
      false
    )
    assert.equal(
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches: true,
        hasSearchSubmit: true,
        key: 'Enter',
        inputValue: '   ',
        isLoading: true,
      }),
      false
    )
    assert.equal(
      shouldSubmitMultiSelectSearchOnEnter({
        submitSearchOnEnterWithMatches: true,
        hasSearchSubmit: true,
        key: 'Tab',
        inputValue: 'gpt-5.6',
        isLoading: true,
      }),
      false
    )
  })
})

describe('MultiSelect 表单提交防护', () => {
  test('存在搜索词时 Enter 不应冒泡提交父表单', () => {
    assert.equal(
      shouldPreventMultiSelectEnterFormSubmit({
        key: 'Enter',
        inputValue: 'gpt-5.6',
      }),
      true
    )
  })

  test('空输入或非 Enter 键不拦截父级默认行为', () => {
    assert.equal(
      shouldPreventMultiSelectEnterFormSubmit({
        key: 'Enter',
        inputValue: '   ',
      }),
      false
    )
    assert.equal(
      shouldPreventMultiSelectEnterFormSubmit({
        key: 'Tab',
        inputValue: 'gpt-5.6',
      }),
      false
    )
  })
})

describe('MultiSelect 自定义添加判定', () => {
  const options = [
    { value: 'gpt-5.6-terra', label: 'gpt-5.6-terra' },
    { value: 'gpt-5.6-luna', label: 'gpt-5.6-luna' },
    { value: 'gpt-5.6-sol', label: 'gpt-5.6-sol' },
  ]

  test('部分匹配候选不阻止添加完整自定义值', () => {
    assert.equal(
      canCreateMultiSelectValue({
        allowCreate: true,
        inputValue: 'gpt-5.6',
        selected: [],
        options,
      }),
      true
    )
  })

  test('调用方可要求存在候选时隐藏自定义添加', () => {
    assert.equal(
      canCreateMultiSelectValue({
        allowCreate: true,
        inputValue: 'gpt-5.6',
        selected: [],
        options,
        allowCreateWithMatches: false,
        hasMatchingOption: true,
      }),
      false
    )
  })

  test('禁止匹配候选创建时仍允许真正无候选的自定义值', () => {
    assert.equal(
      canCreateMultiSelectValue({
        allowCreate: true,
        inputValue: 'custom-model',
        selected: [],
        options,
        allowCreateWithMatches: false,
        hasMatchingOption: false,
      }),
      true
    )
  })

  test('允许在搜索加载中继续自定义创建时，不会被 loading 强制吞掉', () => {
    assert.equal(
      canCreateMultiSelectValue({
        allowCreate: true,
        inputValue: 'custom-model',
        selected: [],
        options,
        isLoading: false,
      }),
      true
    )
    assert.equal(
      canCreateMultiSelectValue({
        allowCreate: true,
        inputValue: 'custom-model',
        selected: [],
        options,
        isLoading: true,
      }),
      false
    )
  })

  test('精确重复候选会阻止创建', () => {
    assert.equal(
      canCreateMultiSelectValue({
        allowCreate: true,
        inputValue: 'gpt-5.6-sol',
        selected: [],
        options,
      }),
      false
    )
  })

  test('已选值会阻止重复创建', () => {
    assert.equal(
      canCreateMultiSelectValue({
        allowCreate: true,
        inputValue: 'custom-model',
        selected: ['custom-model'],
        options,
      }),
      false
    )
  })

  test('已选值大小写不同也会阻止重复创建', () => {
    assert.equal(
      canCreateMultiSelectValue({
        allowCreate: true,
        inputValue: 'Custom-Model',
        selected: ['custom-model'],
        options,
      }),
      false
    )
  })
})
