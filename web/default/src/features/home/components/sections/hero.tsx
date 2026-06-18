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
import {
  ArrowRight,
  CheckCircle2,
  Layers3,
  ShieldCheck,
  Sparkles,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { HeroTerminalDemo } from '../hero-terminal-demo'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

const HIGHLIGHTS = [
  {
    key: 'One gateway for every model',
    icon: Layers3,
  },
  {
    key: 'One key for every provider',
    icon: ShieldCheck,
  },
  {
    key: 'Usage-aware billing',
    icon: Sparkles,
  },
] as const

const TRUST_POINTS = [
  'OpenAI',
  'Claude',
  'Gemini',
  'Azure',
  'Bedrock',
  'OpenRouter',
] as const

export function Hero(props: HeroProps) {
  const { t } = useTranslation()

  return (
    <section className='relative overflow-hidden px-6 pt-20 pb-12 md:px-8 md:pt-24 md:pb-16 lg:pb-20'>
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 -z-10 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:radial-gradient(ellipse_72%_58%_at_50%_18%,black_18%,transparent_80%)] bg-[size:5rem_5rem] opacity-[0.04]'
      />
      <div
        aria-hidden
        className='pointer-events-none absolute inset-x-0 top-0 -z-10 h-[34rem] bg-[radial-gradient(ellipse_75%_45%_at_22%_12%,color-mix(in_oklch,var(--primary)_16%,transparent)_0%,transparent_60%),radial-gradient(ellipse_55%_35%_at_82%_8%,color-mix(in_oklch,var(--secondary)_22%,transparent)_0%,transparent_62%),linear-gradient(to_bottom,color-mix(in_oklch,var(--muted)_48%,transparent)_0%,transparent_100%)] dark:opacity-80'
      />

      <div className='mx-auto grid w-full max-w-7xl min-w-0 items-center gap-10 lg:grid-cols-[minmax(0,1.02fr)_minmax(0,0.98fr)] lg:gap-12'>
        <div className='w-full min-w-0 max-w-2xl'>
          <Badge
            variant='secondary'
            className='mb-5 gap-1.5 px-2.5 py-0.5 text-[11px] tracking-[0.18em] uppercase'
          >
            <CheckCircle2 data-icon='inline-start' />
            {t('Live gateway')}
          </Badge>

          <h1
            className='landing-animate-fade-up max-w-xl text-4xl leading-[1.04] font-bold tracking-tight text-balance opacity-0 md:text-5xl lg:text-[3.85rem]'
            style={{ animationDelay: '0ms' }}
          >
            {t('Unified API Gateway for')}
            <br />
            <span className='from-primary bg-gradient-to-r via-indigo-500 to-violet-500 bg-clip-text text-transparent'>
              {t('All Your AI Models')}
            </span>
          </h1>

          <p
            className='landing-animate-fade-up text-muted-foreground/85 mt-6 max-w-xl text-base leading-7 text-balance opacity-0 md:text-lg'
            style={{ animationDelay: '80ms' }}
          >
            {t(
              'Power AI applications, manage digital assets, connect the Future'
            )}
          </p>

          <div
            className='landing-animate-fade-up mt-8 flex flex-wrap items-center gap-3 opacity-0'
            style={{ animationDelay: '140ms' }}
          >
            {props.isAuthenticated ? (
              <Button
                className='group rounded-full px-5'
                render={<Link to='/dashboard' />}
              >
                {t('Go to Dashboard')}
                <ArrowRight
                  data-icon='inline-end'
                  className='transition-transform duration-200 group-hover:translate-x-0.5'
                />
              </Button>
            ) : (
              <>
                <Button
                  className='group rounded-full px-5'
                  render={<Link to='/sign-up' />}
                >
                  {t('Get Started')}
                  <ArrowRight
                    data-icon='inline-end'
                    className='transition-transform duration-200 group-hover:translate-x-0.5'
                  />
                </Button>
                <Button
                  variant='outline'
                  className='rounded-full border-border/60 px-5 hover:border-border hover:bg-muted/60'
                  render={<Link to='/pricing' />}
                >
                  {t('View Pricing')}
                </Button>
              </>
            )}
          </div>

          <div className='mt-8 grid gap-3 sm:grid-cols-3'>
            {HIGHLIGHTS.map((item, index) => {
              const Icon = item.icon
              return (
                <div
                  key={item.key}
                  className='landing-animate-fade-up border-border/60 bg-background/80 flex items-center gap-3 rounded-2xl border px-4 py-3 opacity-0 shadow-[0_8px_28px_-20px_rgba(15,23,42,0.3)] backdrop-blur-sm'
                  style={{ animationDelay: `${200 + index * 60}ms` }}
                >
                  <span className='bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-xl'>
                    <Icon data-icon='inline-start' />
                  </span>
                  <div className='min-w-0'>
                    <p className='text-sm leading-5 font-medium'>
                      {t(item.key)}
                    </p>
                    <p className='text-muted-foreground text-xs leading-4'>
                      {t('Unified routing')}
                    </p>
                  </div>
                </div>
              )
            })}
          </div>

          <div className='text-muted-foreground mt-7 flex flex-wrap items-center gap-2 text-sm'>
            <span className='text-foreground/70 font-medium'>
              {t('Route requests across providers')}
            </span>
            <Separator orientation='vertical' className='hidden h-4 md:block' />
            <span>{t('Unified keys, billing, and observability')}</span>
          </div>

          <div className='mt-5 flex flex-wrap items-center gap-2'>
            {TRUST_POINTS.map((item) => (
              <Badge
                key={item}
                variant='outline'
                className='rounded-full border-border/60 px-2.5 py-0.5 text-[11px] font-medium'
              >
                {item}
              </Badge>
            ))}
          </div>
        </div>

        <div className='relative min-w-0'>
          <div className='absolute -inset-x-6 top-10 -z-10 h-[24rem] rounded-[2rem] bg-[radial-gradient(circle_at_50%_18%,color-mix(in_oklch,var(--primary)_12%,transparent)_0%,transparent_52%),radial-gradient(circle_at_50%_90%,color-mix(in_oklch,var(--accent)_18%,transparent)_0%,transparent_56%)] opacity-90 blur-2xl' />
          <div
            className='landing-animate-scale-in border-border/60 bg-background/80 rounded-[2rem] border p-3 opacity-0 shadow-[0_26px_70px_-34px_rgba(15,23,42,0.4)] backdrop-blur-sm md:p-4'
            style={{ animationDelay: '240ms' }}
          >
            <div className='border-border/60 bg-muted/30 flex items-center justify-between rounded-[1.25rem] border px-4 py-3'>
              <div className='flex items-center gap-3'>
                <span className='size-2 rounded-full bg-emerald-500 shadow-[0_0_0_4px_rgba(34,197,94,0.12)]' />
                <div>
                  <p className='text-xs font-semibold tracking-[0.18em] uppercase'>
                    {t('Live gateway')}
                  </p>
                  <p className='text-muted-foreground text-[11px]'>
                    {t('Observability')}
                  </p>
                </div>
              </div>
              <Badge
                variant='secondary'
                className='rounded-full px-2.5 py-0.5 text-[11px]'
              >
                {t('Healthy')}
              </Badge>
            </div>

            <div className='border-border/60 bg-background/95 mt-3 overflow-hidden rounded-[1.35rem] border'>
              <div className='border-border/50 flex items-center gap-2 overflow-x-auto border-b px-4 py-3'>
                {[t('Request'), t('Response'), t('Channels')].map(
                  (label, index) => (
                    <button
                      key={label}
                      className={cn(
                        'shrink-0 rounded-full px-3 py-1 text-[11px] font-medium transition-colors',
                        index === 0
                          ? 'bg-primary text-primary-foreground'
                          : 'text-muted-foreground hover:bg-muted hover:text-foreground'
                      )}
                      type='button'
                    >
                      {label}
                    </button>
                  )
                )}
                <div className='text-muted-foreground ml-auto flex shrink-0 items-center gap-2 text-[11px]'>
                  <span>{t('Latency')}</span>
                  <Separator orientation='vertical' className='h-3' />
                  <span>{t('Usage')}</span>
                </div>
              </div>

              <HeroTerminalDemo />
            </div>
          </div>
        </div>
      </div>
    </section>
  )
}
