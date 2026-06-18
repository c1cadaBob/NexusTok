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
  BarChart3,
  CreditCard,
  DatabaseZap,
  KeyRound,
  Layers3,
  LineChart,
  ShieldCheck,
  Workflow,
  type LucideIcon,
} from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { AnimateInView } from '@/components/animate-in-view'

interface FeaturesProps {
  className?: string
}

interface FeatureItem {
  icon: LucideIcon
  title: string
  description: string
  detail: string
  scope: string
}

const FEATURE_ITEMS: FeatureItem[] = [
  {
    icon: Layers3,
    title: 'Provider aggregation',
    description:
      'Route requests across OpenAI, Claude, Gemini, Azure, Bedrock, and self-hosted models.',
    detail: '40+ providers',
    scope: 'Channels and models',
  },
  {
    icon: Workflow,
    title: 'Format conversion',
    description:
      'Expose OpenAI-compatible, Claude-compatible, and Gemini-compatible routes from one gateway.',
    detail: 'Unified API',
    scope: 'Relay adapters',
  },
  {
    icon: CreditCard,
    title: 'Billing controls',
    description:
      'Manage model ratios, groups, subscriptions, redemption codes, and wallet balances.',
    detail: 'Quota aware',
    scope: 'Pricing and wallet',
  },
  {
    icon: KeyRound,
    title: 'Key management',
    description:
      'Issue user keys, scope access, apply rate limits, and audit usage by credential.',
    detail: 'Access scoped',
    scope: 'API keys',
  },
  {
    icon: ShieldCheck,
    title: 'Identity and security',
    description:
      'Support JWT sessions, OAuth providers, OIDC, Discord, GitHub, and WebAuthn passkeys.',
    detail: 'Passkey ready',
    scope: 'Auth settings',
  },
  {
    icon: BarChart3,
    title: 'Usage observability',
    description:
      'Inspect request logs, token usage, cost, latency, channel status, and error patterns.',
    detail: 'Log centered',
    scope: 'Logs and charts',
  },
  {
    icon: DatabaseZap,
    title: 'Deployment flexibility',
    description:
      'Run with SQLite, MySQL, or PostgreSQL while keeping Redis optional for cache acceleration.',
    detail: 'Self-hosted',
    scope: 'Runtime options',
  },
  {
    icon: LineChart,
    title: 'Operational dashboards',
    description:
      'Give admins and users focused views for channels, models, pricing, wallet, and usage.',
    detail: 'Role aware',
    scope: 'Dashboard views',
  },
]

export function Features(props: FeaturesProps) {
  const { t } = useTranslation()

  return (
    <section className={cn('px-6 py-16 md:py-20', props.className)}>
      <div className='mx-auto max-w-7xl'>
        <div className='mb-8 grid gap-4 md:grid-cols-[0.8fr_1.2fr] md:items-end'>
          <AnimateInView>
            <Badge variant='outline'>{t('Core Features')}</Badge>
            <h2 className='mt-4 text-2xl leading-tight font-semibold tracking-tight md:text-3xl'>
              {t('One gateway surface for daily AI operations.')}
            </h2>
          </AnimateInView>
          <AnimateInView delay={80} animation='fade-left'>
            <p className='text-muted-foreground max-w-2xl text-sm leading-relaxed md:justify-self-end'>
              {t(
                'NexusTok brings provider routing, identity, quota, pricing, and request logs into the same control plane so operators can adjust upstream strategy without changing client code.'
              )}
            </p>
          </AnimateInView>
        </div>

        <div className='grid gap-3 sm:grid-cols-2 lg:grid-cols-4'>
          {FEATURE_ITEMS.map((feature, index) => {
            const Icon = feature.icon

            return (
              <AnimateInView key={feature.title} delay={index * 40}>
                <Card className='h-full'>
                  <CardHeader>
                    <div className='mb-2 flex items-center justify-between gap-3'>
                      <span className='bg-muted flex size-9 items-center justify-center rounded-lg border'>
                        <Icon
                          className='text-muted-foreground size-4'
                          aria-hidden
                        />
                      </span>
                      <Badge variant='secondary'>{t(feature.detail)}</Badge>
                    </div>
                    <CardTitle>{t(feature.title)}</CardTitle>
                    <CardDescription>{t(feature.description)}</CardDescription>
                  </CardHeader>
                  <CardContent className='mt-auto'>
                    <div className='flex items-center justify-between gap-3 border-t pt-3'>
                      <span className='text-muted-foreground text-xs'>
                        {t('Surface')}
                      </span>
                      <span className='text-xs font-medium'>
                        {t(feature.scope)}
                      </span>
                    </div>
                  </CardContent>
                </Card>
              </AnimateInView>
            )
          })}
        </div>
      </div>
    </section>
  )
}
