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
import {
  CameraIcon,
  FileIcon,
  ImageIcon,
  ScreenShareIcon,
  type LucideIcon,
} from 'lucide-react'

interface AttachmentAction {
  action: string
  icon: LucideIcon
  label: string
}

interface InputToolNotice {
  description?: string
  title: string
}

export const ATTACHMENT_ACTIONS = [
  { action: 'upload-file', icon: FileIcon, label: 'Upload file' },
  { action: 'upload-photo', icon: ImageIcon, label: 'Upload photo' },
  {
    action: 'take-screenshot',
    icon: ScreenShareIcon,
    label: 'Take screenshot',
  },
  { action: 'take-photo', icon: CameraIcon, label: 'Take photo' },
] satisfies AttachmentAction[]

/**
 * 返回附件工具的占位提示。
 *
 * 当前 Playground 还没有真实上传链路，保留 action 作为描述方便调试；
 * 真实能力接入时只需要替换这里的 notice 或 handler。
 */
export function getAttachmentActionNotice(action: string): InputToolNotice {
  return {
    description: action,
    title: 'Feature in development',
  }
}

/**
 * 返回搜索工具的占位提示，避免组件里散落 toast 文案。
 */
export function getSearchActionNotice(): InputToolNotice {
  return {
    title: 'Search feature in development',
  }
}
