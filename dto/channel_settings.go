// Package dto - channel_settings.go
// 该文件定义了渠道设置相关的数据传输对象
//
// 渠道设置分为两类：
// 1. ChannelSettings: 渠道基本设置（代理、格式化、系统提示等）
// 2. ChannelOtherSettings: 渠道其他设置（Azure 版本、Vertex Key 类型等）
package dto

// ChannelSettings 渠道基本设置
// 存储在渠道的 setting 字段中（JSON 格式）
type ChannelSettings struct {
	ForceFormat            bool   `json:"force_format,omitempty"`              // 是否强制格式化请求
	ThinkingToContent      bool   `json:"thinking_to_content,omitempty"`       // 是否将 Thinking 内容转换为 Content
	Proxy                  string `json:"proxy"`                               // 代理地址（如 http://proxy:8080）
	PassThroughBodyEnabled bool   `json:"pass_through_body_enabled,omitempty"` // 是否启用请求体透传
	SystemPrompt           string `json:"system_prompt,omitempty"`             // 系统提示词
	SystemPromptOverride   bool   `json:"system_prompt_override,omitempty"`    // 是否覆盖系统提示词
}

// VertexKeyType Vertex AI Key 类型
type VertexKeyType string

const (
	VertexKeyTypeJSON   VertexKeyType = "json"    // JSON 格式的服务账号密钥
	VertexKeyTypeAPIKey VertexKeyType = "api_key" // API Key
)

// AwsKeyType AWS Key 类型
type AwsKeyType string

const (
	AwsKeyTypeAKSK   AwsKeyType = "ak_sk"   // AK/SK 认证（默认）
	AwsKeyTypeApiKey AwsKeyType = "api_key" // API Key 认证
)

// ChannelOtherSettings 渠道其他设置
// 存储在渠道的 settings 字段中（JSON 格式）
// 包含不需要索引检索的配置信息
type ChannelOtherSettings struct {
	AzureResponsesVersion                 string                `json:"azure_responses_version,omitempty"`                    // Azure Responses API 版本
	VertexKeyType                         VertexKeyType         `json:"vertex_key_type,omitempty"`                            // Vertex AI Key 类型
	OpenRouterEnterprise                  *bool                 `json:"openrouter_enterprise,omitempty"`                      // 是否使用 OpenRouter 企业版
	ClaudeBetaQuery                       bool                  `json:"claude_beta_query,omitempty"`                          // Claude 渠道是否强制追加 ?beta=true
	AllowServiceTier                      bool                  `json:"allow_service_tier,omitempty"`                         // 是否允许 service_tier 透传（默认过滤以避免额外计费）
	AllowInferenceGeo                     bool                  `json:"allow_inference_geo,omitempty"`                        // 是否允许 inference_geo 透传（仅 Claude，默认过滤以满足数据驻留合规）
	AllowSpeed                            bool                  `json:"allow_speed,omitempty"`                                // 是否允许 speed 透传（仅 Claude，默认过滤以避免意外切换推理速度模式）
	AllowSafetyIdentifier                 bool                  `json:"allow_safety_identifier,omitempty"`                    // 是否允许 safety_identifier 透传（默认过滤以保护用户隐私）
	DisableStore                          bool                  `json:"disable_store,omitempty"`                              // 是否禁用 store 透传（默认允许透传，禁用后可能导致 Codex 无法使用）
	AllowIncludeObfuscation               bool                  `json:"allow_include_obfuscation,omitempty"`                  // 是否允许 stream_options.include_obfuscation 透传（默认过滤以避免关闭流混淆保护）
	DisableTaskPollingSleep               bool                  `json:"disable_task_polling_sleep,omitempty"`                 // 是否跳过该渠道异步视频任务逐个轮询之间的保护性等待
	AwsKeyType                            AwsKeyType            `json:"aws_key_type,omitempty"`                               // AWS Key 类型
	UpstreamModelUpdateCheckEnabled       bool                  `json:"upstream_model_update_check_enabled,omitempty"`        // 是否检测上游模型更新
	UpstreamModelUpdateAutoSyncEnabled    bool                  `json:"upstream_model_update_auto_sync_enabled,omitempty"`    // 是否自动同步上游模型更新
	UpstreamModelUpdateLastCheckTime      int64                 `json:"upstream_model_update_last_check_time,omitempty"`      // 上次检测时间
	UpstreamModelUpdateLastDetectedModels []string              `json:"upstream_model_update_last_detected_models,omitempty"` // 上次检测到的可加入模型
	UpstreamModelUpdateLastRemovedModels  []string              `json:"upstream_model_update_last_removed_models,omitempty"`  // 上次检测到的可删除模型
	UpstreamModelUpdateIgnoredModels      []string              `json:"upstream_model_update_ignored_models,omitempty"`       // 手动忽略的模型
	AdvancedCustom                        *AdvancedCustomConfig `json:"advanced_custom,omitempty"`                            // Advanced Custom Channel 路由配置
}

// IsOpenRouterEnterprise 检查是否使用 OpenRouter 企业版
//
// 返回值：
//   - bool: 是否使用企业版
func (s *ChannelOtherSettings) IsOpenRouterEnterprise() bool {
	if s == nil || s.OpenRouterEnterprise == nil {
		return false
	}
	return *s.OpenRouterEnterprise
}
