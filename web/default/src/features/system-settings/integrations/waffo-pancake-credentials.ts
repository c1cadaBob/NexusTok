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

export interface WaffoPancakeCredentialValues {
  WaffoPancakeMerchantID: string
  WaffoPancakePrivateKey: string
}

export interface WaffoPancakeAdminCredentialPayload {
  merchant_id: string
  private_key: string
}

export interface WaffoPancakeAdminCredentialState {
  payload: WaffoPancakeAdminCredentialPayload
  edited: boolean
  ready: boolean
  missingMerchantID: boolean
  missingPrivateKey: boolean
}

// 管理端辅助接口遵循后端约定：MerchantID 与 PrivateKey 同时为空表示复用已保存凭据；
// 只传其中一个字段会被视为不完整临时凭据。因此前端需要先判断管理员是否正在编辑凭据，
// 再决定发送空 payload 还是发送成对的临时凭据，避免私钥不回显时误发“旧 MerchantID + 空 Key”。
export function resolveWaffoPancakeAdminCredentials(
  defaults: WaffoPancakeCredentialValues,
  values: WaffoPancakeCredentialValues
): WaffoPancakeAdminCredentialState {
  const savedMerchantID = defaults.WaffoPancakeMerchantID.trim()
  const merchantID = values.WaffoPancakeMerchantID.trim()
  const privateKey = values.WaffoPancakePrivateKey.trim()
  const edited = merchantID !== savedMerchantID || privateKey.length > 0
  const hasSavedCredentials = savedMerchantID.length > 0

  if (!edited) {
    return {
      payload: { merchant_id: '', private_key: '' },
      edited,
      ready: hasSavedCredentials,
      missingMerchantID: !hasSavedCredentials,
      missingPrivateKey: !hasSavedCredentials,
    }
  }

  return {
    payload: { merchant_id: merchantID, private_key: privateKey },
    edited,
    ready: merchantID.length > 0 && privateKey.length > 0,
    missingMerchantID: merchantID.length === 0,
    missingPrivateKey: privateKey.length === 0,
  }
}
