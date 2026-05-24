// config - config.go
// 该文件提供公共 SDK 配置 API。
// 重新导出服务器配置类型和辅助函数，使外部项目可以嵌入 CLIProxyAPI
// 而无需导入内部包。包括配置加载、解析、保存和注释规范化等功能。

// Package config provides the public SDK configuration API.
//
// It re-exports the server configuration types and helpers so external projects can
// embed CLIProxyAPI without importing internal packages.
package config

import internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"

// SDKConfig 是 SDK 配置的类型别名
type SDKConfig = internalconfig.SDKConfig

// Config 是完整应用配置的类型别名
type Config = internalconfig.Config

// StreamingConfig 是流式配置的类型别名
type StreamingConfig = internalconfig.StreamingConfig
// TLSConfig 是 TLS 配置的类型别名
type TLSConfig = internalconfig.TLSConfig
// RemoteManagement 是远程管理配置的类型别名
type RemoteManagement = internalconfig.RemoteManagement
// AmpCode 是 Amp Code 配置的类型别名
type AmpCode = internalconfig.AmpCode
// OAuthModelAlias 是 OAuth 模型别名配置的类型别名
type OAuthModelAlias = internalconfig.OAuthModelAlias
// PayloadConfig 是载荷配置的类型别名
type PayloadConfig = internalconfig.PayloadConfig
// PayloadRule 是载荷规则的类型别名
type PayloadRule = internalconfig.PayloadRule
// PayloadFilterRule 是载荷过滤规则的类型别名
type PayloadFilterRule = internalconfig.PayloadFilterRule
// PayloadModelRule 是载荷模型规则的类型别名
type PayloadModelRule = internalconfig.PayloadModelRule

// GeminiKey 是 Gemini API Key 配置的类型别名
type GeminiKey = internalconfig.GeminiKey
// CodexKey 是 Codex API Key 配置的类型别名
type CodexKey = internalconfig.CodexKey
// ClaudeKey 是 Claude API Key 配置的类型别名
type ClaudeKey = internalconfig.ClaudeKey
// VertexCompatKey 是 Vertex 兼容 API Key 配置的类型别名
type VertexCompatKey = internalconfig.VertexCompatKey
// VertexCompatModel 是 Vertex 兼容模型配置的类型别名
type VertexCompatModel = internalconfig.VertexCompatModel
// OpenAICompatibility 是 OpenAI 兼容性配置的类型别名
type OpenAICompatibility = internalconfig.OpenAICompatibility
// OpenAICompatibilityAPIKey 是 OpenAI 兼容性 API Key 配置的类型别名
type OpenAICompatibilityAPIKey = internalconfig.OpenAICompatibilityAPIKey
// OpenAICompatibilityModel 是 OpenAI 兼容性模型配置的类型别名
type OpenAICompatibilityModel = internalconfig.OpenAICompatibilityModel

// TLS 是 TLS 配置的类型别名
type TLS = internalconfig.TLSConfig

// LoadConfig 从指定文件路径加载完整配置。
func LoadConfig(configFile string) (*Config, error) { return internalconfig.LoadConfig(configFile) }

// LoadConfigOptional 从指定文件路径加载配置，可选地允许文件不存在。
func LoadConfigOptional(configFile string, optional bool) (*Config, error) {
	return internalconfig.LoadConfigOptional(configFile, optional)
}

// ParseConfigBytes 从字节数据解析配置。
func ParseConfigBytes(data []byte) (*Config, error) { return internalconfig.ParseConfigBytes(data) }

// SaveConfigPreserveComments 保存配置到文件，保留原始注释格式。
func SaveConfigPreserveComments(configFile string, cfg *Config) error {
	return internalconfig.SaveConfigPreserveComments(configFile, cfg)
}

// SaveConfigPreserveCommentsUpdateNestedScalar 保存配置到文件，
// 仅更新嵌套路径指定的标量值，保留其他内容和注释格式。
func SaveConfigPreserveCommentsUpdateNestedScalar(configFile string, path []string, value string) error {
	return internalconfig.SaveConfigPreserveCommentsUpdateNestedScalar(configFile, path, value)
}

// NormalizeCommentIndentation 规范化配置文件中注释的缩进格式。
func NormalizeCommentIndentation(data []byte) []byte {
	return internalconfig.NormalizeCommentIndentation(data)
}
