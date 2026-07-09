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
import { useEffect, useState } from 'react'
import i18next from 'i18next'
import { toast } from 'sonner'
import { isHttpUrl } from '@/lib/content-format'
import { getHomePageContent } from '../api'
import type { HomePageContentResult } from '../types'

const STORAGE_KEY = 'home_page_content'

/**
 * 加载并管理自定义主页内容。
 *
 * 后端配置可以是 Markdown、HTML 或完整的 HTTP(S) URL。这里优先读取本地缓存以减少首屏等待，
 * 但最终始终以接口返回值为准，避免用户看到已经被管理员清空的旧内容。
 */
export function useHomePageContent(): HomePageContentResult {
  const [content, setContent] = useState<string>('')
  const [isLoaded, setIsLoaded] = useState(false)

  useEffect(() => {
    let mounted = true

    const loadContent = async () => {
      // 先读取本地缓存，让自定义主页在接口返回前仍能立即展示。
      const cached = localStorage.getItem(STORAGE_KEY)
      if (cached && mounted) {
        setContent(cached)
      }

      try {
        const response = await getHomePageContent()
        const { success, data } = response

        if (!mounted) return

        if (success && data) {
          setContent(data)
          localStorage.setItem(STORAGE_KEY, data)
        } else {
          // 接口明确返回空内容时同步清理缓存，保证禁用配置能及时生效。
          setContent('')
          localStorage.removeItem(STORAGE_KEY)
        }
      } catch (error) {
        if (!mounted) return
        // eslint-disable-next-line no-console
        console.error('Failed to load home page content:', error)
        toast.error(i18next.t('Failed to load home page content'))
      } finally {
        if (mounted) {
          setIsLoaded(true)
        }
      }
    }

    loadContent()

    return () => {
      mounted = false
    }
  }, [])

  const isUrl = isHttpUrl(content)

  return { content, isLoaded, isUrl }
}
