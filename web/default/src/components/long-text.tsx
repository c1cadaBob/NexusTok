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
import { useCallback, useEffect, useRef, useState } from 'react'
import { cn } from '@/lib/utils'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'

type LongTextProps = {
  children: React.ReactNode
  className?: string
  contentClassName?: string
  side?: LongTextSide
}

export type LongTextSide =
  | 'bottom'
  | 'inline-end'
  | 'inline-start'
  | 'left'
  | 'right'
  | 'top'

export function LongText({
  children,
  className = '',
  contentClassName = '',
  side = 'top',
}: LongTextProps) {
  const desktopRef = useRef<HTMLDivElement>(null)
  const mobileRef = useRef<HTMLDivElement>(null)
  const [isOverflown, setIsOverflown] = useState(false)
  const [isMobile, setIsMobile] = useState(false)

  const updateOverflow = useCallback(() => {
    const current = isMobile ? mobileRef.current : desktopRef.current

    // 表格列宽、字体加载和响应式断点都会改变实际溢出状态，
    // 因此这里以 DOM 实测结果作为 tooltip/popover 是否启用的唯一依据。
    setIsOverflown(checkOverflow(current))
  }, [isMobile])

  useEffect(() => {
    if (typeof window === 'undefined' || !window.matchMedia) return

    const mediaQuery = window.matchMedia('(max-width: 639px)')
    const syncViewport = () => {
      setIsMobile(mediaQuery.matches)
    }

    syncViewport()
    mediaQuery.addEventListener('change', syncViewport)

    return () => {
      mediaQuery.removeEventListener('change', syncViewport)
    }
  }, [])

  useEffect(() => {
    const current = isMobile ? mobileRef.current : desktopRef.current
    updateOverflow()

    if (!current) return

    if (typeof ResizeObserver === 'undefined') {
      window.addEventListener('resize', updateOverflow)

      return () => {
        window.removeEventListener('resize', updateOverflow)
      }
    }

    const observer = new ResizeObserver(updateOverflow)
    observer.observe(current)

    return () => {
      observer.disconnect()
    }
  }, [children, className, isMobile, isOverflown, updateOverflow])

  if (!isOverflown) {
    return (
      <>
        <div className='hidden sm:block'>
          <div ref={desktopRef} className={cn('truncate', className)}>
            {children}
          </div>
        </div>
        <div className='sm:hidden'>
          <div ref={mobileRef} className={cn('truncate', className)}>
            {children}
          </div>
        </div>
      </>
    )
  }

  return (
    <>
      <div className='hidden sm:block'>
        <TooltipProvider delay={0}>
          <Tooltip>
            <TooltipTrigger
              render={
                <div ref={desktopRef} className={cn('truncate', className)} />
              }
            >
              {children}
            </TooltipTrigger>
            <TooltipContent side={side}>
              <p className={contentClassName}>{children}</p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>
      <div className='sm:hidden'>
        <Popover>
          <PopoverTrigger
            render={
              <div ref={mobileRef} className={cn('truncate', className)} />
            }
          >
            {children}
          </PopoverTrigger>
          <PopoverContent
            side={side}
            className={cn('w-fit max-w-xs', contentClassName)}
          >
            <p>{children}</p>
          </PopoverContent>
        </Popover>
      </div>
    </>
  )
}

const checkOverflow = (textContainer: HTMLDivElement | null) => {
  if (textContainer) {
    return (
      textContainer.offsetHeight < textContainer.scrollHeight ||
      textContainer.offsetWidth < textContainer.scrollWidth
    )
  }
  return false
}
