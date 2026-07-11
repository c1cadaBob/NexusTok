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
import type { ReactNode } from 'react'
import { useEffect, useRef, useState } from 'react'
import { SlidersHorizontal } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Drawer,
  DrawerContent,
  DrawerDescription,
  DrawerFooter,
  DrawerHeader,
  DrawerTitle,
  DrawerTrigger,
} from '@/components/ui/drawer'

type AccountPoolHistoryFilterKind = 'usage' | 'state' | 'check'

type AccountPoolHistoryFilterDrawerProps = {
  kind: AccountPoolHistoryFilterKind
  activeCount: number
  resetDisabled?: boolean
  onReset: () => void
  children: ReactNode
  className?: string
}

export function AccountPoolHistoryFilterDrawer({
  kind,
  activeCount,
  resetDisabled = false,
  onReset,
  children,
  className,
}: AccountPoolHistoryFilterDrawerProps) {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const bodyRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open) return

    const focusTimer = window.setTimeout(() => {
      const firstFocusable = bodyRef.current?.querySelector<
        HTMLInputElement | HTMLButtonElement | HTMLSelectElement
      >('input, select, button, [role="combobox"]')
      firstFocusable?.focus()
    }, 0)

    return () => window.clearTimeout(focusTimer)
  }, [open])

  const handleOpenChange = (nextOpen: boolean) => {
    // Vaul 打开 Drawer 时会隐藏页面其它区域；先释放触发按钮焦点，
    // 避免焦点仍停留在被 aria-hidden 的页面内容里。
    if (nextOpen && document.activeElement instanceof HTMLElement) {
      document.activeElement.blur()
    }
    setOpen(nextOpen)
  }

  return (
    <Drawer open={open} onOpenChange={handleOpenChange}>
      <DrawerTrigger asChild>
        <Button
          type='button'
          variant='ghost'
          data-account-pool-history-filter-trigger={kind}
          className={cn(
            'text-muted-foreground hover:text-foreground gap-1 px-2',
            activeCount > 0 && 'text-primary hover:text-primary',
            className
          )}
        >
          <SlidersHorizontal data-icon='inline-start' />
          {t('Filter')}
          {activeCount > 0 && (
            <Badge className='ms-0.5 size-5 justify-center p-0 text-[10px]'>
              {activeCount}
            </Badge>
          )}
        </Button>
      </DrawerTrigger>
      <DrawerContent className='h-[85dvh] max-h-[85dvh] p-0'>
        <div className='mx-auto flex h-full w-full max-w-md flex-col overflow-hidden'>
          <DrawerHeader className='border-b px-4 py-3 text-left'>
            <DrawerTitle>{t('Filter')}</DrawerTitle>
            <DrawerDescription>
              {t('Adjust filters, then search to refresh the logs.')}
            </DrawerDescription>
          </DrawerHeader>
          <div
            ref={bodyRef}
            className='flex min-h-0 flex-1 flex-col gap-2 overflow-y-auto px-4 py-3'
          >
            {children}
          </div>
          <DrawerFooter className='grid grid-cols-2 gap-2 border-t px-4 py-3'>
            <Button
              type='button'
              variant='outline'
              onClick={onReset}
              disabled={resetDisabled}
            >
              {t('Reset')}
            </Button>
            <Button type='button' onClick={() => setOpen(false)}>
              {t('Done')}
            </Button>
          </DrawerFooter>
        </div>
      </DrawerContent>
    </Drawer>
  )
}
