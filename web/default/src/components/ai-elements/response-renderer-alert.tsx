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
import { t } from 'i18next'
import type { ReactNode } from 'react'
import type { BlockquoteNode, ParsedNode } from 'stream-markdown-parser'

import {
  Alert,
  AlertDescription,
  AlertTitle,
} from '@/components/ui/alert'
import { cn } from '@/lib/utils'

import { hasParsedChildren } from './response-node-guards'
import type {
  AlertConfig,
  AlertKind,
  BlockRendererOptions,
} from './response-types'

const alertConfig = {
  note: {
    label: 'Note',
    className: 'border-info/40 bg-info/10 text-foreground',
    markerClassName: 'text-info',
  },
  tip: {
    label: 'Tip',
    className: 'border-success/40 bg-success/10 text-foreground',
    markerClassName: 'text-success',
  },
  important: {
    label: 'Important',
    className: 'border-primary/40 bg-primary/10 text-foreground',
    markerClassName: 'text-primary',
  },
  warning: {
    label: 'Warning',
    className: 'border-warning/40 bg-warning/10 text-foreground',
    markerClassName: 'text-warning',
  },
  caution: {
    label: 'Caution',
    className: 'border-destructive/40 bg-destructive/10 text-foreground',
    markerClassName: 'text-destructive',
  },
} satisfies Record<AlertKind, AlertConfig>

function getAlertKind(node: BlockquoteNode): AlertKind | null {
  const firstChild = node.children[0]
  if (!firstChild || firstChild.type !== 'paragraph') {
    return null
  }

  if (!hasParsedChildren(firstChild)) {
    return null
  }

  const firstInline = firstChild.children[0]
  if (!firstInline || firstInline.type !== 'text') {
    return null
  }

  if (!('content' in firstInline) || typeof firstInline.content !== 'string') {
    return null
  }

  const markerPattern = /^\[!(NOTE|TIP|IMPORTANT|WARNING|CAUTION)\]\s*\n?/i
  const match = firstInline.content.match(markerPattern)
  if (!match) {
    return null
  }

  return match[1].toLowerCase() as AlertKind
}

function getAlertChildren(node: BlockquoteNode, kind: AlertKind): ParsedNode[] {
  const firstChild = node.children[0]
  if (!firstChild || firstChild.type !== 'paragraph') {
    return node.children
  }

  if (!hasParsedChildren(firstChild)) {
    return node.children
  }

  const firstInline = firstChild.children[0]
  if (!firstInline || firstInline.type !== 'text') {
    return node.children
  }

  if (!('content' in firstInline) || typeof firstInline.content !== 'string') {
    return node.children
  }

  const marker = `[!${kind.toUpperCase()}]`
  const content = firstInline.content.replace(marker, '').replace(/^\s*\n?/, '')
  const nextParagraph = {
    ...firstChild,
    children: [
      { ...firstInline, content, raw: content },
      ...firstChild.children.slice(1),
    ],
  }

  if (!content && nextParagraph.children.length === 1) {
    return node.children.slice(1)
  }

  return [nextParagraph, ...node.children.slice(1)]
}

export function renderBlockquote(
  node: BlockquoteNode,
  key: string,
  options: BlockRendererOptions
): ReactNode {
  const alertKind = getAlertKind(node)
  if (alertKind) {
    const config = alertConfig[alertKind]
    const alertChildren = getAlertChildren(node, alertKind)

    return (
      <Alert
        className={cn(
          'my-4 rounded-lg border px-4 py-3 text-sm',
          '[&>*:first-child]:mt-0 [&>*:last-child]:mb-0',
          config.className
        )}
        key={key}
        role='note'
      >
        <AlertTitle
          className={cn(
            'mb-2 text-xs font-semibold tracking-wide uppercase',
            config.markerClassName
          )}
        >
          {t(config.label)}
        </AlertTitle>
        <AlertDescription className='text-foreground [&>*:first-child]:mt-0 [&>*:last-child]:mb-0'>
          {options.renderChildren(alertChildren)}
        </AlertDescription>
      </Alert>
    )
  }

  return (
    <blockquote
      className='border-border text-muted-foreground my-4 border-l-2 pl-4'
      key={key}
    >
      {options.renderChildren(node.children)}
    </blockquote>
  )
}
