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
  Database,
  GitBranch,
  KeyRound,
  RadioTower,
  ReceiptText,
  Route,
  ShieldCheck,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { DEFAULT_LOGO } from '@/lib/constants'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'

interface HeroProps {
  className?: string
  isAuthenticated?: boolean
}

const COMPATIBILITY_KEYS = [
  'OpenAI compatible',
  'Claude compatible',
  'Gemini compatible',
] as const

const SPEC_ITEMS = [
  {
    value: '40+',
    label: 'upstream providers',
    description: 'OpenAI, Claude, Gemini, Azure, Bedrock, and more',
  },
  {
    value: '3',
    label: 'database engines',
    description: 'SQLite, MySQL, and PostgreSQL',
  },
  {
    value: '4',
    label: 'auth patterns',
    description: 'JWT, OAuth providers, WebAuthn passkeys',
  },
] as const

const WORKFLOW_ITEMS = [
  {
    icon: KeyRound,
    title: 'Client request',
    description: 'Bearer key validation',
  },
  {
    icon: GitBranch,
    title: 'Routing policy',
    description: 'Group-based model access',
  },
  {
    icon: RadioTower,
    title: 'Provider pool',
    description: 'Fallback on provider error',
  },
  {
    icon: ReceiptText,
    title: 'Usage settlement',
    description: 'Token usage captured',
  },
] as const

const PROVIDER_ROWS = [
  { name: 'OpenAI', endpoint: '/v1/chat/completions', status: 'Active' },
  { name: 'Anthropic', endpoint: '/v1/messages', status: 'Active' },
  { name: 'Google Gemini', endpoint: '/v1beta/models', status: 'Active' },
  { name: 'AWS Bedrock', endpoint: 'bedrock-runtime', status: 'Standby' },
] as const

