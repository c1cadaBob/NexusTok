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
import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { Menu } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { type TopNavLink } from '../types'

type TopNavProps = React.HTMLAttributes<HTMLElement> & {
  links: TopNavLink[]
}

/**
 * 顶部导航栏组件
 * 在大屏幕显示水平导航，在小屏幕显示下拉菜单
 */
export function TopNav({ className, links, ...props }: TopNavProps) {
  // 规范化链接，确保所有可选属性都有默认值
  const normalizedLinks = useMemo(
    () =>
      links.map((link) => ({
        isActive: false,
        disabled: false,
        external: false,
        ...link,
      })),
    [links]
  )

  return (
    <>
      {/* 移动端下拉菜单 */}
      <div className='lg:hidden'>
        <DropdownMenu modal={false}>
          <DropdownMenuTrigger
            render={<Button size='icon' variant='outline' className='size-7' />}
          >
            <Menu />
          </DropdownMenuTrigger>
          <DropdownMenuContent side='bottom' align='start'>
            {normalizedLinks.map(
              ({ title, href, isActive, disabled, external }) => {
                const linkClassName = cn(
                  !isActive && 'text-muted-foreground',
                  disabled && 'pointer-events-none opacity-50'
                )

                return (
                  <DropdownMenuItem
                    key={`${title}-${href}`}
                    disabled={disabled}
                    render={
                      external ? (
                        <a
                          href={disabled ? undefined : href}
                          target={disabled ? undefined : '_blank'}
                          rel={disabled ? undefined : 'noopener noreferrer'}
                          aria-disabled={disabled || undefined}
                          tabIndex={disabled ? -1 : undefined}
                          className={linkClassName}
                        >
                          {title}
                        </a>
                      ) : (
                        <Link
                          to={href}
                          className={linkClassName}
                          disabled={disabled}
                          aria-disabled={disabled || undefined}
                        >
                          {title}
                        </Link>
                      )
                    }
                  ></DropdownMenuItem>
                )
              }
            )}
          </DropdownMenuContent>
        </DropdownMenu>
      </div>

      {/* 桌面端水平导航 */}
      <nav
        className={cn(
          'hidden items-center space-x-4 lg:flex lg:space-x-4 xl:space-x-6',
          className
        )}
        {...props}
      >
        {normalizedLinks.map(({ title, href, isActive, disabled, external }) =>
          external ? (
            <a
              key={`${title}-${href}`}
              href={disabled ? undefined : href}
              target={disabled ? undefined : '_blank'}
              rel={disabled ? undefined : 'noopener noreferrer'}
              aria-disabled={disabled || undefined}
              tabIndex={disabled ? -1 : undefined}
              className={cn(
                'hover:text-primary text-sm font-medium transition-colors',
                !isActive && 'text-muted-foreground',
                disabled && 'pointer-events-none opacity-50'
              )}
            >
              {title}
            </a>
          ) : (
            <Link
              key={`${title}-${href}`}
              to={href}
              disabled={disabled}
              aria-disabled={disabled || undefined}
              className={cn(
                'hover:text-primary text-sm font-medium transition-colors',
                !isActive && 'text-muted-foreground',
                disabled && 'pointer-events-none opacity-50'
              )}
            >
              {title}
            </Link>
          )
        )}
      </nav>
    </>
  )
}
