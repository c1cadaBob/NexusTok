// 包 config - config.go
// 该文件提供了公共 SDK 配置 API。它重新导出服务端配置类型和辅助函数，
// 使外部项目可以嵌入 CLIProxyAPI 而无需导入内部包。
package config

import internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"

// SDKConfig 是内部 SDKConfig 的类型别名，表示 SDK 整体配置。
type SDKConfig = internalconfig.SDKConfig

// Config 是内部 Config 的类型别名，表示应用主配置。
type Config = internalconfig.Config

// StreamingConfig 是内部 StreamingConfig 的类型别名，表示流式传输配置。
type StreamingConfig = internalconfig.StreamingConfig

// TLSConfig 是内部 TLSConfig 的类型别名，表示 TLS 安全连接配置。
type TLSConfig = internalconfig.TLSConfig

// RemoteManagement 是内部 RemoteManagement 的类型别名，表示远程管理配置。
type RemoteManagement = internalconfig.RemoteManagement

// AmpCode 是内部 AmpCode 的类型别名，表示 AMP 代码配置。
type AmpCode = internalconfig.AmpCode

// OAuthModelAlias 是内部 OAuthModelAlias 的类型别名，表示 OAuth 模型别名配置。
type OAuthModelAlias = internalconfig.OAuthModelAlias

// PayloadConfig 是内部 PayloadConfig 的类型别名，表示请求载荷配置。
type PayloadConfig = internalconfig.PayloadConfig

// PayloadRule 是内部 PayloadRule 的类型别名，表示载荷规则。
type PayloadRule = internalconfig.PayloadRule

// PayloadFilterRule 是内部 PayloadFilterRule 的类型别名，表示载荷过滤规则。
type PayloadFilterRule = internalconfig.PayloadFilterRule

// PayloadModelRule 是内部 PayloadModelRule 的类型别名，表示载荷模型规则。
type PayloadModelRule = internalconfig.PayloadModelRule

// GeminiKey 是内部 GeminiKey 的类型别名，表示 Gemini API 密钥配置。
type GeminiKey = internalconfig.GeminiKey

// CodexKey 是内部 CodexKey 的类型别名，表示 Codex API 密钥配置。
type CodexKey = internalconfig.CodexKey

// ClaudeKey 是内部 ClaudeKey 的类型别名，表示 Claude API 密钥配置。
type ClaudeKey = internalconfig.ClaudeKey

// VertexCompatKey 是内部 VertexCompatKey 的类型别名，表示 Vertex 兼容密钥配置。
type VertexCompatKey = internalconfig.VertexCompatKey

// VertexCompatModel 是内部 VertexCompatModel 的类型别名，表示 Vertex 兼容模型配置。
type VertexCompatModel = internalconfig.VertexCompatModel

// OpenAICompatibility 是内部 OpenAICompatibility 的类型别名，表示 OpenAI 兼容性配置。
type OpenAICompatibility = internalconfig.OpenAICompatibility

// OpenAICompatibilityAPIKey 是内部 OpenAICompatibilityAPIKey 的类型别名，表示 OpenAI 兼容 API 密钥配置。
type OpenAICompatibilityAPIKey = internalconfig.OpenAICompatibilityAPIKey

// OpenAICompatibilityModel 是内部 OpenAICompatibilityModel 的类型别名，表示 OpenAI 兼容模型配置。
type OpenAICompatibilityModel = internalconfig.OpenAICompatibilityModel

// TLS 是 TLSConfig 的便捷别名，表示 TLS 安全连接配置。
type TLS = internalconfig.TLSConfig

// DefaultPanelGitHubRepository 是面板默认 GitHub 仓库地址常量。
const (
	DefaultPanelGitHubRepository = internalconfig.DefaultPanelGitHubRepository
)

// LoadConfig 从指定的配置文件加载应用配置。
//
// 参数:
//   - configFile: 配置文件路径
//
// 返回:
//   - *Config: 加载成功的配置对象
//   - error: 加载失败时返回错误信息
func LoadConfig(configFile string) (*Config, error) { return internalconfig.LoadConfig(configFile) }

// LoadConfigOptional 可选地加载配置文件。当 optional 为 true 且文件不存在时，不返回错误。
//
// 参数:
//   - configFile: 配置文件路径
//   - optional: 是否允许文件不存在
//
// 返回:
//   - *Config: 加载成功的配置对象
//   - error: 加载失败时返回错误信息
func LoadConfigOptional(configFile string, optional bool) (*Config, error) {
	return internalconfig.LoadConfigOptional(configFile, optional)
}

// ParseConfigBytes 从字节数据解析应用配置。
//
// 参数:
//   - data: 配置文件的字节内容
//
// 返回:
//   - *Config: 解析成功的配置对象
//   - error: 解析失败时返回错误信息
func ParseConfigBytes(data []byte) (*Config, error) { return internalconfig.ParseConfigBytes(data) }

// SaveConfigPreserveComments 保存配置到文件，同时保留原始注释格式。
//
// 参数:
//   - configFile: 配置文件路径
//   - cfg: 要保存的配置对象
//
// 返回:
//   - error: 保存失败时返回错误信息
func SaveConfigPreserveComments(configFile string, cfg *Config) error {
	return internalconfig.SaveConfigPreserveComments(configFile, cfg)
}

// SaveConfigPreserveCommentsUpdateNestedScalar 保存配置到文件，同时更新嵌套标量值并保留注释格式。
//
// 参数:
//   - configFile: 配置文件路径
//   - path: 嵌套配置的键路径
//   - value: 要设置的新值
//
// 返回:
//   - error: 保存失败时返回错误信息
func SaveConfigPreserveCommentsUpdateNestedScalar(configFile string, path []string, value string) error {
	return internalconfig.SaveConfigPreserveCommentsUpdateNestedScalar(configFile, path, value)
}

// NormalizeCommentIndentation 规范化配置文件中注释的缩进格式。
//
// 参数:
//   - data: 原始配置文件字节数据
//
// 返回:
//   - []byte: 规范化后的字节数据
func NormalizeCommentIndentation(data []byte) []byte {
	return internalconfig.NormalizeCommentIndentation(data)
}
