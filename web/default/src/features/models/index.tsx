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
import { useCallback, useEffect, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { getRouteApi, useNavigate } from '@tanstack/react-router'
import { Plus } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { SectionPageLayout } from '@/components/layout'
import { listDeployments } from './api'
import { DeploymentAccessGuard } from './components/deployment-access-guard'
import { DeploymentsTable } from './components/deployments-table'
import { CreateDeploymentDrawer } from './components/dialogs/create-deployment-drawer'
import { ModelsDialogs } from './components/models-dialogs'
import { ModelsPrimaryButtons } from './components/models-primary-buttons'
import { ModelsProvider, useModels } from './components/models-provider'
import { ModelsTable } from './components/models-table'
import { useModelDeploymentSettings } from './hooks/use-model-deployment-settings'
import { useModelPermissions } from './hooks/use-model-permissions'
import { deploymentsQueryKeys } from './lib'
import {
  type ModelsSectionId,
  MODELS_DEFAULT_SECTION,
  MODELS_SECTION_IDS,
} from './section-registry'

const route = getRouteApi('/_authenticated/models/$section')

const SECTION_META: Record<
  ModelsSectionId,
  { titleKey: string; descriptionKey: string }
> = {
  metadata: {
    titleKey: 'Metadata',
    descriptionKey: 'Manage model metadata and configuration',
  },
  deployments: {
    titleKey: 'Deployments',
    descriptionKey: 'Manage model deployments',
  },
}

function ModelsContent() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { tabCategory, setTabCategory } = useModels()
  const permissions = useModelPermissions()
  const noPermissionMessage = t("You don't have necessary permission")
  const params = route.useParams()
  const activeSection = (params.section ??
    MODELS_DEFAULT_SECTION) as ModelsSectionId

  // 部署创建抽屉状态，独立于模型元数据弹窗上下文。
  const [createDeploymentOpen, setCreateDeploymentOpen] = useState(false)

  // 同步上下文中的当前分类，供仍依赖 models provider 的子组件使用。
  useEffect(() => {
    if (tabCategory !== activeSection) {
      setTabCategory(activeSection)
    }
  }, [activeSection, setTabCategory, tabCategory])

  const {
    loading: deploymentLoading,
    loadingPhase,
    isIoNetEnabled,
    connectionLoading,
    connectionOk,
    connectionError,
    testConnection,
    refresh: refreshDeploymentSettings,
  } = useModelDeploymentSettings()

  // 切换到部署页时刷新服务配置，避免使用旧的启用状态或连接状态。
  useEffect(() => {
    if (activeSection === 'deployments') {
      refreshDeploymentSettings()
    }
  }, [activeSection, refreshDeploymentSettings])

  // 连接检查期间预取部署列表；守卫通过后表格可以直接复用缓存数据。
  useEffect(() => {
    if (
      activeSection === 'deployments' &&
      isIoNetEnabled &&
      loadingPhase === 'connection'
    ) {
      const defaultParams = { p: 1, page_size: 10 }
      queryClient.prefetchQuery({
        queryKey: deploymentsQueryKeys.list(defaultParams),
        queryFn: () => listDeployments(defaultParams),
        staleTime: 30 * 1000, // 30 秒
      })
    }
  }, [activeSection, isIoNetEnabled, loadingPhase, queryClient])

  const handleSectionChange = useCallback(
    (section: string) => {
      void navigate({
        to: '/models/$section',
        params: { section: section as ModelsSectionId },
      })
    },
    [navigate]
  )

  const meta = SECTION_META[activeSection] ?? SECTION_META.metadata

  return (
    <>
      <SectionPageLayout>
        <SectionPageLayout.Title>{t(meta.titleKey)}</SectionPageLayout.Title>
        <SectionPageLayout.Description>
          {t(meta.descriptionKey)}
        </SectionPageLayout.Description>
        <SectionPageLayout.Actions>
          {activeSection === 'metadata' ? (
            <ModelsPrimaryButtons />
          ) : (
            <Button
              onClick={() => setCreateDeploymentOpen(true)}
              size='sm'
              disabled={!permissions.canWrite}
              title={permissions.canWrite ? undefined : noPermissionMessage}
            >
              <Plus data-icon='inline-start' />
              {t('Create deployment')}
            </Button>
          )}
        </SectionPageLayout.Actions>
        <SectionPageLayout.Content>
          <div className='space-y-4'>
            <Tabs value={activeSection} onValueChange={handleSectionChange}>
              <TabsList className='h-auto max-w-full flex-wrap justify-start'>
                {MODELS_SECTION_IDS.map((section) => (
                  <TabsTrigger key={section} value={section}>
                    {t(SECTION_META[section].titleKey)}
                  </TabsTrigger>
                ))}
              </TabsList>
            </Tabs>
            {activeSection === 'metadata' ? (
              <ModelsTable />
            ) : (
              <DeploymentAccessGuard
                loading={deploymentLoading}
                loadingPhase={loadingPhase}
                isEnabled={isIoNetEnabled}
                connectionLoading={connectionLoading}
                connectionOk={connectionOk}
                connectionError={connectionError}
                onRetry={testConnection}
              >
                <DeploymentsTable />
              </DeploymentAccessGuard>
            )}
          </div>
        </SectionPageLayout.Content>
      </SectionPageLayout>

      <ModelsDialogs />
      <CreateDeploymentDrawer
        open={createDeploymentOpen}
        onOpenChange={setCreateDeploymentOpen}
      />
    </>
  )
}

export function Models() {
  return (
    <ModelsProvider>
      <ModelsContent />
    </ModelsProvider>
  )
}