function GatewayWorkbench() {
  const { t } = useTranslation()

  return (
    <div className='bg-card text-card-foreground overflow-hidden rounded-lg border shadow-xs'>
      <div className='flex flex-col gap-4 border-b p-4 md:flex-row md:items-center md:justify-between'>
        <div className='flex items-center gap-3'>
          <div className='bg-background flex size-9 items-center justify-center rounded-lg border'>
            <img
              src={DEFAULT_LOGO}
              alt='NexusTok'
              className='size-6 object-contain'
            />
          </div>
          <div className='flex min-w-0 flex-col gap-0.5'>
            <p className='text-sm font-medium'>
              {t('Gateway operations board')}
            </p>
            <p className='text-muted-foreground text-xs'>
              {t('Request lifecycle')}
            </p>
          </div>
        </div>
        <Badge variant='secondary'>{t('Unified request formats')}</Badge>
      </div>

      <div className='bg-border grid gap-px md:grid-cols-4'>
        {WORKFLOW_ITEMS.map((item, index) => {
          const Icon = item.icon

          return (
            <div key={item.title} className='bg-card p-4'>
              <div className='flex items-center justify-between gap-3'>
                <span className='text-muted-foreground font-mono text-xs tabular-nums'>
                  0{index + 1}
                </span>
                <Icon className='text-muted-foreground size-4' aria-hidden />
              </div>
              <div className='mt-5 flex flex-col gap-1.5'>
                <p className='text-sm font-medium'>{t(item.title)}</p>
                <p className='text-muted-foreground text-xs leading-relaxed'>
                  {t(item.description)}
                </p>
              </div>
            </div>
          )
        })}
      </div>

      <div className='bg-border grid gap-px lg:grid-cols-[1.1fr_0.9fr]'>
        <div className='bg-card p-4 md:p-5'>
          <div className='mb-4 flex items-center justify-between gap-3'>
            <div className='flex items-center gap-2'>
              <Route className='text-muted-foreground size-4' aria-hidden />
              <p className='text-sm font-medium'>{t('Provider routing')}</p>
            </div>
            <Badge variant='outline'>POST</Badge>
          </div>
          <div className='overflow-hidden rounded-lg border'>
            {PROVIDER_ROWS.map((provider) => (
              <div
                key={provider.name}
                className='grid grid-cols-[1fr_auto] gap-3 border-b px-3 py-2.5 last:border-b-0'
              >
                <div className='min-w-0'>
                  <p className='truncate text-sm font-medium'>
                    {provider.name}
                  </p>
                  <p className='text-muted-foreground truncate font-mono text-xs'>
                    {provider.endpoint}
                  </p>
                </div>
                <Badge
                  variant={
                    provider.status === 'Active' ? 'secondary' : 'outline'
                  }
                >
                  {t(provider.status)}
                </Badge>
              </div>
            ))}
          </div>
        </div>

        <div className='bg-card p-4 md:p-5'>
          <div className='mb-4 flex items-center gap-2'>
            <Database className='text-muted-foreground size-4' aria-hidden />
            <p className='text-sm font-medium'>
              {t('Quota, billing, and logs')}
            </p>
          </div>
          <div className='grid gap-3'>
            {[
              ['Model pricing', 'Dynamic ratios and groups'],
              ['Usage logs', 'Tokens, cost, latency, errors'],
              ['Access control', 'Users, keys, and permissions'],
            ].map(([title, description]) => (
              <div
                key={title}
                className='bg-muted/40 rounded-lg border px-3 py-2.5'
              >
                <div className='flex items-center justify-between gap-3'>
                  <p className='text-sm font-medium'>{t(title)}</p>
                  <ShieldCheck
                    className='text-muted-foreground size-4'
                    aria-hidden
                  />
                </div>
                <p className='text-muted-foreground mt-1 text-xs'>
                  {t(description)}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  )
}

export function Hero(props: HeroProps) {
  const { t } = useTranslation()

  return (
    <section
      className={cn(
        'relative overflow-hidden border-b px-6 pt-28 pb-10 md:pt-32 md:pb-12',
        props.className
      )}
    >
      <div
        aria-hidden
        className='pointer-events-none absolute inset-0 bg-[linear-gradient(to_right,var(--border)_1px,transparent_1px),linear-gradient(to_bottom,var(--border)_1px,transparent_1px)] [mask-image:linear-gradient(to_bottom,black,transparent_92%)] bg-[size:4rem_4rem] opacity-45'
      />

      <div className='relative mx-auto flex max-w-7xl flex-col gap-10'>
        <div className='grid gap-8 lg:grid-cols-[1.05fr_0.95fr] lg:items-end'>
          <div className='flex flex-col gap-7'>
            <div className='flex flex-wrap items-center gap-2'>
              {COMPATIBILITY_KEYS.map((item) => (
                <Badge key={item} variant='outline'>
                  {t(item)}
                </Badge>
              ))}
            </div>

            <div className='flex max-w-3xl flex-col gap-5'>
              <h1 className='text-4xl leading-[1.05] font-semibold tracking-tight text-balance md:text-6xl'>
                {t('NexusTok AI API Gateway')}
              </h1>
              <p className='text-muted-foreground max-w-2xl text-base leading-relaxed md:text-lg'>
                {t(
                  'One control plane for model routing, billing, access control, and provider failover across OpenAI, Claude, Gemini, Azure, AWS Bedrock, and more.'
                )}
              </p>
            </div>

            <div className='flex flex-col gap-3 sm:flex-row'>
              {props.isAuthenticated ? (
                <Button size='lg' render={<Link to='/dashboard' />}>
                  {t('Go to Dashboard')}
                  <ArrowRight data-icon='inline-end' />
                </Button>
              ) : (
                <>
                  <Button size='lg' render={<Link to='/sign-up' />}>
                    {t('Get Started')}
                    <ArrowRight data-icon='inline-end' />
                  </Button>
                  <Button
                    size='lg'
                    variant='outline'
                    render={<Link to='/pricing' />}
                  >
                    {t('View Pricing')}
                  </Button>
                </>
              )}
            </div>
          </div>

          <div className='bg-border grid gap-px overflow-hidden rounded-lg border sm:grid-cols-3'>
            {SPEC_ITEMS.map((item) => (
              <div key={item.label} className='bg-background p-4'>
                <div className='font-mono text-2xl font-semibold tabular-nums'>
                  {item.value}
                </div>
                <p className='mt-2 text-sm font-medium'>{t(item.label)}</p>
                <p className='text-muted-foreground mt-1 text-xs leading-relaxed'>
                  {t(item.description)}
                </p>
              </div>
            ))}
          </div>
        </div>

        <GatewayWorkbench />
      </div>
    </section>
  )
}
