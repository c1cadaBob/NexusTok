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
import { create } from 'zustand'
import { persist } from 'zustand/middleware'
import { DEFAULT_SYSTEM_NAME, DEFAULT_LOGO } from '@/lib/constants'

export type CurrencyDisplayType = 'USD' | 'CNY' | 'TOKENS' | 'CUSTOM'

export interface CurrencyConfig {
  /** 是否把额度值渲染为货币，而不是原始单位。 */
  displayInCurrency: boolean
  /** 管理员配置的额度展示策略。 */
  quotaDisplayType: CurrencyDisplayType
  /** 多少额度单位等于 1 USD。 */
  quotaPerUnit: number
  /** USD 到系统内置本地货币的汇率。 */
  usdExchangeRate: number
  /** 管理员配置的自定义货币符号，仅在 type === CUSTOM 时使用。 */
  customCurrencySymbol: string
  /** USD 到自定义货币的汇率，仅在 type === CUSTOM 时使用。 */
  customCurrencyExchangeRate: number
}

export interface SystemConfig {
  systemName: string
  logo: string
  footerHtml?: string
  demoSiteEnabled?: boolean
  displayTokenStatEnabled?: boolean
  currency: CurrencyConfig
}

export const DEFAULT_CURRENCY_CONFIG: CurrencyConfig = {
  displayInCurrency: true,
  quotaDisplayType: 'USD',
  quotaPerUnit: 500000,
  usdExchangeRate: 1,
  customCurrencySymbol: '¤',
  customCurrencyExchangeRate: 1,
}

interface SystemConfigState {
  config: SystemConfig
  loading: boolean
  loadedLogoUrl: string
  setConfig: (config: Partial<SystemConfig>) => void
  setLoadedLogoUrl: (url: string) => void
  setLoading: (loading: boolean) => void
}

/**
 * 系统配置状态存储。
 *
 * 仅持久化展示配置和加载状态；Logo 始终使用 DEFAULT_LOGO，避免旧 localStorage
 * 或后端历史配置在用户刷新后重新覆盖当前随包发布的品牌图。
 */
export const useSystemConfigStore = create<SystemConfigState>()(
  persist(
    (set) => ({
      config: {
        systemName: DEFAULT_SYSTEM_NAME,
        logo: DEFAULT_LOGO,
        currency: { ...DEFAULT_CURRENCY_CONFIG },
      },
      loading: true,
      loadedLogoUrl: DEFAULT_LOGO,
      setConfig: (newConfig) =>
        set((state) => ({
          config: {
            ...state.config,
            ...newConfig,
            logo: DEFAULT_LOGO,
            currency: {
              ...state.config.currency,
              ...(newConfig.currency ?? {}),
            },
          },
        })),
      setLoadedLogoUrl: (url) => set({ loadedLogoUrl: url }),
      setLoading: (loading) => set({ loading }),
    }),
    {
      name: 'system-config-storage',
      merge: (persistedState, currentState) => {
        const persisted = persistedState as Partial<SystemConfigState>

        return {
          ...currentState,
          config: {
            ...currentState.config,
            ...(persisted.config ?? {}),
            logo: DEFAULT_LOGO,
            currency: {
              ...currentState.config.currency,
              ...(persisted.config?.currency ?? {}),
            },
          },
          // 旧版本可能已经把带旧版本号的 Logo 写入 localStorage。
          // 持久化合并时强制归一，确保首屏和 favicon 都使用当前随包发布的品牌图。
          loadedLogoUrl: DEFAULT_LOGO,
        }
      },
      partialize: (state) => ({
        config: {
          ...state.config,
          logo: DEFAULT_LOGO,
        },
        loadedLogoUrl: DEFAULT_LOGO,
      }),
    }
  )
)

// Selector helpers for convenience
export const getSystemName = () =>
  useSystemConfigStore.getState().config.systemName

export const getLogo = () => useSystemConfigStore.getState().config.logo

export const getFooterHtml = () =>
  useSystemConfigStore.getState().config.footerHtml
