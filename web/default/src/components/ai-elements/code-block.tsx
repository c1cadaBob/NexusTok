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
/* eslint-disable react-refresh/only-export-components */
'use client'

import {
  type ComponentProps,
  createContext,
  type CSSProperties,
  type HTMLAttributes,
  type ReactNode,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { markdown } from '@codemirror/lang-markdown'
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language'
import { EditorState, type Extension } from '@codemirror/state'
import { EditorView, lineNumbers } from '@codemirror/view'
import { tags as highlightTags } from '@lezer/highlight'
import type { Element } from 'hast'
import {
  CheckIcon,
  ChevronDownIcon,
  ChevronRightIcon,
  CopyIcon,
  DownloadIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import {
  type BundledLanguage,
  codeToHtml,
  type ShikiTransformer,
} from 'shiki/bundle/web'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'

type CodeBlockProps = HTMLAttributes<HTMLDivElement> & {
  code: string
  collapsedLines?: number
  defaultCollapsed?: boolean
  enableCollapse?: boolean
  filename?: string
  language: BundledLanguage | string
  maxExpandedLines?: number
  /** @deprecated use collapsedLines for collapsed preview height. */
  maxCollapsedLines?: number
  showLineNumbers?: boolean
  showToolbar?: boolean
  title?: ReactNode
}

type CodeBlockEditorProps = Omit<
  HTMLAttributes<HTMLDivElement>,
  'onChange' | 'onKeyDown' | 'title'
> & {
  actions?: ReactNode
  ariaLabel: string
  autoFocus?: boolean
  language: BundledLanguage | string
  onChange: (value: string) => void
  onKeyDown?: (event: globalThis.KeyboardEvent) => void
  readOnly?: boolean
  rows?: number
  title?: ReactNode
  value: string
}

type CodeMirrorCodeViewProps = {
  ariaLabel: string
  autoFocus?: boolean
  language: BundledLanguage | string
  onChange: (value: string) => void
  onKeyDown?: (event: globalThis.KeyboardEvent) => void
  readOnly?: boolean
  rows?: number
  showLineNumbers?: boolean
  value: string
}

type CodeBlockFrameProps = Omit<HTMLAttributes<HTMLDivElement>, 'title'> & {
  bodyClassName?: string
  bodyMaxHeight?: string
  bodyOverlay?: ReactNode
  children: ReactNode
  endActions?: ReactNode
  showToolbar?: boolean
  title?: ReactNode
}

type CodeBlockDisplayStateOptions = {
  code: string
  collapsedLines?: number
  enableCollapse?: boolean
  filename?: string
  isCollapsed?: boolean
  language: BundledLanguage | string
  maxCollapsedLines?: number
  maxExpandedLines?: number
}

type CodeBlockContextType = {
  code: string
}

const CodeBlockContext = createContext<CodeBlockContextType>({
  code: '',
})

const CODE_MIRROR_MONO_FONT =
  "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace"

const codeMirrorTheme = EditorView.theme({
  '&': {
    background: 'transparent',
    color: 'var(--foreground)',
    fontSize: '13px',
  },
  '.cm-content': {
    caretColor: 'var(--foreground)',
    fontFamily: CODE_MIRROR_MONO_FONT,
    lineHeight: '1.5rem',
    minHeight: 'var(--code-editor-min-height)',
    minWidth: 'max-content',
    padding: '1rem 1rem 1rem 0',
  },
  '.cm-editor': {
    background: 'transparent',
    width: '100%',
  },
  '.cm-focused': {
    outline: 'none',
  },
  '.cm-gutters': {
    background: 'transparent',
    borderRight: '0',
    color: 'var(--muted-foreground)',
    fontFamily: CODE_MIRROR_MONO_FONT,
    fontSize: '13px',
    lineHeight: '1.5rem',
    padding: '1rem 1rem 1rem 0',
  },
  '.cm-gutters:empty': {
    display: 'none',
  },
  '.cm-lineNumbers .cm-gutterElement': {
    minWidth: '2.5rem',
    padding: '0 1rem 0 0',
    textAlign: 'right',
  },
  '.cm-line': {
    padding: '0',
  },
  '.cm-scroller': {
    fontFamily: CODE_MIRROR_MONO_FONT,
    lineHeight: '1.5rem',
    minHeight: 'var(--code-editor-min-height)',
    overflow: 'auto',
  },
  '.cm-selectionBackground': {
    background:
      'color-mix(in oklch, var(--primary) 28%, transparent) !important',
  },
})

const codeMirrorHighlightStyle = syntaxHighlighting(
  HighlightStyle.define([
    { tag: highlightTags.heading, color: 'var(--primary)', fontWeight: '600' },
    {
      tag: [highlightTags.strong, highlightTags.emphasis],
      color: 'var(--warning)',
    },
    { tag: [highlightTags.link, highlightTags.url], color: 'var(--info)' },
    {
      tag: [highlightTags.monospace, highlightTags.contentSeparator],
      color: 'var(--success)',
    },
    {
      tag: [highlightTags.keyword, highlightTags.processingInstruction],
      color: 'color-mix(in oklch, var(--primary) 70%, var(--foreground))',
    },
    {
      tag: [highlightTags.atom, highlightTags.bool, highlightTags.number],
      color: 'var(--warning)',
    },
    {
      tag: [highlightTags.string, highlightTags.inserted],
      color: 'var(--success)',
    },
    {
      tag: [highlightTags.deleted, highlightTags.invalid],
      color: 'var(--destructive)',
    },
    {
      tag: [highlightTags.meta, highlightTags.comment],
      color: 'var(--muted-foreground)',
    },
  ])
)

const lineNumberTransformer: ShikiTransformer = {
  name: 'line-numbers',
  line(node: Element, line: number) {
    node.children.unshift({
      type: 'element',
      tagName: 'span',
      properties: {
        className: [
          'inline-block',
          'min-w-10',
          'mr-4',
          'text-right',
          'select-none',
          'text-muted-foreground',
        ],
      },
      children: [{ type: 'text', value: String(line) }],
    })
  },
}

export async function highlightCode(
  code: string,
  language: BundledLanguage | string,
  showLineNumbers = false
) {
  const transformers: ShikiTransformer[] = showLineNumbers
    ? [lineNumberTransformer]
    : []

  const requestedLanguage = normalizeCodeLanguage(language)

  try {
    return await codeToHtml(code, {
      lang: requestedLanguage,
      themes: {
        light: 'one-light',
        dark: 'one-dark-pro',
      },
      transformers,
    })
  } catch {
    // 模型输出的 fence 语言可能不是 Shiki 支持项；回退 plaintext，保证响应区不被高亮错误拖垮。
    return codeToHtml(code, {
      lang: 'plaintext',
      themes: {
        light: 'one-light',
        dark: 'one-dark-pro',
      },
      transformers,
    })
  }
}

function normalizeCodeLanguage(language: BundledLanguage | string) {
  const normalized = String(language || 'plaintext')
    .trim()
    .toLowerCase()

  if (!/^[a-z0-9][a-z0-9+#._-]{0,31}$/i.test(normalized)) {
    return 'plaintext'
  }

  if (normalized === 'text' || normalized === 'plain') {
    return 'plaintext'
  }

  if (normalized === 'shell' || normalized === 'sh') {
    return 'bash'
  }

  if (normalized === 'js') {
    return 'javascript'
  }

  if (normalized === 'ts') {
    return 'typescript'
  }

  if (normalized === 'golang') {
    return 'go'
  }

  return normalized
}

function getCodeLineCount(code: string) {
  if (!code) {
    return 1
  }

  return code.split('\n').length
}

function getDownloadFilename(language: string, filename?: string) {
  if (filename) {
    return filename
  }

  const extension = language === 'plaintext' ? 'txt' : language
  return `code.${extension}`
}

function getCodeBlockHeight(lines: number) {
  return `${Math.max(4, lines) * 1.5 + 2}rem`
}

function getCodeBlockMaxHeight(
  isCodeCollapsed: boolean,
  previewLines: number,
  maxExpandedLines?: number
): string | undefined {
  if (isCodeCollapsed) {
    return getCodeBlockHeight(previewLines)
  }

  if (maxExpandedLines) {
    return getCodeBlockHeight(maxExpandedLines)
  }

  return undefined
}

// resolveCodeBlockDisplayState 集中计算只读代码块展示状态，避免渲染层和测试对折叠规则各算一套。
export function resolveCodeBlockDisplayState({
  code,
  collapsedLines = 12,
  enableCollapse = true,
  filename,
  isCollapsed = false,
  language,
  maxCollapsedLines,
  maxExpandedLines,
}: CodeBlockDisplayStateOptions) {
  const displayLanguage = normalizeCodeLanguage(language)
  const lineCount = getCodeLineCount(code)
  const previewLines = maxCollapsedLines ?? collapsedLines
  const canCollapse = enableCollapse && lineCount > previewLines
  const isCodeCollapsed = canCollapse && isCollapsed
  const bodyMaxHeight = getCodeBlockMaxHeight(
    isCodeCollapsed,
    previewLines,
    maxExpandedLines
  )

  return {
    bodyMaxHeight,
    canCollapse,
    displayLanguage,
    downloadFilename: getDownloadFilename(displayLanguage, filename),
    isCodeCollapsed,
    lineCount,
    previewLines,
  }
}

/**
 * 根据代码语言选择 CodeMirror 扩展。
 *
 * 当前只为 Markdown 编辑启用专用语法能力；其他语言仍使用通用文本编辑，
 * 避免一次性引入多语言 parser 依赖和额外包体。
 */
function getCodeMirrorLanguageExtension(language: BundledLanguage | string) {
  const requestedLanguage = normalizeCodeLanguage(language)
  if (
    requestedLanguage === 'markdown' ||
    requestedLanguage === 'md' ||
    requestedLanguage === 'mdx'
  ) {
    return markdown()
  }

  return []
}

/**
 * 组装 CodeMirror 扩展。
 *
 * 这里统一挂载主题、基础 tab 设置和换行能力；键盘事件在视图 DOM
 * 单独绑定，避免调用方回调变化时重建 EditorView。
 */
function getCodeMirrorExtensions(options: {
  language: BundledLanguage | string
  readOnly: boolean
  showLineNumbers: boolean
}): Extension[] {
  const extensions: Extension[] = [
    getCodeMirrorLanguageExtension(options.language),
    codeMirrorHighlightStyle,
    codeMirrorTheme,
    EditorState.tabSize.of(2),
    EditorView.lineWrapping,
  ]

  if (options.showLineNumbers) {
    extensions.unshift(lineNumbers())
  }

  if (options.readOnly) {
    extensions.push(
      EditorState.readOnly.of(true),
      EditorView.editable.of(false)
    )
  }

  return extensions
}

/**
 * 受控的 CodeMirror 视图。
 *
 * 首次挂载时创建 EditorView，卸载时必须销毁；外部 value 变化时通过
 * dispatch 同步到编辑器，保证 Reset、切换编辑消息等外部状态能回写。
 */
function CodeMirrorCodeView({
  ariaLabel,
  autoFocus = false,
  language,
  onChange,
  onKeyDown,
  readOnly = false,
  rows = 8,
  showLineNumbers = true,
  value,
}: CodeMirrorCodeViewProps) {
  const editorHostRef = useRef<HTMLDivElement>(null)
  const editorViewRef = useRef<EditorView | null>(null)
  const latestValueRef = useRef(value)
  const onChangeRef = useRef(onChange)
  const editorMinHeight = `${Math.max(4, rows) * 1.5 + 2}rem`
  const editorExtensions = useMemo(
    () =>
      getCodeMirrorExtensions({
        language,
        readOnly,
        showLineNumbers,
      }),
    [language, readOnly, showLineNumbers]
  )

  useEffect(() => {
    latestValueRef.current = value
  }, [value])

  useEffect(() => {
    onChangeRef.current = onChange
  }, [onChange])

  useEffect(() => {
    const editorHost = editorHostRef.current
    if (!editorHost) return

    // EditorView 直接管理 DOM，组件卸载时必须 destroy，避免遗留监听器。
    const editorView = new EditorView({
      doc: latestValueRef.current,
      extensions: [
        ...editorExtensions,
        EditorView.updateListener.of((update) => {
          if (update.docChanged) {
            onChangeRef.current(update.state.doc.toString())
          }
        }),
      ],
      parent: editorHost,
    })
    editorViewRef.current = editorView

    if (autoFocus) {
      editorView.focus()
    }

    return () => {
      editorView.destroy()
      editorViewRef.current = null
    }
  }, [autoFocus, editorExtensions])

  useEffect(() => {
    const editorView = editorViewRef.current
    if (!editorView || !onKeyDown) return

    const handleEditorKeyDown = (event: KeyboardEvent) => {
      onKeyDown(event)
    }

    // 键盘事件单独挂载，避免每次回调变化都重建 CodeMirror 实例。
    editorView.dom.addEventListener('keydown', handleEditorKeyDown, true)
    return () => {
      editorView.dom.removeEventListener('keydown', handleEditorKeyDown, true)
    }
  }, [editorExtensions, onKeyDown])

  useEffect(() => {
    const editorView = editorViewRef.current
    if (!editorView) return

    const currentValue = editorView.state.doc.toString()
    if (currentValue === value) return

    // 外部状态可能来自 Reset 或切换编辑消息，这里把受控值同步回编辑器。
    editorView.dispatch({
      changes: {
        from: 0,
        to: editorView.state.doc.length,
        insert: value,
      },
    })
  }, [value])

  return (
    <div
      aria-label={ariaLabel}
      aria-multiline='true'
      aria-readonly={readOnly ? 'true' : undefined}
      className='min-h-(--code-editor-min-height)'
      ref={editorHostRef}
      role='textbox'
      style={
        {
          '--code-editor-min-height': editorMinHeight,
        } as CSSProperties
      }
    />
  )
}

export const CodeBlockFrame = ({
  bodyClassName,
  bodyMaxHeight,
  bodyOverlay,
  children,
  className,
  endActions,
  showToolbar = false,
  title,
  ...props
}: CodeBlockFrameProps) => (
  <div
    className={cn(
      'group/code-block bg-background text-foreground my-0 w-full max-w-full overflow-hidden rounded-lg border shadow-xs',
      className
    )}
    {...props}
  >
    {showToolbar && (
      <div className='bg-muted/35 border-border/70 flex min-h-10 items-center gap-2 border-b px-2 py-1.5'>
        <div className='min-w-0 flex-1'>
          <div className='text-muted-foreground truncate font-mono text-[11px] font-medium tracking-wide uppercase'>
            {title}
          </div>
        </div>
        {endActions && (
          <div className='flex shrink-0 items-center gap-1'>{endActions}</div>
        )}
      </div>
    )}
    <div className='relative min-w-0'>
      <div
        className={cn(
          'code-block-scroll max-w-full overflow-auto transition-[max-height] duration-200 ease-out',
          bodyClassName
        )}
        style={{ maxHeight: bodyMaxHeight }}
      >
        {children}
      </div>
      {bodyOverlay}
    </div>
  </div>
)

export const CodeBlock = ({
  code,
  collapsedLines = 12,
  defaultCollapsed,
  enableCollapse = true,
  filename,
  language,
  maxExpandedLines,
  maxCollapsedLines,
  showLineNumbers = false,
  showToolbar = false,
  title,
  className,
  children,
  ...props
}: CodeBlockProps) => {
  const { t } = useTranslation()
  const [html, setHtml] = useState<string>('')
  const [isCollapsed, setIsCollapsed] = useState(Boolean(defaultCollapsed))
  const displayState = useMemo(
    () =>
      resolveCodeBlockDisplayState({
        code,
        collapsedLines,
        enableCollapse,
        filename,
        isCollapsed,
        language,
        maxCollapsedLines,
        maxExpandedLines,
      }),
    [
      code,
      collapsedLines,
      enableCollapse,
      filename,
      isCollapsed,
      language,
      maxCollapsedLines,
      maxExpandedLines,
    ]
  )
  const {
    bodyMaxHeight,
    canCollapse,
    displayLanguage,
    downloadFilename,
    isCodeCollapsed,
  } = displayState
  const displayTitle = title ?? displayLanguage

  useEffect(() => {
    let cancelled = false
    highlightCode(code, language, showLineNumbers).then((next) => {
      if (!cancelled) {
        setHtml(next)
      }
    })
    return () => {
      cancelled = true
    }
  }, [code, language, showLineNumbers])

  const downloadCode = () => {
    if (typeof window === 'undefined' || typeof document === 'undefined') {
      return
    }

    const blob = new Blob([code], { type: 'text/plain;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = downloadFilename
    anchor.click()
    URL.revokeObjectURL(url)
  }

  return (
    <CodeBlockContext.Provider value={{ code }}>
      <CodeBlockFrame
        bodyClassName='p-0'
        bodyMaxHeight={bodyMaxHeight}
        bodyOverlay={
          <>
            {isCodeCollapsed && (
              <div className='from-background/0 to-background pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-linear-to-b' />
            )}
            {!showToolbar && children && (
              <div className='absolute top-2 right-2 flex items-center gap-1'>
                {children}
              </div>
            )}
          </>
        }
        className={cn('my-3', className)}
        endActions={
          <>
            {canCollapse && (
              <Tooltip>
                <TooltipTrigger
                  render={
                    <Button
                      aria-label={isCodeCollapsed ? t('Expand') : t('Collapse')}
                      onClick={() => setIsCollapsed((value) => !value)}
                      size='icon-sm'
                      type='button'
                      variant='ghost'
                    >
                      {isCodeCollapsed ? (
                        <ChevronRightIcon />
                      ) : (
                        <ChevronDownIcon />
                      )}
                    </Button>
                  }
                />
                <TooltipContent>
                  <p>{isCodeCollapsed ? t('Expand') : t('Collapse')}</p>
                </TooltipContent>
              </Tooltip>
            )}
            {showToolbar && children}
            <Tooltip>
              <TooltipTrigger
                render={
                  <Button
                    aria-label={t('Download')}
                    onClick={downloadCode}
                    size='icon-sm'
                    type='button'
                    variant='ghost'
                  >
                    <DownloadIcon />
                  </Button>
                }
              />
              <TooltipContent>
                <p>{t('Download')}</p>
              </TooltipContent>
            </Tooltip>
          </>
        }
        showToolbar={showToolbar}
        title={displayTitle}
        {...props}
      >
        <div
          className='[&>pre]:bg-background! [&>pre]:text-foreground! overflow-hidden [&_code]:font-mono [&_code]:text-sm [&>pre]:m-0 [&>pre]:p-4 [&>pre]:text-sm'
          // biome-ignore lint/security/noDangerouslySetInnerHtml: "this is needed."
          dangerouslySetInnerHTML={{ __html: html }}
        />
      </CodeBlockFrame>
    </CodeBlockContext.Provider>
  )
}

/**
 * 面向结构化文本编辑的代码块编辑器。
 *
 * 只负责编辑态 frame 和 CodeMirror 宿主；只读代码展示继续使用上方
 * `CodeBlock` 的 Shiki 渲染路径，降低公共展示链路的回归风险。
 */
export const CodeBlockEditor = ({
  actions,
  ariaLabel,
  autoFocus = true,
  className,
  language,
  onChange,
  onKeyDown,
  readOnly = false,
  rows = 8,
  title,
  value,
  ...props
}: CodeBlockEditorProps) => (
  <CodeBlockFrame
    bodyClassName='p-0'
    className={cn('my-0', className)}
    endActions={actions}
    showToolbar={Boolean(title || actions)}
    title={title}
    {...props}
  >
    <CodeMirrorCodeView
      ariaLabel={ariaLabel}
      autoFocus={autoFocus}
      language={language}
      onChange={onChange}
      onKeyDown={onKeyDown}
      readOnly={readOnly}
      rows={rows}
      showLineNumbers
      value={value}
    />
  </CodeBlockFrame>
)

export type CodeBlockCopyButtonProps = ComponentProps<typeof Button> & {
  onCopy?: () => void
  onError?: (error: Error) => void
  timeout?: number
}

export const CodeBlockCopyButton = ({
  onCopy,
  onError,
  timeout = 2000,
  children,
  className,
  'aria-label': ariaLabel,
  ...props
}: CodeBlockCopyButtonProps) => {
  const { t } = useTranslation()
  const [isCopied, setIsCopied] = useState(false)
  const { code } = useContext(CodeBlockContext)

  const copyToClipboard = async () => {
    if (typeof window === 'undefined' || !navigator?.clipboard?.writeText) {
      onError?.(new Error('Clipboard API not available'))
      return
    }

    try {
      await navigator.clipboard.writeText(code)
      setIsCopied(true)
      onCopy?.()
      setTimeout(() => setIsCopied(false), timeout)
    } catch (error) {
      onError?.(error as Error)
    }
  }

  const Icon = isCopied ? CheckIcon : CopyIcon

  return (
    <Button
      aria-label={ariaLabel ?? (isCopied ? t('Copied!') : t('Copy code'))}
      className={cn('shrink-0', className)}
      onClick={copyToClipboard}
      size='icon-sm'
      type='button'
      variant='ghost'
      {...props}
    >
      {children ?? <Icon />}
    </Button>
  )
}
