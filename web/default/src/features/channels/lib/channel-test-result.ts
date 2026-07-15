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

export const CHANNEL_TEST_MODEL_PRICE_ERROR_CODE = 'model_price_error'

const FAILURE_SUMMARY_MAX_LENGTH = 96

export type ChannelTestFailureDisplay = {
  summary: string
  details?: string
}

function normalizeInlineError(errorText: string): string {
  return errorText.replaceAll(/\s+/g, ' ').trim()
}

function getFirstErrorLine(errorText: string): string | undefined {
  return errorText
    .split(/\r?\n/)
    .map((line) => line.trim())
    .find(Boolean)
}

function truncateFailureSummary(summary: string): string {
  if (summary.length <= FAILURE_SUMMARY_MAX_LENGTH) {
    return summary
  }

  return `${summary.slice(0, FAILURE_SUMMARY_MAX_LENGTH).trimEnd()}...`
}

export function getChannelTestFailureDisplay({
  errorText,
  fallbackSummary,
  isModelPriceError,
  modelPriceSummary,
}: {
  errorText?: string
  fallbackSummary: string
  isModelPriceError: boolean
  modelPriceSummary: string
}): ChannelTestFailureDisplay {
  const rawError = errorText?.trim()

  if (!rawError) {
    return { summary: fallbackSummary }
  }

  if (isModelPriceError) {
    if (rawError === modelPriceSummary) {
      return { summary: modelPriceSummary }
    }

    return { summary: modelPriceSummary, details: rawError }
  }

  const firstLine = getFirstErrorLine(rawError) ?? rawError
  const summary = truncateFailureSummary(normalizeInlineError(firstLine))
  const normalizedRawError = normalizeInlineError(rawError)

  if (normalizedRawError === summary) {
    return { summary }
  }

  return { summary, details: errorText }
}
