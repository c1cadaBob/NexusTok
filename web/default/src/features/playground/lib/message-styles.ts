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
/**
 * 获取消息正文样式。
 *
 * 这里集中维护 user 气泡和 assistant 回复的阅读宽度、字体与 surface，
 * 避免展示组件里散落角色相关样式。assistant 采用文档列宽，减少宽屏长
 * 回答横向铺满；user 保持紧凑气泡，便于和右侧对齐布局配合。
 */
export function getMessageContentStyles() {
  return [
    // assistant 内容按文档列阅读，user 气泡保持按内容收缩。
    'group-[.is-assistant]:w-full',
    'group-[.is-assistant]:max-w-[78ch]',
    'group-[.is-user]:w-fit',

    // user 气泡使用语义 muted surface，避免手写深色模式覆盖。
    'group-[.is-user]:rounded-2xl',
    'group-[.is-user]:rounded-br-md',
    'group-[.is-user]:border',
    'group-[.is-user]:border-border/70',
    'group-[.is-user]:bg-muted/70',
    'group-[.is-user]:px-4',
    'group-[.is-user]:py-2.5',
    'group-[.is-user]:text-foreground',
    'group-[.is-user]:shadow-sm',

    // assistant 回复保持平面阅读面，并跟随当前默认前端字体轴。
    'group-[.is-assistant]:bg-transparent',
    'group-[.is-assistant]:p-0',
    'group-[.is-assistant]:rounded-none',
    'group-[.is-assistant]:overflow-visible',
    'group-[.is-assistant]:font-sans',
    'group-[.is-assistant]:text-foreground/90',

    // 正文默认字号和行高保持稳定，移动与桌面都优先可读。
    'text-[0.95rem]',
    'leading-6',
    'break-words',
    'whitespace-pre-wrap',
    'sm:text-[0.975rem]',
    'sm:leading-7',

    // 限制 user 气泡宽度，避免长 prompt 在宽屏上变成横幅。
    'group-[.is-user]:max-w-[85%]',
    'sm:group-[.is-user]:max-w-[62ch]',
    'md:group-[.is-user]:max-w-[68ch]',
    'lg:group-[.is-user]:max-w-[72ch]',
  ].join(' ')
}
