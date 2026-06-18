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
import { Link } from '@tanstack/react-router'
import { ArrowRight, BookOpen, LayoutDashboard } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { AnimateInView } from '@/components/animate-in-view'

interface CTAProps {
  className?: string
  isAuthenticated?: boolean
}

export function CTA(props: CTAProps) {
  const { t } = useTranslation()

  return (
    <section className='px-6 py-14 md:py-16'>
      <AnimateInView className='mx-auto max-w-7xl'>
        <div className='bg-card grid gap-6 rounded-lg border p-5 shadow-xs md:grid-cols-[1fr_auto] md:items-center md:p-6'>
          <div className='flex flex-col gap-2'>
            <h2 className='text-xl font-semibold tracking-tight md:text-2xl'>
              {props.isAuthenticated
                ? t('Continue operating your gateway')
                : t('Start with the control plane')}
            </h2>
            <p className='text-muted-foreground max-w-2xl text-sm leading-relaxed'>
              {props.isAuthenticated
                ? t(
                    'Open the dashboard to review channels, usage, users, and billing settings.'
                  )
                : t(
                    'Create an account, add a channel, issue a key, and route your first compatible API request.'
                  )}
            </p>
          </div>
          <div className='flex flex-col gap-2 sm:flex-row md:justify-end'>
            {props.isAuthenticated ? (
              <Button render={<Link to='/dashboard' />}>
                <LayoutDashboard data-icon='inline-start' />
                {t('Go to Dashboard')}
              </Button>
            ) : (
              <>
                <Button render={<Link to='/sign-up' />}>
                  {t('Get Started')}
                  <ArrowRight data-icon='inline-end' />
                </Button>
                <Button variant='outline' render={<Link to='/pricing' />}>
                  <BookOpen data-icon='inline-start' />
                  {t('View Pricing')}
                </Button>
              </>
            )}
          </div>
        </div>
      </AnimateInView>
    </section>
  )
}
