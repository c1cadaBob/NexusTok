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

export const UPSTREAM_ACCOUNT_SYNC_UNITS = [
  'month',
  'week',
  'day',
  'hour',
  'minute',
  'second',
] as const

export type UpstreamAccountSyncUnit =
  (typeof UPSTREAM_ACCOUNT_SYNC_UNITS)[number]

export type UpstreamAccountSyncSettingsInput = {
  enabled: boolean
  interval: number
  unit: string
  syncKeyModelsEnabled?: boolean
  keyModelSyncOverwriteManualEnabled?: boolean
}

export type UpstreamAccountSyncSettingsValue = {
  enabled: boolean
  interval: number
  unit: UpstreamAccountSyncUnit
  sync_key_models_enabled: boolean
  key_model_sync_overwrite_manual_enabled: boolean
}

export const UPSTREAM_ACCOUNT_SYNC_UNIT_LABEL_KEYS: Record<
  UpstreamAccountSyncUnit,
  string
> = {
  month: 'Months',
  week: 'Weeks',
  day: 'Days',
  hour: 'Hours',
  minute: 'Minutes',
  second: 'Seconds',
}

export function normalizeUpstreamAccountSyncUnit(
  value: string
): UpstreamAccountSyncUnit {
  const normalized = value.trim().toLowerCase()
  return (UPSTREAM_ACCOUNT_SYNC_UNITS as readonly string[]).includes(normalized)
    ? (normalized as UpstreamAccountSyncUnit)
    : 'hour'
}

export function normalizeUpstreamAccountSyncInterval(value: number): number {
  return Number.isInteger(value) && value >= 1 ? value : 1
}

export function buildUpstreamAccountSyncFormDefaults(
  defaults: UpstreamAccountSyncSettingsInput
): UpstreamAccountSyncSettingsValue {
  return {
    enabled: defaults.enabled,
    interval: normalizeUpstreamAccountSyncInterval(defaults.interval),
    unit: normalizeUpstreamAccountSyncUnit(defaults.unit),
    sync_key_models_enabled: defaults.syncKeyModelsEnabled ?? true,
    key_model_sync_overwrite_manual_enabled:
      defaults.keyModelSyncOverwriteManualEnabled ?? false,
  }
}

export function buildUpstreamAccountSyncPersistedDefaults(
  defaults: UpstreamAccountSyncSettingsInput
): UpstreamAccountSyncSettingsInput & {
  syncKeyModelsEnabled: boolean
  keyModelSyncOverwriteManualEnabled: boolean
} {
  return {
    enabled: defaults.enabled,
    interval: defaults.interval,
    unit: defaults.unit,
    syncKeyModelsEnabled: defaults.syncKeyModelsEnabled ?? true,
    keyModelSyncOverwriteManualEnabled:
      defaults.keyModelSyncOverwriteManualEnabled ?? false,
  }
}

export function formatUpstreamAccountSyncDescription(
  enabled: boolean,
  interval: number,
  unit: UpstreamAccountSyncUnit,
  t: (key: string, options?: Record<string, unknown>) => string
) {
  if (!enabled) {
    return t(
      'This setting is disabled; upstream account pools will not be synchronized automatically.'
    )
  }
  return t('Syncing every {{interval}} {{unit}}', {
    interval,
    unit: t(UPSTREAM_ACCOUNT_SYNC_UNIT_LABEL_KEYS[unit]),
  })
}
