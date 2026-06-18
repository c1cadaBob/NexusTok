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
import { type ComponentPropsWithoutRef } from 'react'
import { cn } from '@/lib/utils'
import { DEFAULT_LOGO } from '@/lib/constants'

export function Logo({
  className,
  alt = 'NexusTok',
  ...props
}: ComponentPropsWithoutRef<'img'>) {
  return (
    <img
      id='nexustok-logo'
      src={DEFAULT_LOGO}
      alt={alt}
      className={cn('size-6', className)}
      {...props}
    />
  )
}
