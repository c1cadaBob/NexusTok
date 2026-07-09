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
import { useEffect, useMemo, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useQueryClient } from '@tanstack/react-query'
import { AddCircleIcon, DatabaseSyncIcon } from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import {
  createWaffoPancakePair,
  listWaffoPancakeCatalog,
  saveWaffoPancakeConfig,
} from '../api'
import { SettingsSection } from '../components/settings-section'
import { useSystemSettingPermissions } from '../hooks/use-system-setting-permissions'
import { useUpdateOption } from '../hooks/use-update-option'
import type { WaffoPancakeCatalogStore } from '../types'
import { removeTrailingSlash } from './utils'

export interface WaffoPancakeSettingsValues {
  WaffoPancakeEnabled: boolean
  WaffoPancakeSandbox: boolean
  WaffoPancakeMerchantID: string
  WaffoPancakePrivateKey: string
  WaffoPancakeWebhookPublicKey: string
  WaffoPancakeWebhookTestKey: string
  WaffoPancakeStoreID: string
  WaffoPancakeProductID: string
  WaffoPancakeReturnURL: string
  WaffoPancakeCurrency: string
  WaffoPancakeUnitPrice: number
  WaffoPancakeMinTopUp: number
}

interface Props {
  defaultValues: WaffoPancakeSettingsValues
}

type CatalogSelectItem = {
  value: string
  label: string
}

