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
import { useEffect, useId, useMemo, useRef, useState } from 'react'
import { Code, Plus, Table, Trash2 } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Textarea } from '@/components/ui/textarea'

type ModelMappingEditorProps = {
  value: string
  onChange: (value: string) => void
  disabled?: boolean
  sourceModelOptions?: string[]
  targetModelOptions?: string[]
}

export type MappingRow = {
  id: string
  from: string
  to: string
}

type ModelMappingParseResult =
  | {
      ok: true
      entries: Array<{ from: string; to: string }>
    }
  | {
      ok: false
      error:
        | 'Model mapping must be a valid JSON object'
        | 'Model mapping values must be strings'
        | 'Model mapping must be valid JSON format'
    }

const DUPLICATE_MAPPING_SENTINEL = '{ "duplicate_source_models": '

// 识别重复的入口模型。模型映射使用 JSON object 存储，重复 key 会被覆盖；
// 在可视化编辑时提前拦截，可以避免管理员误以为多个重定向规则都会生效。
export function getDuplicateSources(
  rows: readonly Pick<MappingRow, 'from'>[]
): string[] {
  const seen = new Set<string>()
  const duplicates = new Set<string>()

  for (const row of rows) {
    const source = row.from.trim()
    if (!source) continue
    if (seen.has(source)) {
      duplicates.add(source)
    } else {
      seen.add(source)
    }
  }

  return Array.from(duplicates)
}

// 将可视化行转换为后端保存的 JSON 字符串。空入口模型不参与保存，
// 这样新增空白行不会污染当前渠道的 model_mapping 草稿。
export function modelMappingRowsToJson(
  rows: readonly Pick<MappingRow, 'from' | 'to'>[]
): string {
  if (rows.length === 0) return ''

  const obj: Record<string, string> = {}
  rows.forEach((row) => {
    const source = row.from.trim()
    if (source) {
      obj[source] = row.to.trim()
    }
  })

  if (Object.keys(obj).length === 0) return ''
  return JSON.stringify(obj, null, 2)
}

// 解析 JSON 文本时严格要求 object<string, string>，和后端 model_mapping
// 语义保持一致；数组、null 和非字符串目标值都应在保存前提示用户修正。
export function parseModelMappingJson(json: string): ModelMappingParseResult {
  try {
    if (!json.trim()) {
      return { ok: true, entries: [] }
    }

    const parsed = JSON.parse(json)
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      return {
        ok: false,
        error: 'Model mapping must be a valid JSON object',
      }
    }

    const entries = Object.entries(parsed)
    const invalidValue = entries.find(([, to]) => typeof to !== 'string')
    if (invalidValue) {
      return {
        ok: false,
        error: 'Model mapping values must be strings',
      }
    }

    return {
      ok: true,
      entries: entries.map(([from, to]) => ({
        from,
        to: String(to),
      })),
    }
  } catch (_error) {
    return {
      ok: false,
      error: 'Model mapping must be valid JSON format',
    }
  }
}

