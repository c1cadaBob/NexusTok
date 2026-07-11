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
import { useMemo, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import {
  EyeIcon,
  RefreshIcon,
  RotateLeft01Icon,
  SaveIcon,
  SecurityCheckIcon,
} from '@hugeicons/core-free-icons'
import { HugeiconsIcon } from '@hugeicons/react'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import {
  ADMIN_ROLE_KEY,
  EMPTY_PERMISSION_CATALOG,
  type AdminPermissionMatrix,
  type PermissionCatalog,
  type PermissionResourceDefinition,
} from '@/lib/admin-permissions'
import { ROLE } from '@/lib/roles'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Separator } from '@/components/ui/separator'
import { Skeleton } from '@/components/ui/skeleton'
import { Spinner } from '@/components/ui/spinner'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import {
  getAuthzRoles,
  getPermissionCatalog,
  updateAuthzRolePolicies,
} from '../api'
import { SettingsCard } from '../components/settings-card'
import { SettingsSection } from '../components/settings-section'
import { useSystemSettingPermissions } from '../hooks/use-system-setting-permissions'
import type { AuthzRolePolicy, AuthzRolePolicyUpdateResult } from '../types'
import {
  countEnabledActions,
  countResourceEnabledActions,
  countTotalActions,
  diffPermissionMatrix,
  normalizeRolePolicyGrants,
  permissionMatrixSignature,
  replaceResourceGrants,
} from './role-policy-utils'

type PreviewState = {
  roleKey: string
  signature: string
  result: AuthzRolePolicyUpdateResult
}

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback
}

function getRoleBadges(role: AuthzRolePolicy) {
  const badges = []
  if (role.built_in) badges.push('Built-in')
  if (role.superuser) badges.push('Superuser')
  if (role.runtime_managed) {
    badges.push('Runtime-managed')
  } else {
    badges.push('Template only')
  }
  if (!role.enabled) badges.push('Disabled')
  return badges
}

function getRoleInitialGrants(
  role: AuthzRolePolicy | undefined,
  catalog: PermissionCatalog
) {
  return normalizeRolePolicyGrants(role?.grants, catalog)
}

