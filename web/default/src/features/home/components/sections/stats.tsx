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
import { useTranslation } from 'react-i18next'

interface StatsProps {
  className?: string
}

interface StatItem {
  value: string
  label: string
  description: string
}

export function Stats(_props: StatsProps) {
  const { t } = useTranslation()

  const stats: StatItem[] = [
    {
      value: '40+',
      label: t('providers'),
      description: t(
        'OpenAI, Claude, Gemini, Azure, Bedrock, and local models'
      ),
    },
    {
      value: '3',
      label: t('databases'),
      description: t('SQLite, MySQL, and PostgreSQL supported by one codepath'),
    },
    {
      value: '7',
      label: t('locale packs'),
      description: t(
        'English, Simplified Chinese, Traditional Chinese, French, Japanese, Russian, Vietnamese'
      ),
    },
    {
      value: '1',
      label: t('management plane'),
      description: t('Users, keys, channels, pricing, logs, and wallet'),
    },
  ]

  return (
    <section className='bg-muted/20 border-b px-6'>
      <div className='mx-auto max-w-7xl'>
        <div className='bg-border grid gap-px md:grid-cols-4'>
          {stats.map((s) => (
            <div key={s.label} className='bg-background p-5 md:p-6'>
              <div className='font-mono text-3xl font-semibold tracking-tight tabular-nums'>
                {s.value}
              </div>
              <p className='mt-3 text-sm font-medium'>{s.label}</p>
              <p className='text-muted-foreground mt-1.5 text-xs leading-relaxed'>
                {s.description}
              </p>
            </div>
          ))}
        </div>
      </div>
    </section>
  )
}
