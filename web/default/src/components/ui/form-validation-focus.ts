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

export function getFormScopedSelector(
  formId: string,
  selector: string
): string {
  return `[data-form-root="${formId}"]${selector}`
}

export function hasFormErrors(errors: unknown): boolean {
  return (
    typeof errors === 'object' &&
    errors !== null &&
    Object.keys(errors).length > 0
  )
}

export function getFirstFormErrorTarget<
  T extends { compareDocumentPosition: (other: T) => number },
>(
  invalidControl: T | null,
  errorMessage: T | null,
  precedingFlag: number
): T | null {
  if (!invalidControl) return errorMessage
  if (!errorMessage) return invalidControl

  const position = invalidControl.compareDocumentPosition(errorMessage)
  return position & precedingFlag ? errorMessage : invalidControl
}
