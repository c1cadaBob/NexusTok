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
import { Main } from '@/components/layout'
import { DEFAULT_BILLING_SETTINGS } from '@/features/system-settings/billing'
import {
  getGroupDefaults,
  getModelDefaults,
} from '@/features/system-settings/billing/section-registry'
import {
  getOptionValue,
  useSystemOptions,
} from '@/features/system-settings/hooks/use-system-options'
import { RatioSettingsCard } from '@/features/system-settings/models/ratio-settings-card'
import type { BillingSettings } from '@/features/system-settings/types'

export function PricingSettings() {
  const { t } = useTranslation()
  const { data, isLoading } = useSystemOptions()

  if (isLoading) {
    return (
      <Main>
        <div className='flex items-center justify-center py-12'>
          <div className='text-muted-foreground'>
            {t('Loading settings...')}
          </div>
        </div>
      </Main>
    )
  }

  const settings = getOptionValue(
    data?.data,
    DEFAULT_BILLING_SETTINGS
  ) as BillingSettings

  return (
    <Main>
      <div className='min-h-0 flex-1 px-4 pt-6 pb-4'>
        <div className='h-full w-full overflow-y-auto scroll-smooth pe-4 pb-12'>
          <RatioSettingsCard
            titleKey='Model Pricing Groups'
            descriptionKey='Configure model, caching, and group ratios used for billing'
            modelDefaults={getModelDefaults(settings)}
            groupDefaults={getGroupDefaults(settings)}
            toolPricesDefault={settings['tool_price_setting.prices']}
          />
        </div>
      </div>
    </Main>
  )
}
