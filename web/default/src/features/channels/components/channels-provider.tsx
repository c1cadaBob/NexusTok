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
import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getChannel } from '../api'
import { useChannelUpstreamUpdates } from '../hooks/use-channel-upstream-updates'
import { channelsQueryKeys } from '../lib'
import type { Channel, UpstreamAccountPlatform } from '../types'

// ============================================================================
// 类型定义
// ============================================================================

type DialogType =
  | 'create-channel'
  | 'update-channel'
  | 'test-channel'
  | 'balance-query'
  | 'fetch-models'
  | 'ollama-models'
  | 'multi-key-manage'
  | 'account-pool-manage'
  | 'tag-batch-edit'
  | 'edit-tag'
  | 'copy-channel'
  | null

type UpstreamUpdateState = ReturnType<typeof useChannelUpstreamUpdates>

export type UpstreamCaptureReturnContext = {
  captureId: string
  mode: 'create' | 'refresh'
  platform?: UpstreamAccountPlatform
  baseUrl?: string
  channelId?: number
}

type ChannelsContextType = {
  open: DialogType
  setOpen: (open: DialogType) => void
  currentRow: Channel | null
  setCurrentRow: (row: Channel | null) => void
  upstreamCaptureReturnContext: UpstreamCaptureReturnContext | null
  currentTag: string | null
  setCurrentTag: (tag: string | null) => void
  enableTagMode: boolean
  setEnableTagMode: (enabled: boolean) => void
  idSort: boolean
  setIdSort: (enabled: boolean) => void
  batchMode: boolean
  setBatchMode: (enabled: boolean) => void
  sensitiveVisible: boolean
  setSensitiveVisible: (visible: boolean) => void
  expandedAccountPoolChannelIds: Set<number>
  toggleAccountPoolExpanded: (channelId: number) => void
  isAccountPoolExpanded: (channelId: number) => boolean
  upstream: UpstreamUpdateState
}

// ============================================================================
// 上下文
// ============================================================================

const ChannelsContext = createContext<ChannelsContextType | undefined>(
  undefined
)

function parseUpstreamCaptureReturnContext(): UpstreamCaptureReturnContext | null {
  if (typeof window === 'undefined') return null

  const searchParams = new URLSearchParams(window.location.search)
  const captureId = searchParams.get('upstream_capture_id')?.trim() || ''
  if (!captureId) return null

  const modeParam = searchParams.get('upstream_capture_mode')
  const mode = modeParam === 'refresh' ? 'refresh' : 'create'
  const platformParam = searchParams
    .get('upstream_capture_platform')
    ?.trim()
    .toLowerCase()
  const platform =
    platformParam === 'new-api' || platformParam === 'sub2api'
      ? platformParam
      : undefined
  const channelIdValue = Number(
    searchParams.get('upstream_capture_channel_id') || ''
  )
  const channelId =
    Number.isInteger(channelIdValue) && channelIdValue > 0
      ? channelIdValue
      : undefined

  if (mode === 'refresh' && !channelId) return null

  return {
    captureId,
    mode,
    platform,
    baseUrl: searchParams.get('upstream_capture_base_url')?.trim() || undefined,
    channelId,
  }
}

function clearUpstreamCaptureReturnParams() {
  if (typeof window === 'undefined') return
  const url = new URL(window.location.href)
  for (const key of [
    'upstream_capture_id',
    'upstream_capture_mode',
    'upstream_capture_platform',
    'upstream_capture_base_url',
    'upstream_capture_channel_id',
  ]) {
    url.searchParams.delete(key)
  }
  window.history.replaceState(
    window.history.state,
    '',
    `${url.pathname}${url.search}${url.hash}`
  )
}

// ============================================================================
// Provider
// ============================================================================

