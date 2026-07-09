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
import type { AxiosRequestConfig } from 'axios'
import { api } from '@/lib/api'
import type {
  ConfirmPaymentComplianceResponse,
  CreateWaffoPancakePairRequest,
  CreateWaffoPancakePairResponse,
  CreateLogCleanupTaskResponse,
  FetchUpstreamRatiosRequest,
  SaveWaffoPancakeConfigRequest,
  SaveWaffoPancakeConfigResponse,
  SystemOptionsResponse,
  UpdateOptionRequest,
  UpdateOptionResponse,
  UpstreamChannelsResponse,
  UpstreamRatiosResponse,
  WaffoPancakeCatalogRequest,
  WaffoPancakeCatalogResponse,
} from './types'

interface ExtendedApiConfig extends AxiosRequestConfig {
  skipBusinessError?: boolean
}

export async function getSystemOptions() {
  const res = await api.get<SystemOptionsResponse>('/api/option/')
  return res.data
}

export async function updateSystemOption(request: UpdateOptionRequest) {
  const res = await api.put<UpdateOptionResponse>('/api/option/', request)
  return res.data
}

export async function saveWaffoPancakeConfig(
  request: SaveWaffoPancakeConfigRequest
) {
  const res = await api.post<SaveWaffoPancakeConfigResponse>(
    '/api/option/waffo-pancake/save',
    request
  )
  return res.data
}

export async function listWaffoPancakeCatalog(
  request?: WaffoPancakeCatalogRequest
) {
  const config: ExtendedApiConfig = { skipBusinessError: true }
  if (request) {
    const res = await api.post<WaffoPancakeCatalogResponse>(
      '/api/option/waffo-pancake/catalog',
      request,
      config
    )
    return res.data
  }
  const res = await api.get<WaffoPancakeCatalogResponse>(
    '/api/option/waffo-pancake/catalog',
    config
  )
  return res.data
}

export async function createWaffoPancakePair(
  request: CreateWaffoPancakePairRequest
) {
  const config: ExtendedApiConfig = { skipBusinessError: true }
  const res = await api.post<CreateWaffoPancakePairResponse>(
    '/api/option/waffo-pancake/pair',
    request,
    config
  )
  return res.data
}

export async function confirmPaymentCompliance() {
  const res = await api.post<ConfirmPaymentComplianceResponse>(
    '/api/option/payment_compliance',
    { confirmed: true }
  )
  return res.data
}

export async function createLogCleanupTask(targetTimestamp: number) {
  const res = await api.post<CreateLogCleanupTaskResponse>(
    '/api/system-task/log-cleanup',
    null,
    {
      params: { target_timestamp: targetTimestamp },
    }
  )
  return res.data
}

export async function resetModelRatios() {
  const res = await api.post<UpdateOptionResponse>(
    '/api/option/rest_model_ratio'
  )
  return res.data
}

export async function getUpstreamChannels() {
  const res = await api.get<UpstreamChannelsResponse>(
    '/api/ratio_sync/channels'
  )
  return res.data
}

export async function fetchUpstreamRatios(request: FetchUpstreamRatiosRequest) {
  const res = await api.post<UpstreamRatiosResponse>(
    '/api/ratio_sync/fetch',
    request
  )
  return res.data
}
