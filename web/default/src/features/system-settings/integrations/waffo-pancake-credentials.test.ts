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
import assert from 'node:assert/strict'
import { describe, test } from 'node:test'
import { resolveWaffoPancakeAdminCredentials } from './waffo-pancake-credentials'

const savedDefaults = {
  WaffoPancakeMerchantID: 'MER_saved',
  WaffoPancakePrivateKey: '',
}

describe('Waffo Pancake 管理端凭据解析', () => {
  test('未编辑已保存凭据时发送空 payload 让后端复用已保存私钥', () => {
    assert.deepEqual(
      resolveWaffoPancakeAdminCredentials(savedDefaults, {
        WaffoPancakeMerchantID: 'MER_saved',
        WaffoPancakePrivateKey: '',
      }),
      {
        payload: { merchant_id: '', private_key: '' },
        edited: false,
        ready: true,
        missingMerchantID: false,
        missingPrivateKey: false,
      }
    )
  })

  test('管理员输入全新凭据时发送成对临时凭据', () => {
    assert.deepEqual(
      resolveWaffoPancakeAdminCredentials(savedDefaults, {
        WaffoPancakeMerchantID: ' MER_new ',
        WaffoPancakePrivateKey: ' KEY_new ',
      }),
      {
        payload: { merchant_id: 'MER_new', private_key: 'KEY_new' },
        edited: true,
        ready: true,
        missingMerchantID: false,
        missingPrivateKey: false,
      }
    )
  })

  test('只改 Merchant ID 时标记缺少私钥，避免混合态请求', () => {
    assert.deepEqual(
      resolveWaffoPancakeAdminCredentials(savedDefaults, {
        WaffoPancakeMerchantID: 'MER_new',
        WaffoPancakePrivateKey: '',
      }),
      {
        payload: { merchant_id: 'MER_new', private_key: '' },
        edited: true,
        ready: false,
        missingMerchantID: false,
        missingPrivateKey: true,
      }
    )
  })

  test('只填 Private Key 时标记缺少 Merchant ID', () => {
    assert.deepEqual(
      resolveWaffoPancakeAdminCredentials(
        {
          WaffoPancakeMerchantID: '',
          WaffoPancakePrivateKey: '',
        },
        {
          WaffoPancakeMerchantID: '',
          WaffoPancakePrivateKey: 'KEY_new',
        }
      ),
      {
        payload: { merchant_id: '', private_key: 'KEY_new' },
        edited: true,
        ready: false,
        missingMerchantID: true,
        missingPrivateKey: false,
      }
    )
  })
})
