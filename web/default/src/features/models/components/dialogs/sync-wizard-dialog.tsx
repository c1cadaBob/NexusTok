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
import { useQueryClient } from '@tanstack/react-query'
import type { TFunction } from 'i18next'
import { GripVertical, Loader2, RefreshCw, ShieldCheck } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { cn } from '@/lib/utils'
import { useIsMobile } from '@/hooks/use-mobile'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Label } from '@/components/ui/label'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { StatusBadge } from '@/components/status-badge'
import { syncUpstream, previewUpstreamDiff } from '../../api'
import { getSyncLocaleOptions, getSyncSourceOptions } from '../../constants'
import { useModelPermissions } from '../../hooks/use-model-permissions'
import { modelsQueryKeys, vendorsQueryKeys } from '../../lib'
import type { CatalogWriteBackInfo, SyncLocale, SyncSource } from '../../types'
import { useModels } from '../models-provider'

type SyncWizardDialogProps = {
  open: boolean
  onOpenChange: (open: boolean) => void
}

const DEFAULT_PROVIDER_ORDER = ['openai', 'anthropic', 'google', 'azure']

function getCatalogSourceMessage(
  t: TFunction,
  source?: {
    catalog_origin?: string
    fallback_stage?: string
    github_repo?: string
    fallback_name?: string
    catalog_version?: string
  }
) {
  switch (source?.catalog_origin) {
    case 'models_dev_github':
      return t(
        'Models.dev website was unavailable, so GitHub fallback was used.'
      )
    case 'nexustok_embedded':
      return t(
        'External model sources were unavailable, so the built-in NexusTok repository was used.'
      )
    case 'nexustok_repository':
      return t('Synced from the built-in NexusTok model repository.')
    case 'models_dev_web':
      return t('Synced from models.dev website.')
    default:
      if (source?.fallback_stage === 'github') {
        return t(
          'Models.dev website was unavailable, so GitHub fallback was used.'
        )
      }
      if (source?.fallback_stage === 'embedded') {
        return t(
          'External model sources were unavailable, so the built-in NexusTok repository was used.'
        )
      }
      return ''
  }
}

function showCatalogWriteBackToast(
  t: TFunction,
  writeBack?: CatalogWriteBackInfo
) {
  if (!writeBack?.status) return
  const modelCount = writeBack.model_count || 0
  const providerCount = writeBack.provider_count || 0
  switch (writeBack.status) {
    case 'success':
      toast.success(
        t(
          'Model catalog repository updated. {{models}} models and {{providers}} providers will be embedded in the next build.',
          { models: modelCount, providers: providerCount }
        )
      )
      return
    case 'skipped':
      toast.warning(
        t(
          'Synced to the database only. Enable MODEL_CATALOG_WRITE_BACK on the development server before release to embed {{models}} models in the next image.',
          { models: modelCount }
        )
      )
      return
    case 'failed':
      toast.error(
        t('Model catalog write-back failed: {{reason}}', {
          reason: writeBack.reason || t('Unknown'),
        })
      )
      return
    default:
      return
  }
}