export function RolePolicySection() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const currentUser = useAuthStore((state) => state.auth.user)
  const permissions = useSystemSettingPermissions()
  const isRootUser = (currentUser?.role ?? 0) >= ROLE.SUPER_ADMIN
  const canLoadPolicies = isRootUser && permissions.canViewSecret
  const canWritePolicies = isRootUser && permissions.canSensitiveWrite
  const [selectedRoleKey, setSelectedRoleKey] = useState('')
  const [drafts, setDrafts] = useState<Record<string, AdminPermissionMatrix>>(
    {}
  )
  const [preview, setPreview] = useState<PreviewState | null>(null)
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [isPreviewing, setIsPreviewing] = useState(false)
  const [isApplying, setIsApplying] = useState(false)

  const {
    data: catalog = EMPTY_PERMISSION_CATALOG,
    isLoading: isCatalogLoading,
    isError: isCatalogError,
    error: catalogError,
  } = useQuery({
    queryKey: ['authz-permission-catalog'],
    queryFn: getPermissionCatalog,
    enabled: canLoadPolicies,
    staleTime: 5 * 60 * 1000,
  })

  const {
    data: rolesData,
    isLoading: isRolesLoading,
    isError: isRolesError,
    error: rolesError,
  } = useQuery({
    queryKey: ['authz-roles'],
    queryFn: getAuthzRoles,
    enabled: canLoadPolicies,
    staleTime: 60 * 1000,
  })

  const roles = rolesData?.roles ?? []
  const selectedRole =
    roles.find((role) => role.key === selectedRoleKey) ?? roles[0]
  const catalogReady = catalog.resources.length > 0
  const initialGrants = useMemo(
    () => getRoleInitialGrants(selectedRole, catalog),
    [selectedRole, catalog]
  )
  const currentGrants = useMemo(() => {
    if (!selectedRole) return initialGrants
    return normalizeRolePolicyGrants(
      drafts[selectedRole.key] ?? initialGrants,
      catalog
    )
  }, [catalog, drafts, initialGrants, selectedRole])
  const totalActions = useMemo(() => countTotalActions(catalog), [catalog])
  const enabledActions = useMemo(
    () => countEnabledActions(currentGrants, catalog),
    [catalog, currentGrants]
  )
  const initialSignature = useMemo(
    () => permissionMatrixSignature(initialGrants, catalog),
    [catalog, initialGrants]
  )
  const currentSignature = useMemo(
    () => permissionMatrixSignature(currentGrants, catalog),
    [catalog, currentGrants]
  )
  const diff = useMemo(
    () => diffPermissionMatrix(initialGrants, currentGrants, catalog),
    [catalog, currentGrants, initialGrants]
  )
  const hasChanges = currentSignature !== initialSignature
  const isSelectedRoleEditable = Boolean(
    selectedRole?.enabled && !selectedRole.superuser && canWritePolicies
  )
  const adminRoleWouldBeEmpty =
    selectedRole?.key === ADMIN_ROLE_KEY && enabledActions === 0
  const previewIsCurrent =
    preview?.roleKey === selectedRole?.key &&
    preview?.signature === currentSignature
  const canPreview =
    Boolean(selectedRole) &&
    catalogReady &&
    isSelectedRoleEditable &&
    hasChanges &&
    !adminRoleWouldBeEmpty
  const canApply = canPreview && previewIsCurrent
  const isLoading = isCatalogLoading || isRolesLoading
  const isError = isCatalogError || isRolesError

  const updateDraft = (next: AdminPermissionMatrix) => {
    if (!selectedRole) return
    setDrafts((previous) => ({
      ...previous,
      [selectedRole.key]: normalizeRolePolicyGrants(next, catalog),
    }))
    setPreview(null)
  }

  const handleToggleAction = (
    resourceKey: string,
    actionKey: string,
    checked: boolean
  ) => {
    updateDraft({
      ...currentGrants,
      [resourceKey]: {
        ...currentGrants[resourceKey],
        [actionKey]: checked,
      },
    })
  }

  const handleToggleResource = (resourceKey: string, enabled: boolean) => {
    updateDraft(
      replaceResourceGrants(currentGrants, catalog, resourceKey, enabled)
    )
  }

  const handleReset = () => {
    if (!selectedRole) return
    setDrafts((previous) => {
      const next = { ...previous }
      delete next[selectedRole.key]
      return next
    })
    setPreview(null)
  }

  const handleRefresh = async () => {
    setDrafts({})
    setPreview(null)
    await Promise.all([
      queryClient.invalidateQueries({ queryKey: ['authz-permission-catalog'] }),
      queryClient.invalidateQueries({ queryKey: ['authz-roles'] }),
    ])
  }

  const handlePreview = async () => {
    if (!selectedRole || !canPreview || isPreviewing) return

    setIsPreviewing(true)
    try {
      const result = await updateAuthzRolePolicies(selectedRole.key, {
        dry_run: true,
        grants: currentGrants,
      })
      setPreview({
        roleKey: selectedRole.key,
        signature: currentSignature,
        result,
      })
      toast.success(t('Role policy preview is ready'))
    } catch (error) {
      toast.error(
        getErrorMessage(error, t('Failed to preview role policy changes'))
      )
    } finally {
      setIsPreviewing(false)
    }
  }

  const handleApply = async () => {
    if (!selectedRole || !canApply || isApplying) return

    setIsApplying(true)
    try {
      await updateAuthzRolePolicies(selectedRole.key, {
        dry_run: false,
        grants: currentGrants,
      })
      setConfirmOpen(false)
      setPreview(null)
      setDrafts((previous) => {
        const next = { ...previous }
        delete next[selectedRole.key]
        return next
      })
      await queryClient.invalidateQueries({ queryKey: ['authz-roles'] })
      toast.success(t('Role policies saved'))
    } catch (error) {
      toast.error(
        getErrorMessage(error, t('Failed to save role policy changes'))
      )
    } finally {
      setIsApplying(false)
    }
  }

  return (
    <SettingsSection
      title={t('Role Policies')}
      description={t('Manage administrator role permission baselines.')}
    >
      {!canLoadPolicies ? (
        <Alert variant='destructive'>
          <AlertTitle>{t('Root permission required')}</AlertTitle>
          <AlertDescription>
            {t('Only root users can view and edit role policies.')}
          </AlertDescription>
        </Alert>
      ) : (
        <SettingsCard
          title={t('Role baseline editor')}
          description={t(
            'Changes affect administrators that use the selected role baseline.'
          )}
        >
          <div className='flex flex-col gap-4'>
            <div className='flex flex-col gap-3 lg:flex-row lg:items-end lg:justify-between'>
              <div className='flex min-w-0 flex-1 flex-col gap-2'>
                <label
                  className='text-sm font-medium'
                  htmlFor='authz-role-policy-selector'
                >
                  {t('Role')}
                </label>
                <Select
                  value={selectedRole?.key ?? ''}
                  onValueChange={(value) => {
                    setSelectedRoleKey(value ?? '')
                    setPreview(null)
                  }}
                  disabled={isLoading || roles.length === 0}
                >
                  <SelectTrigger
                    id='authz-role-policy-selector'
                    className='w-full min-w-0 lg:w-[280px]'
                  >
                    <SelectValue placeholder={t('Select a role')} />
                  </SelectTrigger>
                  <SelectContent alignItemWithTrigger={false}>
                    <SelectGroup>
                      {roles.map((role) => (
                        <SelectItem key={role.key} value={role.key}>
                          {role.name || role.key}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  </SelectContent>
                </Select>
              </div>

              <div className='flex flex-wrap items-center gap-2'>
                <Button
                  variant='outline'
                  type='button'
                  onClick={() => void handleRefresh()}
                  disabled={isLoading || isPreviewing || isApplying}
                >
                  <HugeiconsIcon
                    icon={RefreshIcon}
                    strokeWidth={2}
                    data-icon='inline-start'
                  />
                  {t('Refresh')}
                </Button>
                <Button
                  variant='outline'
                  type='button'
                  onClick={handleReset}
                  disabled={!hasChanges || isPreviewing || isApplying}
                >
                  <HugeiconsIcon
                    icon={RotateLeft01Icon}
                    strokeWidth={2}
                    data-icon='inline-start'
                  />
                  {t('Reset changes')}
                </Button>
                <Button
                  variant='secondary'
                  type='button'
                  onClick={() => void handlePreview()}
                  disabled={!canPreview || isPreviewing || isApplying}
                  title={
                    adminRoleWouldBeEmpty
                      ? t('Admin role must keep at least one permission')
                      : undefined
                  }
                >
                  {isPreviewing ? (
                    <Spinner data-icon='inline-start' />
                  ) : (
                    <HugeiconsIcon
                      icon={EyeIcon}
                      strokeWidth={2}
                      data-icon='inline-start'
                    />
                  )}
                  {isPreviewing ? t('Previewing...') : t('Preview changes')}
                </Button>
                <Button
                  type='button'
                  onClick={() => setConfirmOpen(true)}
                  disabled={!canApply || isPreviewing || isApplying}
                >
                  <HugeiconsIcon
                    icon={SaveIcon}
                    strokeWidth={2}
                    data-icon='inline-start'
                  />
                  {t('Save changes')}
                </Button>
              </div>
            </div>

            {isLoading && <RolePolicySkeleton />}

            {isError && (
              <Alert variant='destructive'>
                <AlertTitle>{t('Role policies unavailable')}</AlertTitle>
                <AlertDescription>
                  {getErrorMessage(
                    rolesError ?? catalogError,
                    t('Failed to load role policies.')
                  )}
                </AlertDescription>
              </Alert>
            )}

            {!isLoading && !isError && selectedRole && catalogReady && (
              <>
                <RoleSummary
                  role={selectedRole}
                  enabledActions={enabledActions}
                  totalActions={totalActions}
                />

                {selectedRole.superuser && (
                  <Alert>
                    <AlertTitle>{t('Superuser role is read-only')}</AlertTitle>
                    <AlertDescription>
                      {t(
                        'Root keeps full access by definition and cannot be edited here.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}

                {!selectedRole.enabled && (
                  <Alert variant='destructive'>
                    <AlertTitle>{t('Role is disabled')}</AlertTitle>
                    <AlertDescription>
                      {t('Disabled roles cannot be edited.')}
                    </AlertDescription>
                  </Alert>
                )}

                {adminRoleWouldBeEmpty && (
                  <Alert variant='destructive'>
                    <AlertTitle>
                      {t('Admin role must keep at least one permission')}
                    </AlertTitle>
                    <AlertDescription>
                      {t(
                        'Saving an empty Admin baseline is blocked to avoid an unsafe fallback state.'
                      )}
                    </AlertDescription>
                  </Alert>
                )}

                <RolePolicyStats
                  enabledActions={enabledActions}
                  totalActions={totalActions}
                  policyCount={selectedRole.policy_count}
                  diffChanged={diff.changed}
                  diffEnabled={diff.enabled}
                  diffDisabled={diff.disabled}
                />

                {previewIsCurrent && preview && (
                  <PreviewSummary result={preview.result} />
                )}

                {preview && !previewIsCurrent && (
                  <Alert>
                    <AlertTitle>{t('Preview is out of date')}</AlertTitle>
                    <AlertDescription>
                      {t('Preview the latest changes before saving.')}
                    </AlertDescription>
                  </Alert>
                )}

                <RolePermissionTable
                  catalog={catalog}
                  grants={currentGrants}
                  editable={isSelectedRoleEditable}
                  onToggleAction={handleToggleAction}
                  onToggleResource={handleToggleResource}
                />
              </>
            )}

            {!isLoading && !isError && !selectedRole && (
              <Alert>
                <AlertTitle>{t('No roles available')}</AlertTitle>
                <AlertDescription>
                  {t('No authorization roles were returned by the server.')}
                </AlertDescription>
              </Alert>
            )}
          </div>
        </SettingsCard>
      )}

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogMedia>
              <HugeiconsIcon icon={SecurityCheckIcon} strokeWidth={2} />
            </AlertDialogMedia>
            <AlertDialogTitle>
              {t('Save role policy changes?')}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t(
                'This will replace the selected role baseline and reload authorization policies immediately.'
              )}
            </AlertDialogDescription>
          </AlertDialogHeader>
          {preview?.result && (
            <div className='grid grid-cols-3 gap-2 text-sm'>
              <PolicyCount
                label={t('Created policies')}
                value={preview.result.created_policy_count}
              />
              <PolicyCount
                label={t('Deleted policies')}
                value={preview.result.deleted_policy_count}
              />
              <PolicyCount
                label={t('Unchanged policies')}
                value={preview.result.unchanged_policy_count}
              />
            </div>
          )}
          <AlertDialogFooter>
            <AlertDialogCancel disabled={isApplying}>
              {t('Cancel')}
            </AlertDialogCancel>
            <AlertDialogAction
              onClick={() => void handleApply()}
              disabled={isApplying}
            >
              {isApplying ? t('Saving...') : t('Apply changes')}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </SettingsSection>
  )
}

function RolePolicySkeleton() {
  return (
    <div className='flex flex-col gap-3'>
      <Skeleton className='h-16 w-full' />
      <Skeleton className='h-10 w-full' />
      <Skeleton className='h-52 w-full' />
    </div>
  )
}

function RoleSummary({
  role,
  enabledActions,
  totalActions,
}: {
  role: AuthzRolePolicy
  enabledActions: number
  totalActions: number
}) {
  const { t } = useTranslation()
  return (
    <div className='rounded-md border p-3'>
      <div className='flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between'>
        <div className='flex min-w-0 flex-col gap-1'>
          <div className='flex flex-wrap items-center gap-2'>
            <h4 className='text-sm font-semibold'>{role.name || role.key}</h4>
            <span className='text-muted-foreground text-xs'>/{role.key}</span>
          </div>
          {role.description && (
            <p className='text-muted-foreground text-sm leading-5'>
              {role.description}
            </p>
          )}
        </div>
        <div className='flex shrink-0 flex-wrap gap-1.5'>
          {getRoleBadges(role).map((badge) => (
            <Badge key={badge} variant='secondary'>
              {t(badge)}
            </Badge>
          ))}
        </div>
      </div>
      <Separator className='my-3' />
      <div className='text-muted-foreground text-xs'>
        {t('Enabled actions: {{enabled}} / {{total}}', {
          enabled: enabledActions,
          total: totalActions,
        })}
      </div>
    </div>
  )
}

function RolePolicyStats({
  enabledActions,
  totalActions,
  policyCount,
  diffChanged,
  diffEnabled,
  diffDisabled,
}: {
  enabledActions: number
  totalActions: number
  policyCount: number
  diffChanged: number
  diffEnabled: number
  diffDisabled: number
}) {
  const { t } = useTranslation()
  return (
    <div className='grid gap-2 sm:grid-cols-2 xl:grid-cols-4'>
      <PolicyCount
        label={t('Enabled')}
        value={`${enabledActions}/${totalActions}`}
      />
      <PolicyCount label={t('Current policies')} value={policyCount} />
      <PolicyCount label={t('Changed')} value={diffChanged} />
      <PolicyCount
        label={t('Enable / disable')}
        value={`${diffEnabled}/${diffDisabled}`}
      />
    </div>
  )
}

function PreviewSummary({ result }: { result: AuthzRolePolicyUpdateResult }) {
  const { t } = useTranslation()
  return (
    <Alert>
      <AlertTitle>{t('Preview ready')}</AlertTitle>
      <AlertDescription>
        <div className='flex flex-wrap gap-2 pt-1'>
          <Badge variant='secondary'>
            {t('Created policies')}: {result.created_policy_count}
          </Badge>
          <Badge variant='secondary'>
            {t('Deleted policies')}: {result.deleted_policy_count}
          </Badge>
          <Badge variant='secondary'>
            {t('Unchanged policies')}: {result.unchanged_policy_count}
          </Badge>
        </div>
      </AlertDescription>
    </Alert>
  )
}

function PolicyCount({
  label,
  value,
}: {
  label: string
  value: number | string
}) {
  return (
    <div className='rounded-md border px-3 py-2'>
      <div className='text-muted-foreground text-xs'>{label}</div>
      <div className='text-sm font-semibold tabular-nums'>{value}</div>
    </div>
  )
}

function RolePermissionTable({
  catalog,
  grants,
  editable,
  onToggleAction,
  onToggleResource,
}: {
  catalog: PermissionCatalog
  grants: AdminPermissionMatrix
  editable: boolean
  onToggleAction: (
    resourceKey: string,
    actionKey: string,
    checked: boolean
  ) => void
  onToggleResource: (resourceKey: string, enabled: boolean) => void
}) {
  const { t } = useTranslation()

  return (
    <Table>
      <TableHeader>
        <TableRow>
          <TableHead className='min-w-56'>{t('Resource')}</TableHead>
          <TableHead className='min-w-[520px]'>{t('Actions')}</TableHead>
          <TableHead className='w-28 text-right'>{t('Enabled')}</TableHead>
          <TableHead className='w-28 text-right'>{t('Row action')}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {catalog.resources.map((resource) => (
          <RolePermissionRow
            key={resource.resource}
            resource={resource}
            catalog={catalog}
            grants={grants}
            editable={editable}
            onToggleAction={onToggleAction}
            onToggleResource={onToggleResource}
          />
        ))}
      </TableBody>
    </Table>
  )
}

function RolePermissionRow({
  resource,
  catalog,
  grants,
  editable,
  onToggleAction,
  onToggleResource,
}: {
  resource: PermissionResourceDefinition
  catalog: PermissionCatalog
  grants: AdminPermissionMatrix
  editable: boolean
  onToggleAction: (
    resourceKey: string,
    actionKey: string,
    checked: boolean
  ) => void
  onToggleResource: (resourceKey: string, enabled: boolean) => void
}) {
  const { t } = useTranslation()
  const selectedCount = countResourceEnabledActions(
    grants,
    catalog,
    resource.resource
  )
  const allSelected = selectedCount === resource.actions.length

  return (
    <TableRow>
      <TableCell className='min-w-56 whitespace-normal'>
        <div className='flex min-w-0 flex-col gap-1'>
          <span className='font-medium'>{t(resource.label_key)}</span>
          <span className='text-muted-foreground text-xs'>
            {resource.resource}
          </span>
        </div>
      </TableCell>
      <TableCell className='min-w-[520px] whitespace-normal'>
        <div className='grid min-w-[500px] gap-2 md:grid-cols-2 xl:grid-cols-3'>
          {resource.actions.map((action) => {
            const checkboxId = `role-policy-${resource.resource}-${action.action}`
            const checked = grants[resource.resource]?.[action.action] === true
            return (
              <label
                key={action.action}
                htmlFor={checkboxId}
                className='flex min-h-16 items-start gap-2 rounded-md border p-2'
              >
                <Checkbox
                  id={checkboxId}
                  checked={checked}
                  disabled={!editable}
                  onCheckedChange={(value) =>
                    onToggleAction(
                      resource.resource,
                      action.action,
                      value === true
                    )
                  }
                  aria-label={t('Toggle {{action}} for {{resource}}', {
                    action: t(action.label_key),
                    resource: t(resource.label_key),
                  })}
                />
                <span className='flex min-w-0 flex-col gap-1'>
                  <span className='text-sm font-medium'>
                    {t(action.label_key)}
                  </span>
                  <span className='text-muted-foreground text-xs leading-4 whitespace-normal'>
                    {t(action.description_key)}
                  </span>
                </span>
              </label>
            )
          })}
        </div>
      </TableCell>
      <TableCell className='text-right tabular-nums'>
        {selectedCount}/{resource.actions.length}
      </TableCell>
      <TableCell className='text-right'>
        <Button
          type='button'
          variant='ghost'
          size='xs'
          disabled={!editable}
          onClick={() => onToggleResource(resource.resource, !allSelected)}
        >
          {allSelected ? t('Clear row') : t('Select row')}
        </Button>
      </TableCell>
    </TableRow>
  )
}
