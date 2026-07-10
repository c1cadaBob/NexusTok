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

export type FlatGrokSettings = {
  'grok.violation_deduction_enabled': boolean
  'grok.violation_deduction_amount': number
}

export type GrokFormInput = {
  grok: {
    violation_deduction_enabled: boolean
    violation_deduction_amount: unknown
  }
}

export type GrokFormValues = {
  grok: {
    violation_deduction_enabled: boolean
    violation_deduction_amount: number
  }
}

export function buildGrokFormDefaults(
  defaults: FlatGrokSettings
): GrokFormInput {
  return {
    grok: {
      violation_deduction_enabled:
        defaults['grok.violation_deduction_enabled'],
      violation_deduction_amount: defaults['grok.violation_deduction_amount'],
    },
  }
}

export function normalizeGrokFormValues(
  values: GrokFormValues
): FlatGrokSettings {
  return {
    'grok.violation_deduction_enabled':
      values.grok.violation_deduction_enabled,
    'grok.violation_deduction_amount':
      values.grok.violation_deduction_amount,
  }
}

export function getChangedGrokSettingKeys(
  values: FlatGrokSettings,
  baseline: FlatGrokSettings
): Array<keyof FlatGrokSettings> {
  return (Object.keys(values) as Array<keyof FlatGrokSettings>).filter(
    (key) => values[key] !== baseline[key]
  )
}