export function ChannelsProvider({ children }: { children: React.ReactNode }) {
  const [open, setOpen] = useState<DialogType>(null)
  const [currentRow, setCurrentRow] = useState<Channel | null>(null)
  const [upstreamCaptureReturnContext] =
    useState<UpstreamCaptureReturnContext | null>(
      parseUpstreamCaptureReturnContext
    )
  const captureReturnAppliedRef = useRef('')
  const [currentTag, setCurrentTag] = useState<string | null>(null)
  const [enableTagMode, setEnableTagMode] = useState(() => {
    return localStorage.getItem('enable-tag-mode') === 'true'
  })
  const [idSort, setIdSort] = useState(() => {
    return localStorage.getItem('channels-id-sort') === 'true'
  })
  const [batchMode, setBatchMode] = useState(false)
  const [sensitiveVisible, setSensitiveVisible] = useState(true)
  const [expandedAccountPoolChannelIds, setExpandedAccountPoolChannelIds] =
    useState<Set<number>>(() => new Set())

  const queryClient = useQueryClient()
  const captureChannelQuery = useQuery({
    queryKey: channelsQueryKeys.detail(
      upstreamCaptureReturnContext?.channelId || 0
    ),
    queryFn: () => getChannel(upstreamCaptureReturnContext!.channelId!),
    enabled:
      upstreamCaptureReturnContext?.mode === 'refresh' &&
      Boolean(upstreamCaptureReturnContext.channelId),
  })

  useEffect(() => {
    const context = upstreamCaptureReturnContext
    if (!context) return

    const contextKey = [
      context.mode,
      context.captureId,
      context.channelId || '',
    ].join(':')
    if (captureReturnAppliedRef.current === contextKey) return

    if (context.mode === 'refresh') {
      const channel = captureChannelQuery.data?.data
      if (!channel) return
      setCurrentRow(channel)
      setOpen('update-channel')
    } else {
      setCurrentRow(null)
      setOpen('create-channel')
    }

    captureReturnAppliedRef.current = contextKey
    clearUpstreamCaptureReturnParams()
  }, [captureChannelQuery.data?.data, upstreamCaptureReturnContext])

  const refreshChannels = useCallback(async () => {
    await queryClient.invalidateQueries({ queryKey: channelsQueryKeys.all })
  }, [queryClient])
  const upstream = useChannelUpstreamUpdates(refreshChannels)
  const toggleAccountPoolExpanded = useCallback((channelId: number) => {
    setExpandedAccountPoolChannelIds((previous) => {
      const next = new Set(previous)
      if (next.has(channelId)) {
        next.delete(channelId)
      } else {
        next.add(channelId)
      }
      return next
    })
  }, [])
  const isAccountPoolExpanded = useCallback(
    (channelId: number) => expandedAccountPoolChannelIds.has(channelId),
    [expandedAccountPoolChannelIds]
  )

  // 批量操作模式和敏感显隐都是页面级临时操作状态，不写入 localStorage，避免跨会话造成误判。
  // 账号池展开状态仅用于当前列表页快速查看，保持临时状态可避免跨页缓存过期账号摘要。
  // context value 使用 memo，减少无关状态更新扩散到表格单元格和移动卡片。
  const value = useMemo<ChannelsContextType>(
    () => ({
      open,
      setOpen,
      currentRow,
      setCurrentRow,
      upstreamCaptureReturnContext,
      currentTag,
      setCurrentTag,
      enableTagMode,
      setEnableTagMode,
      idSort,
      setIdSort,
      batchMode,
      setBatchMode,
      sensitiveVisible,
      setSensitiveVisible,
      expandedAccountPoolChannelIds,
      toggleAccountPoolExpanded,
      isAccountPoolExpanded,
      upstream,
    }),
    [
      open,
      currentRow,
      upstreamCaptureReturnContext,
      currentTag,
      enableTagMode,
      idSort,
      batchMode,
      sensitiveVisible,
      expandedAccountPoolChannelIds,
      toggleAccountPoolExpanded,
      isAccountPoolExpanded,
      upstream,
    ]
  )

  return (
    <ChannelsContext.Provider value={value}>
      {children}
    </ChannelsContext.Provider>
  )
}

// ============================================================================
// Hook
// ============================================================================

export function useChannels() {
  const context = useContext(ChannelsContext)
  if (!context) {
    throw new Error('useChannels must be used within ChannelsProvider')
  }
  return context
}
