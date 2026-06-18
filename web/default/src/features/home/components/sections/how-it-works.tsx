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
import { Cable, Gauge, KeyRound, ReceiptText } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { AnimateInView } from '@/components/animate-in-view'

const STEPS = [
  {
    icon: Cable,
    title: 'Connect providers',
    description: 'Add upstream channels and assign provider credentials.',
  },
  {
    icon: KeyRound,
    title: 'Issue access keys',
    description:
      'Create user keys, groups, rate limits, and model access rules.',
  },
  {
    icon: Gauge,
    title: 'Route requests',
    description: 'Serve compatible API calls through the gateway endpoint.',
  },
  {
    icon: ReceiptText,
    title: 'Settle usage',
    description: 'Record tokens, cost, quota consumption, and request logs.',
  },
] as const

export function HowItWorks() {
  const { t } = useTranslation()

  return (
    <section className='bg-muted/20 border-y px-6 py-16 md:py-20'>
      <div className='mx-auto max-w-7xl'>
        <div className='mb-8 flex flex-col justify-between gap-4 md:flex-row md:items-end'>
          <AnimateInView>
            <Badge variant='outline'>{t('Request path')}</Badge>
            <h2 className='mt-4 text-2xl font-semibold tracking-tight md:text-3xl'>
              {t('From key to settlement')}
            </h2>
          </AnimateInView>
          <AnimateInView delay={80} animation='fade-left'>
            <p className='text-muted-foreground max-w-xl text-sm leading-relaxed'>
              {t(
                'Every request moves through the same observable path, so teams can change providers without changing application code.'
              )}
            </p>
          </AnimateInView>
        </div>

        <div className='bg-border grid gap-px overflow-hidden rounded-lg border md:grid-cols-4'>
          {STEPS.map((step, index) => {
            const Icon = step.icon

            return (
              <AnimateInView
                key={step.title}
                delay={index * 80}
                className='bg-background'
              >
                <div className='flex h-full flex-col gap-8 p-5 md:p-6'>
                  <div className='flex items-center justify-between gap-3'>
                    <span className='text-muted-foreground font-mono text-xs tabular-nums'>
                      0{index + 1}
                    </span>
                    <Icon
                      className='text-muted-foreground size-4'
                      aria-hidden
                    />
                  </div>
                  <div className='flex flex-col gap-2'>
                    <h3 className='text-base font-medium'>{t(step.title)}</h3>
                    <p className='text-muted-foreground text-sm leading-relaxed'>
                      {t(step.description)}
                    </p>
                  </div>
                </div>
              </AnimateInView>
            )
          })}
        </div>
      </div>
    </section>
  )
}