export function WaffoPancakeSettingsSection(props: Props) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const permissions = useSystemSettingPermissions()
  const updateOption = useUpdateOption()
  const canUpdateWaffoPancakeSettings = updateOption.canUpdate
  const waffoPancakeUpdateDisabledReason = updateOption.disabledReason
  const [loading, setLoading] = useState(false)
  const [catalogLoading, setCatalogLoading] = useState(false)
  const [pairLoading, setPairLoading] = useState(false)
  const [catalogStores, setCatalogStores] = useState<
    WaffoPancakeCatalogStore[]
  >([])
  const form = useForm<WaffoPancakeSettingsValues>({
    defaultValues: props.defaultValues,
  })
  const watchedStoreID = form.watch('WaffoPancakeStoreID') || ''
  const watchedProductID = form.watch('WaffoPancakeProductID') || ''
  const noPermissionMessage = t("You don't have necessary permission")

  useEffect(() => {
    form.reset(props.defaultValues)
  }, [props.defaultValues, form])

  const storeSelectItems = useMemo<CatalogSelectItem[]>(() => {
    const items = catalogStores.map((store) => ({
      value: store.id,
      label: store.name ? `${store.name} (${store.id})` : store.id,
    }))
    const currentStoreID = watchedStoreID.trim()
    if (
      currentStoreID &&
      !items.some((item) => item.value === currentStoreID)
    ) {
      return [{ value: currentStoreID, label: currentStoreID }, ...items]
    }
    return items
  }, [catalogStores, watchedStoreID])

  const selectedCatalogStore = useMemo(
    () => catalogStores.find((store) => store.id === watchedStoreID.trim()),
    [catalogStores, watchedStoreID]
  )

  const productSelectItems = useMemo<CatalogSelectItem[]>(() => {
    const products = selectedCatalogStore?.onetimeProducts || []
    const items = products.map((product) => ({
      value: product.id,
      label: product.name ? `${product.name} (${product.id})` : product.id,
    }))
    const currentProductID = watchedProductID.trim()
    if (
      currentProductID &&
      !items.some((item) => item.value === currentProductID)
    ) {
      return [{ value: currentProductID, label: currentProductID }, ...items]
    }
    return items
  }, [selectedCatalogStore, watchedProductID])

  const mergeCreatedStore = (
    stores: WaffoPancakeCatalogStore[],
    storeID: string,
    storeName: string,
    productID?: string,
    productName?: string
  ) => {
    if (!storeID) return stores
    const product =
      productID && productName
        ? { id: productID, name: productName, status: 'active' }
        : null
    const nextStore: WaffoPancakeCatalogStore = {
      id: storeID,
      name: storeName || storeID,
      status: 'active',
      prodEnabled: true,
      onetimeProducts: product ? [product] : [],
    }

    const exists = stores.some((store) => store.id === storeID)
    if (!exists) return [nextStore, ...stores]

    return stores.map((store) => {
      if (store.id !== storeID) return store
      if (
        !product ||
        store.onetimeProducts.some((item) => item.id === product.id)
      ) {
        return { ...store, name: store.name || nextStore.name }
      }
      return {
        ...store,
        name: store.name || nextStore.name,
        onetimeProducts: [product, ...store.onetimeProducts],
      }
    })
  }

  const applyCatalogSelection = (stores: WaffoPancakeCatalogStore[]) => {
    const values = form.getValues()
    const currentStoreID = values.WaffoPancakeStoreID.trim()
    const currentProductID = values.WaffoPancakeProductID.trim()
    const currentStore = stores.find((store) => store.id === currentStoreID)
    const nextStore = currentStore || stores[0]

    if (!nextStore) return

    if (!currentStoreID) {
      form.setValue('WaffoPancakeStoreID', nextStore.id, {
        shouldDirty: true,
      })
    }
    if (currentStoreID && !currentStore) return

    const products = nextStore.onetimeProducts || []
    const productStillAvailable = products.some(
      (product) => product.id === currentProductID
    )
    if (!currentProductID || !productStillAvailable) {
      form.setValue('WaffoPancakeProductID', products[0]?.id || '', {
        shouldDirty: true,
      })
    }
  }

  const handleFetchCatalog = async () => {
    if (!permissions.canOperate) {
      toast.error(noPermissionMessage)
      return
    }

    const values = form.getValues()
    const merchantID = values.WaffoPancakeMerchantID.trim()
    const privateKey = values.WaffoPancakePrivateKey.trim()
    if (privateKey && !merchantID) {
      toast.error(t('Merchant ID is required'))
      return
    }

    setCatalogLoading(true)
    try {
      const result = await listWaffoPancakeCatalog(
        privateKey
          ? {
              merchant_id: merchantID,
              private_key: privateKey,
            }
          : undefined
      )
      if (!result.success) {
        throw new Error(result.message || t('Catalog fetch failed'))
      }

      const stores = result.data?.stores || []
      setCatalogStores(stores)
      applyCatalogSelection(stores)
      toast.success(stores.length > 0 ? t('Catalog loaded') : t('No stores found'))
    } catch (error) {
      toast.error(
        error instanceof Error ? error.message : t('Catalog fetch failed')
      )
    } finally {
      setCatalogLoading(false)
    }
  }

  const handleCreatePair = async () => {
    if (!canUpdateWaffoPancakeSettings) {
      toast.error(waffoPancakeUpdateDisabledReason)
      return
    }

    const values = form.getValues()
    const merchantID = values.WaffoPancakeMerchantID.trim()
    const privateKey = values.WaffoPancakePrivateKey.trim()
    if (privateKey && !merchantID) {
      toast.error(t('Merchant ID is required'))
      return
    }

    setPairLoading(true)
    try {
      const result = await createWaffoPancakePair({
        merchant_id: merchantID,
        private_key: privateKey,
        return_url: removeTrailingSlash(values.WaffoPancakeReturnURL || ''),
      })

      const data = result.data
      if (!result.success) {
        if (data?.orphan_store && data.store_id) {
          form.setValue('WaffoPancakeStoreID', data.store_id, {
            shouldDirty: true,
          })
          setCatalogStores((stores) =>
            mergeCreatedStore(stores, data.store_id || '', data.store_name || '')
          )
          toast.error(t('Store created but product creation failed'))
          return
        }
        throw new Error(result.message || t('Create store + product failed'))
      }

      if (data?.store_id) {
        form.setValue('WaffoPancakeStoreID', data.store_id, {
          shouldDirty: true,
        })
      }
      if (data?.product_id) {
        form.setValue('WaffoPancakeProductID', data.product_id, {
          shouldDirty: true,
        })
      }
      if (data?.store_id) {
        setCatalogStores((stores) =>
          mergeCreatedStore(
            stores,
            data.store_id || '',
            data.store_name || '',
            data.product_id,
            data.product_name
          )
        )
      }
      toast.success(t('Store and product created'))
    } catch (error) {
      toast.error(
        error instanceof Error
          ? error.message
          : t('Create store + product failed')
      )
    } finally {
      setPairLoading(false)
    }
  }

  const handleSave = async () => {
    if (!canUpdateWaffoPancakeSettings) {
      toast.error(waffoPancakeUpdateDisabledReason)
      return
    }

    const values = form.getValues()
    const enabled = !!values.WaffoPancakeEnabled
    const sandbox = !!values.WaffoPancakeSandbox

    if (enabled && !values.WaffoPancakeMerchantID.trim()) {
      toast.error(t('Merchant ID is required'))
      return
    }

    if (enabled && !values.WaffoPancakeStoreID.trim()) {
      toast.error(t('Store ID is required'))
      return
    }

    if (enabled && !values.WaffoPancakeProductID.trim()) {
      toast.error(t('Product ID is required'))
      return
    }

    const requiredWebhookKey = sandbox
      ? values.WaffoPancakeWebhookTestKey
      : values.WaffoPancakeWebhookPublicKey
    if (enabled && !String(requiredWebhookKey || '').trim()) {
      toast.error(
        sandbox
          ? t('Webhook public key (sandbox) is required')
          : t('Webhook public key (production) is required')
      )
      return
    }

    if (enabled && Number(values.WaffoPancakeUnitPrice) <= 0) {
      toast.error(t('Unit price must be greater than 0'))
      return
    }

    if (enabled && Number(values.WaffoPancakeMinTopUp) < 1) {
      toast.error(t('Minimum top-up amount must be at least 1'))
      return
    }

    setLoading(true)
    try {
      const result = await saveWaffoPancakeConfig({
        enabled,
        sandbox,
        merchant_id: values.WaffoPancakeMerchantID || '',
        private_key: values.WaffoPancakePrivateKey || '',
        webhook_public_key: values.WaffoPancakeWebhookPublicKey || '',
        webhook_test_key: values.WaffoPancakeWebhookTestKey || '',
        store_id: values.WaffoPancakeStoreID || '',
        product_id: values.WaffoPancakeProductID || '',
        return_url: removeTrailingSlash(values.WaffoPancakeReturnURL || ''),
        currency: values.WaffoPancakeCurrency || 'USD',
        unit_price: Number(values.WaffoPancakeUnitPrice ?? 1),
        min_top_up: Number(values.WaffoPancakeMinTopUp ?? 1),
      })
      if (!result.success) {
        throw new Error(result.message || t('Update failed'))
      }
      await queryClient.invalidateQueries({ queryKey: ['system-options'] })
      toast.success(t('Updated successfully'))
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t('Update failed'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <SettingsSection
      title={t('Waffo Pancake Payment Gateway')}
      description={t(
        'Configure Waffo Pancake hosted checkout integration for USD-priced top-ups'
      )}
    >
      <Alert>
        <AlertDescription className='text-xs'>
          {t(
            'Obtain the merchant, store, product and signing keys from your Waffo dashboard. Webhook URL: <ServerAddress>/api/waffo-pancake/webhook'
          )}
        </AlertDescription>
      </Alert>

      <div className='grid grid-cols-3 gap-4'>
        <div className='flex items-center gap-2'>
          <Switch
            checked={form.watch('WaffoPancakeEnabled')}
            onCheckedChange={(value) =>
              form.setValue('WaffoPancakeEnabled', value)
            }
          />
          <Label>{t('Enable Waffo Pancake')}</Label>
        </div>
        <div className='flex items-center gap-2'>
          <Switch
            checked={form.watch('WaffoPancakeSandbox')}
            onCheckedChange={(value) =>
              form.setValue('WaffoPancakeSandbox', value)
            }
          />
          <Label>{t('Sandbox mode')}</Label>
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Currency')}</Label>
          <Input placeholder='USD' {...form.register('WaffoPancakeCurrency')} />
        </div>
      </div>

      <div className='grid grid-cols-3 gap-4'>
        <div className='grid gap-1.5'>
          <Label>{t('Merchant ID')}</Label>
          <Input
            placeholder='MER_xxx'
            {...form.register('WaffoPancakeMerchantID')}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Store ID')}</Label>
          <Input
            placeholder='STO_xxx'
            {...form.register('WaffoPancakeStoreID')}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Product ID')}</Label>
          <Input
            placeholder='PROD_xxx'
            {...form.register('WaffoPancakeProductID')}
          />
        </div>
      </div>

      <div className='grid gap-3'>
        <div className='flex flex-wrap items-center justify-between gap-2'>
          <Label>{t('Pancake catalog')}</Label>
          <div className='flex flex-wrap gap-2'>
            <Button
              variant='outline'
              size='sm'
              onClick={handleFetchCatalog}
              disabled={
                catalogLoading || pairLoading || !permissions.canOperate
              }
              title={permissions.canOperate ? undefined : noPermissionMessage}
            >
              <HugeiconsIcon
                icon={DatabaseSyncIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {catalogLoading ? t('Fetching...') : t('Refresh catalog')}
            </Button>
            <Button
              variant='outline'
              size='sm'
              onClick={handleCreatePair}
              disabled={
                pairLoading ||
                catalogLoading ||
                !canUpdateWaffoPancakeSettings
              }
              title={
                canUpdateWaffoPancakeSettings
                  ? undefined
                  : waffoPancakeUpdateDisabledReason
              }
            >
              <HugeiconsIcon
                icon={AddCircleIcon}
                strokeWidth={2}
                data-icon='inline-start'
              />
              {pairLoading ? t('Creating...') : t('Create store + product')}
            </Button>
          </div>
        </div>

        <div className='grid grid-cols-2 gap-4'>
          <div className='grid gap-1.5'>
            <Label>{t('Store ID')}</Label>
            <Select<string>
              items={storeSelectItems}
              value={watchedStoreID || null}
              onValueChange={(value) => {
                if (value === null) return
                form.setValue('WaffoPancakeStoreID', value, {
                  shouldDirty: true,
                })
                const store = catalogStores.find((item) => item.id === value)
                if (store) {
                  form.setValue(
                    'WaffoPancakeProductID',
                    store.onetimeProducts?.[0]?.id || '',
                    { shouldDirty: true }
                  )
                }
              }}
              disabled={storeSelectItems.length === 0}
            >
              <SelectTrigger className='w-full'>
                <SelectValue
                  placeholder={
                    storeSelectItems.length > 0
                      ? t('Select Store')
                      : t('No stores found')
                  }
                />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {storeSelectItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <div className='grid gap-1.5'>
            <Label>{t('Product ID')}</Label>
            <Select<string>
              items={productSelectItems}
              value={watchedProductID || null}
              onValueChange={(value) => {
                if (value === null) return
                form.setValue('WaffoPancakeProductID', value, {
                  shouldDirty: true,
                })
              }}
              disabled={productSelectItems.length === 0}
            >
              <SelectTrigger className='w-full'>
                <SelectValue
                  placeholder={
                    productSelectItems.length > 0
                      ? t('Select Product')
                      : t('No active products found')
                  }
                />
              </SelectTrigger>
              <SelectContent alignItemWithTrigger={false}>
                <SelectGroup>
                  {productSelectItems.map((item) => (
                    <SelectItem key={item.value} value={item.value}>
                      {item.label}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
        </div>
      </div>

      <div className='grid grid-cols-2 gap-4'>
        <div className='grid gap-1.5'>
          <Label>{t('API Private Key')}</Label>
          <Textarea
            rows={3}
            placeholder={t('Leave blank to keep the existing key')}
            {...form.register('WaffoPancakePrivateKey')}
            className='font-mono text-xs'
          />
          <p className='text-muted-foreground text-xs'>
            {t('Stored value is not echoed back for security')}
          </p>
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Payment return URL')}</Label>
          <Input
            placeholder='https://example.com/console/topup'
            {...form.register('WaffoPancakeReturnURL')}
          />
          <p className='text-muted-foreground text-xs'>
            {t('Defaults to the wallet page when empty')}
          </p>
        </div>
      </div>

      <div className='grid grid-cols-2 gap-4'>
        <div className='grid gap-1.5'>
          <Label>{t('Webhook public key (production)')}</Label>
          <Textarea
            rows={3}
            placeholder={t('Leave blank to keep the existing key')}
            {...form.register('WaffoPancakeWebhookPublicKey')}
            className='font-mono text-xs'
          />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Webhook public key (sandbox)')}</Label>
          <Textarea
            rows={3}
            placeholder={t('Leave blank to keep the existing key')}
            {...form.register('WaffoPancakeWebhookTestKey')}
            className='font-mono text-xs'
          />
        </div>
      </div>

      <div className='grid grid-cols-2 gap-4'>
        <div className='grid gap-1.5'>
          <Label>{t('Unit price (local currency / USD)')}</Label>
          <Input
            type='number'
            step={0.01}
            min={0}
            {...form.register('WaffoPancakeUnitPrice', { valueAsNumber: true })}
          />
        </div>
        <div className='grid gap-1.5'>
          <Label>{t('Minimum top-up (USD)')}</Label>
          <Input
            type='number'
            min={1}
            {...form.register('WaffoPancakeMinTopUp', { valueAsNumber: true })}
          />
        </div>
      </div>

      <Button
        onClick={handleSave}
        disabled={
          loading || updateOption.isPending || !canUpdateWaffoPancakeSettings
        }
        title={
          canUpdateWaffoPancakeSettings
            ? undefined
            : waffoPancakeUpdateDisabledReason
        }
      >
        {loading ? t('Saving...') : t('Save Waffo Pancake settings')}
      </Button>
    </SettingsSection>
  )
}
