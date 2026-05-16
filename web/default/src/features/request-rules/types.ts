import { z } from 'zod'

// ============================================================================
// 请求规则 Schema & Types
// ============================================================================

export const requestRuleSchema = z.object({
  id: z.number(),
  name: z.string().min(1),
  description: z.string().optional(),
  status: z.number(), // 1=启用, 0=禁用
  priority: z.number().default(0),
  relay_format: z.string().default(''), // 空=全部
  model_pattern: z.string().default(''),
  model_match_mode: z.number().default(0), // 0=精确,1=前缀,2=包含,3=后缀,4=通配符
  param_override: z.string().nullable().optional(),
  header_override: z.string().nullable().optional(),
  log_request: z.boolean().default(false),
  log_response: z.boolean().default(false),
  log_max_size: z.number().default(4096),
  created_time: z.number(),
  updated_time: z.number(),
})

export type RequestRule = z.infer<typeof requestRuleSchema>

// 创建/编辑表单的 schema（不包含 id 和时间字段）
export const requestRuleFormSchema = z.object({
  name: z.string().min(1, 'Name is required'),
  description: z.string().optional(),
  status: z.number().default(1),
  priority: z.number().default(0),
  relay_format: z.string().default(''),
  model_pattern: z.string().default(''),
  model_match_mode: z.number().default(0),
  param_override: z.string().nullable().optional(),
  header_override: z.string().nullable().optional(),
  log_request: z.boolean().default(false),
  log_response: z.boolean().default(false),
  log_max_size: z.number().default(4096),
})

export type RequestRuleFormValues = z.infer<typeof requestRuleFormSchema>

// ============================================================================
// 请求记录 Schema & Types
// ============================================================================

export const requestLogSchema = z.object({
  id: z.number(),
  request_rule_id: z.number(),
  request_id: z.string(),
  user_id: z.number(),
  token_id: z.number(),
  channel_id: z.number(),
  model_name: z.string(),
  relay_format: z.string(),
  request_body: z.string().optional(),
  response_body: z.string().optional(),
  status_code: z.number(),
  latency: z.number(),
  created_at: z.number(),
})

export type RequestLog = z.infer<typeof requestLogSchema>

// ============================================================================
// API Response Types
// ============================================================================

export interface GetRequestRulesResponse {
  success: boolean
  message?: string
  data?: {
    items: RequestRule[]
    total: number
    page: number
    page_size: number
  }
}

export interface GetRequestLogsResponse {
  success: boolean
  message?: string
  data?: {
    items: RequestLog[]
    total: number
    page: number
    page_size: number
  }
}

// ============================================================================
// 常量选项
// ============================================================================

// RelayFormat 下拉选项（UI 使用 'all' 代替空字符串，提交时转换回空字符串）
export const RELAY_FORMAT_ALL_VALUE = 'all'

export const RELAY_FORMAT_OPTIONS = [
  { value: 'all', labelKey: 'All' },
  { value: 'openai', labelKey: 'OpenAI' },
  { value: 'claude', labelKey: 'Claude' },
  { value: 'gemini', labelKey: 'Gemini' },
  { value: 'openai_responses', labelKey: 'OpenAI Responses' },
  { value: 'openai_image', labelKey: 'OpenAI Image' },
  { value: 'embedding', labelKey: 'Embedding' },
  { value: 'rerank', labelKey: 'Rerank' },
] as const

// ModelMatchMode 下拉选项
export const MODEL_MATCH_MODE_OPTIONS = [
  { value: 0, labelKey: 'Exact Match' },
  { value: 1, labelKey: 'Prefix Match' },
  { value: 2, labelKey: 'Contains Match' },
  { value: 3, labelKey: 'Suffix Match' },
  { value: 4, labelKey: 'Wildcard Match' },
] as const