export function SyncWizardDialog({
  open,
  onOpenChange,
}: SyncWizardDialogProps) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const {
    setOpen,
    setUpstreamConflicts,
    setSyncWizardOptions,
    syncWizardOptions,
  } = useModels()
  const isMobile = useIsMobile()
  const permissions = useModelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const [locale, setLocale] = useState<SyncLocale>('zh')
  const [source, setSource] = useState<SyncSource>('models.dev')
  const [syncPricing, setSyncPricing] = useState(true)
  const [overwriteManualPricing, setOverwriteManualPricing] = useState(false)
  const [providerOrderText, setProviderOrderText] = useState(
    DEFAULT_PROVIDER_ORDER.join('\n')
  )
  const [isSyncing, setIsSyncing] = useState(false)

  // 同步来源和语言选项随当前语言动态翻译。
  const SYNC_SOURCE_OPTIONS = useMemo(() => getSyncSourceOptions(t), [t])
  const SYNC_LOCALE_OPTIONS = useMemo(() => getSyncLocaleOptions(t), [t])

  useEffect(() => {
    if (open) {
      setLocale(syncWizardOptions.locale || 'zh')
      const preferredSource = SYNC_SOURCE_OPTIONS.find(
        (option) => option.value === syncWizardOptions.source
      )
      setSource(
        preferredSource && !preferredSource.disabled
          ? (preferredSource.value as SyncSource)
          : 'models.dev'
      )
      setSyncPricing(true)
      setOverwriteManualPricing(false)
      setProviderOrderText(DEFAULT_PROVIDER_ORDER.join('\n'))
    }
  }, [open, syncWizardOptions, SYNC_SOURCE_OPTIONS])

  const providerOrder = providerOrderText
    .split(/[\n,]/)
    .map((provider) => provider.trim())
    .filter(Boolean)
  const canSyncPricing = source === 'models.dev' || source === 'official'
  const canSyncUpstream = permissions.canOperate && permissions.canWrite

  const handleSync = async () => {
    if (!canSyncUpstream) {
      toast.error(noPermissionMessage)
      return
    }
    setIsSyncing(true)
    try {
      const pricing = {
        enabled: canSyncPricing && syncPricing,
        overwrite_manual: overwriteManualPricing,
        provider_order: providerOrder,
      }
      setSyncWizardOptions({ locale, source, pricing })
      const previewRes = await previewUpstreamDiff({ locale, source })

      if (!previewRes.success) {
        throw new Error(previewRes.message || 'Failed to preview upstream diff')
      }

      const conflicts = previewRes.data?.conflicts || []

      if (conflicts.length > 0) {
        toast.warning(
          t('Found {{count}} conflicts. Please resolve them first.', {
            count: conflicts.length,
          })
        )
        setUpstreamConflicts(conflicts)
        setOpen('upstream-conflict')
        return
      }

      // 无冲突时才真正写入上游同步结果，避免覆盖需要人工确认的模型。
      const response = await syncUpstream({
        locale,
        source,
        pricing,
      })

      if (response.success) {
        const {
          created_models,
          created_vendors,
          updated_models,
          pricing_updated,
          pricing_skipped,
          source: resultSource,
          catalog_write_back,
        } = response.data || {}
        toast.success(
          t(
            'Sync completed. Created {{created}} models, updated {{updated}}, added {{vendors}} vendors, applied pricing to {{priced}} models, skipped {{skipped}} prices.',
            {
              created: created_models || 0,
              updated: updated_models || 0,
              vendors: created_vendors || 0,
              priced: pricing_updated || 0,
              skipped: pricing_skipped || 0,
            }
          )
        )
        const catalogSourceMessage = getCatalogSourceMessage(t, resultSource)
        if (catalogSourceMessage) {
          toast.info(catalogSourceMessage)
        }
        showCatalogWriteBackToast(t, catalog_write_back)
        if (resultSource?.catalog_origin === 'nexustok_embedded') {
          toast.info(
            t('Built-in repository contains {{count}} models.', {
              count:
                resultSource.source_model_count ||
                resultSource.embedded_model_count ||
                0,
            })
          )
        }
        queryClient.invalidateQueries({ queryKey: modelsQueryKeys.lists() })
        queryClient.invalidateQueries({ queryKey: vendorsQueryKeys.lists() })
        onOpenChange(false)
      } else {
        toast.error(response.message || 'Sync failed')
      }
    } catch (error: unknown) {
      toast.error((error as Error)?.message || 'Sync failed')
    } finally {
      setIsSyncing(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className='flex max-h-[90vh] w-full flex-col gap-4 p-4 sm:max-w-2xl sm:p-6'
        initialFocus={!isMobile}
      >
        <DialogHeader className='flex-shrink-0 text-start'>
          <DialogTitle>{t('Sync Source Models')}</DialogTitle>
          <DialogDescription>
            {t(
              'Manage synced model metadata, vendors, pricing, and fallback catalog.'
            )}
          </DialogDescription>
        </DialogHeader>

        <div className='flex min-h-0 flex-1 flex-col gap-6 overflow-y-auto'>
          <div className='space-y-3'>
            <div>
              <Label className='text-base'>{t('Select Sync Source')}</Label>
              <p className='text-muted-foreground text-sm'>
                {t('Choose where to fetch upstream metadata.')}
              </p>
            </div>
            <RadioGroup
              value={source}
              onValueChange={(value) => {
                const selected = SYNC_SOURCE_OPTIONS.find(
                  (option) => option.value === value
                )
                if (!selected || selected.disabled) return
                setSource(selected.value)
              }}
              className='grid gap-3 md:grid-cols-2'
            >
              {SYNC_SOURCE_OPTIONS.map((option) => {
                const isActive = source === option.value
                const isDisabled = option.disabled
                return (
                  <Label
                    key={option.value}
                    htmlFor={`sync-source-${option.value}`}
                    className={cn(
                      'flex-col items-start gap-0 rounded-lg border p-4 font-normal transition-all',
                      isActive && 'border-primary ring-primary ring-1',
                      isDisabled
                        ? 'cursor-not-allowed opacity-60'
                        : 'hover:border-primary/60 cursor-pointer'
                    )}
                  >
                    <div className='flex items-start gap-3'>
                      <RadioGroupItem
                        value={option.value}
                        id={`sync-source-${option.value}`}
                        disabled={isDisabled}
                      />
                      <div className='space-y-1'>
                        <div className='flex items-center gap-2'>
                          <span className='font-medium'>{option.label}</span>
                          {option.value === 'official' && (
                            <StatusBadge
                              label={t('Built-in')}
                              variant='neutral'
                              copyable={false}
                            />
                          )}
                          {option.value === 'models.dev' && (
                            <StatusBadge
                              label={t('Recommended')}
                              variant='neutral'
                              copyable={false}
                            />
                          )}
                        </div>
                        <p className='text-muted-foreground text-sm'>
                          {option.description}
                        </p>
                      </div>
                    </div>
                  </Label>
                )
              })}
            </RadioGroup>
          </div>

          <div className='space-y-2'>
            <Label className='text-base'>{t('Select Language')}</Label>
            <RadioGroup
              value={locale}
              onValueChange={(v) => setLocale(v as SyncLocale)}
              className='grid gap-3 sm:grid-cols-3'
            >
              {SYNC_LOCALE_OPTIONS.map((option) => (
                <div
                  key={option.value}
                  className='flex items-center gap-2 rounded-lg border p-3'
                >
                  <RadioGroupItem
                    value={option.value}
                    id={`locale-${option.value}`}
                  />
                  <Label
                    htmlFor={`locale-${option.value}`}
                    className='cursor-pointer font-normal'
                  >
                    {option.label}
                  </Label>
                </div>
              ))}
            </RadioGroup>
          </div>

          <div className='flex flex-col gap-3'>
            <div>
              <Label className='text-base'>{t('Pricing sync strategy')}</Label>
              <p className='text-muted-foreground text-sm'>
                {t(
                  'Manual pricing stays highest priority. Upstream providers are tried in the order below.'
                )}
              </p>
            </div>

            <div className='rounded-lg border p-4'>
              <div className='flex items-start justify-between gap-4'>
                <div className='flex min-w-0 items-start gap-3'>
                  <ShieldCheck className='text-muted-foreground mt-0.5 h-4 w-4 shrink-0' />
                  <div className='min-w-0'>
                    <div className='font-medium'>
                      {t('Apply upstream pricing')}
                    </div>
                    <p className='text-muted-foreground text-sm'>
                      {t(
                        'Write selected provider prices into the model page during sync.'
                      )}
                    </p>
                  </div>
                </div>
                <Switch
                  checked={canSyncPricing && syncPricing}
                  disabled={!canSyncPricing}
                  onCheckedChange={(checked) => setSyncPricing(!!checked)}
                />
              </div>

              <div className='mt-4 flex items-start justify-between gap-4 border-t pt-4'>
                <div className='min-w-0'>
                  <div className='font-medium'>
                    {t('Allow overwriting manual pricing')}
                  </div>
                  <p className='text-muted-foreground text-sm'>
                    {t(
                      'Keep disabled unless you intentionally want upstream prices to replace manually confirmed model prices.'
                    )}
                  </p>
                </div>
                <Switch
                  checked={overwriteManualPricing}
                  disabled={!canSyncPricing || !syncPricing}
                  onCheckedChange={(checked) =>
                    setOverwriteManualPricing(!!checked)
                  }
                />
              </div>

              <div className='mt-4 border-t pt-4'>
                <div className='mb-2 flex items-center justify-between gap-3'>
                  <Label htmlFor='provider-order'>
                    {t('Provider fallback order')}
                  </Label>
                  <Badge variant='outline'>
                    {t('{{count}} providers', { count: providerOrder.length })}
                  </Badge>
                </div>
                <div className='flex gap-2'>
                  <div className='text-muted-foreground flex w-6 shrink-0 justify-center pt-2'>
                    <GripVertical className='h-4 w-4' />
                  </div>
                  <Textarea
                    id='provider-order'
                    value={providerOrderText}
                    disabled={!canSyncPricing || !syncPricing}
                    onChange={(event) =>
                      setProviderOrderText(event.target.value)
                    }
                    placeholder={'openai\nanthropic\ngoogle\nazure'}
                    className='min-h-28 font-mono text-sm'
                  />
                </div>
                <p className='text-muted-foreground mt-2 text-sm'>
                  {t(
                    'One provider ID per line. If none match, the first valid models.dev provider price is used.'
                  )}
                </p>
              </div>
            </div>
          </div>

          <div className='bg-muted/50 rounded-lg border p-4'>
            <p className='text-muted-foreground text-sm'>
              {t(
                'The sync fetches missing models and vendors. Existing metadata records are updated only when you approve conflicts.'
              )}
            </p>
          </div>
        </div>

        <DialogFooter className='flex-shrink-0 gap-2 sm:justify-end'>
          <Button
            variant='outline'
            onClick={() => onOpenChange(false)}
            disabled={isSyncing}
          >
            {t('Cancel')}
          </Button>
          <Button
            onClick={handleSync}
            disabled={isSyncing || !canSyncUpstream}
            title={canSyncUpstream ? undefined : noPermissionMessage}
          >
            {isSyncing && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
            <RefreshCw className='mr-2 h-4 w-4' />
            {isSyncing ? t('Syncing...') : t('Sync Now')}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