export function ModelMappingEditor({
  value,
  onChange,
  disabled = false,
  sourceModelOptions = [],
  targetModelOptions = [],
}: ModelMappingEditorProps) {
  const { t } = useTranslation()
  const [mode, setMode] = useState<'visual' | 'json'>('visual')
  const [rows, setRows] = useState<MappingRow[]>([])
  const [jsonValue, setJsonValue] = useState(value)
  const [jsonError, setJsonError] = useState<string | null>(null)
  const editorId = useId()
  const sourceOptionsId = `${editorId}-source-models`
  const targetOptionsId = `${editorId}-target-models`
  const nextRowIdRef = useRef(0)
  const duplicateSources = useMemo(() => getDuplicateSources(rows), [rows])

  const createRowId = () => {
    nextRowIdRef.current += 1
    return `mapping-${nextRowIdRef.current}`
  }

  const parseJsonToRows = (json: string): boolean => {
    const parsed = parseModelMappingJson(json)
    if (!parsed.ok) {
      setJsonError(t(parsed.error))
      return false
    }

    setRows((previousRows) => {
      const remainingRows = [...previousRows]
      return parsed.entries.map((entry, index) => {
        const exactIndex = remainingRows.findIndex(
          (row) => row.from === entry.from && row.to === entry.to
        )
        if (exactIndex >= 0) {
          const [existing] = remainingRows.splice(exactIndex, 1)
          return existing
        }

        const sourceIndex = remainingRows.findIndex(
          (row) => row.from === entry.from
        )
        if (sourceIndex >= 0) {
          const [existing] = remainingRows.splice(sourceIndex, 1)
          return {
            id: existing.id,
            from: entry.from,
            to: entry.to,
          }
        }

        if (previousRows[index]) {
          return {
            id: previousRows[index].id,
            from: entry.from,
            to: entry.to,
          }
        }

        return {
          id: createRowId(),
          from: entry.from,
          to: entry.to,
        }
      })
    })
    setJsonError(null)
    return true
  }

  // 外部表单重置或切换渠道时，同步 JSON 草稿到可视化行。
  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setJsonValue(value)
    parseJsonToRows(value)
  }, [value])

  const syncRows = (updatedRows: MappingRow[]) => {
    setRows(updatedRows)
    const duplicates = getDuplicateSources(updatedRows)
    if (duplicates.length > 0) {
      setJsonError(t('Duplicate source model mappings are not allowed'))
      setJsonValue(DUPLICATE_MAPPING_SENTINEL)
      onChange(DUPLICATE_MAPPING_SENTINEL)
      return
    }

    const json = modelMappingRowsToJson(updatedRows)
    setJsonError(null)
    setJsonValue(json)
    onChange(json)
  }

  const handleAddRow = () => {
    const newRow: MappingRow = {
      id: createRowId(),
      from: '',
      to: '',
    }
    syncRows([...rows, newRow])
  }

  const handleDeleteRow = (id: string) => {
    syncRows(rows.filter((row) => row.id !== id))
  }

  const handleRowChange = (
    id: string,
    field: 'from' | 'to',
    newValue: string
  ) => {
    const updatedRows = rows.map((row) =>
      row.id === id ? { ...row, [field]: newValue } : row
    )
    syncRows(updatedRows)
  }

  const handleJsonChange = (newJson: string) => {
    setJsonValue(newJson)
    onChange(newJson)
    parseJsonToRows(newJson)
  }

  const handleFillTemplate = () => {
    const template = JSON.stringify(
      { 'gpt-3.5-turbo': 'gpt-3.5-turbo-0125' },
      null,
      2
    )
    setJsonValue(template)
    onChange(template)
    parseJsonToRows(template)
  }

  const handleModeChange = (nextMode: string) => {
    if (nextMode !== 'visual' && nextMode !== 'json') return

    if (nextMode === 'json') {
      const duplicates = getDuplicateSources(rows)
      if (duplicates.length === 0) {
        const json = modelMappingRowsToJson(rows)
        setJsonValue(json)
        onChange(json)
      }
      setMode('json')
      return
    }

    parseJsonToRows(jsonValue)
    setMode('visual')
  }

  return (
    <div className='flex flex-col gap-2'>
      <Tabs value={mode} onValueChange={handleModeChange} className='gap-2'>
        <div className='flex items-center justify-between gap-3'>
          <TabsList>
            <TabsTrigger value='visual'>
              <Table data-icon='inline-start' aria-hidden='true' />
              {t('Visual')}
            </TabsTrigger>
            <TabsTrigger value='json'>
              <Code data-icon='inline-start' aria-hidden='true' />
              {t('JSON')}
            </TabsTrigger>
          </TabsList>
          <Button
            type='button'
            variant='link'
            size='sm'
            className='h-auto p-0'
            onClick={handleFillTemplate}
            disabled={disabled}
          >
            {t('Fill Template')}
          </Button>
        </div>

        {jsonError && (
          <Alert variant='destructive'>
            <AlertDescription>{jsonError}</AlertDescription>
          </Alert>
        )}

        {duplicateSources.length > 0 && (
          <Alert>
            <AlertDescription>
              {t('Duplicate source model(s): {{models}}', {
                models: duplicateSources.join(', '),
              })}
            </AlertDescription>
          </Alert>
        )}

        <TabsContent value='visual' className='flex flex-col gap-2'>
          {rows.length > 0 ? (
            <div className='flex flex-col gap-2'>
              <div className='grid grid-cols-[1fr_1fr_auto] gap-2 text-sm font-medium'>
                <div>{t('Original Model')}</div>
                <div>{t('Replacement Model')}</div>
                <div className='size-8' />
              </div>
              {rows.map((row) => (
                <div
                  key={row.id}
                  className='grid grid-cols-[1fr_1fr_auto] gap-2'
                >
                  <Input
                    value={row.from}
                    onChange={(e) =>
                      handleRowChange(row.id, 'from', e.target.value)
                    }
                    placeholder='gpt-3.5-turbo'
                    disabled={disabled}
                    list={
                      sourceModelOptions.length > 0
                        ? sourceOptionsId
                        : undefined
                    }
                  />
                  <Input
                    value={row.to}
                    onChange={(e) =>
                      handleRowChange(row.id, 'to', e.target.value)
                    }
                    placeholder='gpt-3.5-turbo-0125'
                    disabled={disabled}
                    list={
                      targetModelOptions.length > 0
                        ? targetOptionsId
                        : undefined
                    }
                  />
                  <Button
                    type='button'
                    variant='ghost'
                    size='icon'
                    onClick={() => handleDeleteRow(row.id)}
                    disabled={disabled}
                    aria-label={t('Delete mapping')}
                  >
                    <Trash2 aria-hidden='true' />
                  </Button>
                </div>
              ))}
            </div>
          ) : (
            <div className='text-muted-foreground flex h-24 items-center justify-center rounded-md border border-dashed text-sm'>
              {t(
                'No model mappings configured. Click "Add Mapping" to get started.'
              )}
            </div>
          )}
          <Button
            type='button'
            variant='outline'
            size='sm'
            onClick={handleAddRow}
            disabled={disabled}
            className='w-full'
          >
            <Plus data-icon='inline-start' />
            {t('Add Mapping')}
          </Button>
        </TabsContent>
        <TabsContent value='json'>
          <Textarea
            value={jsonValue}
            onChange={(e) => handleJsonChange(e.target.value)}
            placeholder={t('{"original-model": "replacement-model"}')}
            disabled={disabled}
            rows={8}
            className={cn(
              'font-mono text-sm',
              jsonError && 'border-destructive'
            )}
            aria-invalid={Boolean(jsonError)}
          />
        </TabsContent>
      </Tabs>

      {sourceModelOptions.length > 0 && (
        <datalist id={sourceOptionsId}>
          {sourceModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
      {targetModelOptions.length > 0 && (
        <datalist id={targetOptionsId}>
          {targetModelOptions.map((model) => (
            <option key={model} value={model} />
          ))}
        </datalist>
      )}
    </div>
  )
}
