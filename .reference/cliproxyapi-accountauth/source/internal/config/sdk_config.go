// 包 config - sdk_config.go
// 该文件定义了 SDK 配置结构体，包含代理、图像生成、流式传输等设置。
package config

// SDKConfig 表示从 YAML 文件加载的应用程序 SDK 配置。
type SDKConfig struct {
	// ProxyURL 是用于出站请求的可选代理服务器 URL。
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// DisableImageGeneration 控制内置 image_generation 工具是否被注入/允许。
	//
	// 支持的值：
	//   - false（默认）：image_generation 在所有地方启用（正常行为）。
	//   - true：image_generation 在所有地方禁用。服务器停止注入它，从请求有效负载中移除它，
	//     并为 /v1/images/generations 和 /v1/images/edits 返回 404。
	//   - "chat"：为所有非图像端点禁用 image_generation 注入（如 /v1/responses、/v1/chat/completions），
	//     同时保持 /v1/images/generations 和 /v1/images/edits 启用并在那里保留 image_generation。
	DisableImageGeneration DisableImageGenerationMode `yaml:"disable-image-generation" json:"disable-image-generation"`

	// EnableGeminiCLIEndpoint 控制 Gemini CLI 内部端点（/v1internal:*）是否启用。
	// 默认为 false 以确保安全；为 false 时，/v1internal:* 请求被拒绝。
	EnableGeminiCLIEndpoint bool `yaml:"enable-gemini-cli-endpoint" json:"enable-gemini-cli-endpoint"`

	// ForceModelPrefix 要求显式模型前缀（如 "teamA/gemini-3-pro-preview"）
	// 以定向到带前缀的凭证。为 false 时，无前缀的模型请求也可以使用带前缀的凭证。
	ForceModelPrefix bool `yaml:"force-model-prefix" json:"force-model-prefix"`

	// RequestLog 启用或禁用详细请求日志功能。
	RequestLog bool `yaml:"request-log" json:"request-log"`

	// APIKeys 是用于认证客户端到此代理服务器的密钥列表。
	APIKeys []string `yaml:"api-keys" json:"api-keys"`

	// PassthroughHeaders 控制上游响应头是否转发给下游客户端。
	// 默认为 false（禁用）。
	PassthroughHeaders bool `yaml:"passthrough-headers" json:"passthrough-headers"`

	// Streaming 配置服务器端流式行为（保活和安全引导重试）。
	Streaming StreamingConfig `yaml:"streaming" json:"streaming"`

	// NonStreamKeepAliveInterval 控制非流式响应发出空行的频率。
	// <= 0 禁用保活。值以秒为单位。
	NonStreamKeepAliveInterval int `yaml:"nonstream-keepalive-interval,omitempty" json:"nonstream-keepalive-interval,omitempty"`
}

// StreamingConfig 保存服务器流式行为配置。
type StreamingConfig struct {
	// KeepAliveSeconds 控制服务器发出 SSE 心跳（": keep-alive\n\n"）的频率。
	// <= 0 禁用保活。默认为 0。
	KeepAliveSeconds int `yaml:"keepalive-seconds,omitempty" json:"keepalive-seconds,omitempty"`

	// BootstrapRetries 控制服务器在发送任何字节之前可以重试流式请求的次数，
	// 以允许认证轮换/瞬态恢复。
	// <= 0 禁用引导重试。默认为 0。
	BootstrapRetries int `yaml:"bootstrap-retries,omitempty" json:"bootstrap-retries,omitempty"`
}
