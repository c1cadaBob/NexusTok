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
import { useState, useCallback } from 'react'
import { DEFAULT_CONFIG, DEFAULT_PARAMETER_ENABLED } from '../constants'
import {
  saveConfig,
  saveParameterEnabled,
  saveMessages,
  getInitialMessages,
  getInitialParameterEnabled,
  getInitialPlaygroundConfig,
  applyMessageStateUpdate,
  type MessageStateUpdater,
} from '../lib'
import type {
  Message,
  PlaygroundConfig,
  ParameterEnabled,
  ModelOption,
  GroupOption,
} from '../types'

/**
 * Playground 主状态管理 hook。
 */
export function usePlaygroundState() {
  // 从 localStorage 加载初始状态，异常数据会在 storage 层兜底。
  const [config, setConfig] = useState<PlaygroundConfig>(
    getInitialPlaygroundConfig
  )

  const [parameterEnabled, setParameterEnabled] = useState<ParameterEnabled>(
    getInitialParameterEnabled
  )

  const [messages, setMessages] = useState<Message[]>(getInitialMessages)

  const [models, setModels] = useState<ModelOption[]>([])
  const [groups, setGroups] = useState<GroupOption[]>([])

  // 更新配置并自动保存。
  const updateConfig = useCallback(
    <K extends keyof PlaygroundConfig>(key: K, value: PlaygroundConfig[K]) => {
      setConfig((prev) => {
        const updated = { ...prev, [key]: value }
        saveConfig(updated)
        return updated
      })
    },
    []
  )

  // 更新参数开关并自动保存。
  const updateParameterEnabled = useCallback(
    (key: keyof ParameterEnabled, value: boolean) => {
      setParameterEnabled((prev) => {
        const updated = { ...prev, [key]: value }
        saveParameterEnabled(updated)
        return updated
      })
    },
    []
  )

  // 更新消息并自动保存。
  const updateMessages = useCallback((updater: MessageStateUpdater) => {
    setMessages((prev) => {
      const newMessages = applyMessageStateUpdate(prev, updater)
      saveMessages(newMessages)
      return newMessages
    })
  }, [])

  // 清空当前会话消息。
  const clearMessages = useCallback(() => {
    updateMessages([])
  }, [updateMessages])

  // 重置配置与参数开关到默认值。
  const resetConfig = useCallback(() => {
    setConfig(DEFAULT_CONFIG)
    setParameterEnabled(DEFAULT_PARAMETER_ENABLED)
    saveConfig(DEFAULT_CONFIG)
    saveParameterEnabled(DEFAULT_PARAMETER_ENABLED)
  }, [])

  return {
    // 状态
    config,
    parameterEnabled,
    messages,
    models,
    groups,

    // 列表 setter
    setModels,
    setGroups,

    // 操作
    updateConfig,
    updateParameterEnabled,
    updateMessages,
    clearMessages,
    resetConfig,
  }
}
