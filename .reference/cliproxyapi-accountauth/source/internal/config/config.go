// 包 config 提供 CLI Proxy API 服务器的配置管理功能。
// 该文件处理 YAML 配置文件的加载和解析，提供对应用程序设置的结构化访问，
// 包括服务器端口、认证目录、调试设置、代理配置和 API 密钥等。
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/registry"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultPanelGitHubRepository 是管理面板的默认 GitHub 仓库地址
	DefaultPanelGitHubRepository = "https://github.com/router-for-me/Cli-Proxy-API-Management-Center"
	// DefaultPprofAddr 是 pprof 调试服务器的默认监听地址
	DefaultPprofAddr = "127.0.0.1:8316"
	// DefaultAuthDir 是认证令牌文件的默认存储目录
	DefaultAuthDir = "~/.cli-proxy-api"
)

// Config 表示应用程序的配置，从 YAML 文件加载。
// 包含服务器设置、认证配置、调试选项、API 密钥等所有配置项。
type Config struct {
	SDKConfig `yaml:",inline"`
	// Host 是 API 服务器绑定的网络主机/接口。
	// 默认为空（""），绑定所有接口（IPv4 + IPv6）。使用 "127.0.0.1" 或 "localhost" 仅允许本地访问。
	Host string `yaml:"host" json:"-"`
	// Port 是 API 服务器监听的网络端口。
	Port int `yaml:"port" json:"-"`

	// TLS 配置控制 HTTPS 服务器设置。
	TLS TLSConfig `yaml:"tls" json:"tls"`

	// Home 配置是运行时配置，从 -home-jwt 参数填充。
	Home HomeConfig `yaml:"-" json:"-"`

	// RemoteManagement 嵌套管理相关选项，位于 'remote-management' 配置项下。
	RemoteManagement RemoteManagement `yaml:"remote-management" json:"-"`

	// AuthDir 是认证令牌文件的存储目录。
	AuthDir string `yaml:"auth-dir" json:"-"`

	// Debug 启用或禁用调试级别日志和其他调试功能。
	Debug bool `yaml:"debug" json:"debug"`

	// Pprof 配置控制可选的 pprof HTTP 调试服务器。
	Pprof PprofConfig `yaml:"pprof" json:"pprof"`

	// CommercialMode 禁用高开销的 HTTP 中间件功能，以最小化每请求内存使用。
	CommercialMode bool `yaml:"commercial-mode" json:"commercial-mode"`

	// LoggingToFile 控制应用程序日志是写入轮转文件还是标准输出。
	LoggingToFile bool `yaml:"logging-to-file" json:"logging-to-file"`

	// LogsMaxTotalSizeMB 限制日志目录下日志文件的总大小（MB）。
	// 超出限制时，最旧的日志文件将被删除，直到在限制范围内。设置为 0 禁用。
	LogsMaxTotalSizeMB int `yaml:"logs-max-total-size-mb" json:"logs-max-total-size-mb"`

	// ErrorLogsMaxFiles 限制在禁用请求日志时保留的错误日志文件数量。
	// 超出限制时，最旧的错误日志文件将被删除。默认为 10。设置为 0 禁用清理。
	ErrorLogsMaxFiles int `yaml:"error-logs-max-files" json:"error-logs-max-files"`

	// UsageStatisticsEnabled 切换内存中的使用量聚合；为 false 时，使用数据将被丢弃。
	UsageStatisticsEnabled bool `yaml:"usage-statistics-enabled" json:"usage-statistics-enabled"`

	// RedisUsageQueueRetentionSeconds 控制使用量队列项在内存中保留的时间
	// 用于 Management API 消费者。
	// 默认：60。最大：3600。
	RedisUsageQueueRetentionSeconds int `yaml:"redis-usage-queue-retention-seconds" json:"redis-usage-queue-retention-seconds"`

	// DisableCooling 为 true 时禁用配额冷却调度。
	DisableCooling bool `yaml:"disable-cooling" json:"disable-cooling"`

	// AuthAutoRefreshWorkers 覆盖核心认证自动刷新工作池的大小。
	// 当 <= 0 时，使用默认工作线程数。
	AuthAutoRefreshWorkers int `yaml:"auth-auto-refresh-workers" json:"auth-auto-refresh-workers"`

	// RequestRetry 定义请求失败时的重试次数。
	RequestRetry int `yaml:"request-retry" json:"request-retry"`
	// MaxRetryCredentials 定义失败请求尝试的最大凭证数量。
	// 设置为 0 或负值将保持尝试所有可用凭证（旧行为）。
	MaxRetryCredentials int `yaml:"max-retry-credentials" json:"max-retry-credentials"`
	// MaxRetryInterval 定义重试冷却凭证前的最大等待时间（秒）。
	MaxRetryInterval int `yaml:"max-retry-interval" json:"max-retry-interval"`

	// QuotaExceeded 定义配额超出时的行为。
	QuotaExceeded QuotaExceeded `yaml:"quota-exceeded" json:"quota-exceeded"`

	// Routing 控制凭证选择行为。
	Routing RoutingConfig `yaml:"routing" json:"routing"`

	// WebsocketAuth 启用或禁用 WebSocket API 的认证。
	WebsocketAuth bool `yaml:"ws-auth" json:"ws-auth"`

	// AntigravitySignatureCacheEnabled 控制是否为 thinking 块启用签名缓存验证。
	// 为 true（默认）时，优先使用和验证缓存的签名。
	// 为 false 时，规范化后直接使用客户端签名（旁路模式）。
	AntigravitySignatureCacheEnabled *bool `yaml:"antigravity-signature-cache-enabled,omitempty" json:"antigravity-signature-cache-enabled,omitempty"`

	// AntigravitySignatureBypassStrict 控制签名旁路模式的严格程度
	AntigravitySignatureBypassStrict *bool `yaml:"antigravity-signature-bypass-strict,omitempty" json:"antigravity-signature-bypass-strict,omitempty"`

	// GeminiKey 定义 Gemini API 密钥配置，可选路由覆盖。
	GeminiKey []GeminiKey `yaml:"gemini-api-key" json:"gemini-api-key"`

	// CodexKey 定义 YAML 配置文件中指定的 Codex API 密钥配置列表。
	CodexKey []CodexKey `yaml:"codex-api-key" json:"codex-api-key"`

	// CodexHeaderDefaults 配置 Codex OAuth 模型请求的回退头。
	// 仅在客户端未发送自己的头时使用。
	CodexHeaderDefaults CodexHeaderDefaults `yaml:"codex-header-defaults" json:"codex-header-defaults"`

	// ClaudeKey 定义 YAML 配置文件中指定的 Claude API 密钥配置列表。
	ClaudeKey []ClaudeKey `yaml:"claude-api-key" json:"claude-api-key"`

	// ClaudeHeaderDefaults 配置 Claude API 请求的默认头值。
	// 在客户端未发送自己的头时作为回退使用。
	ClaudeHeaderDefaults ClaudeHeaderDefaults `yaml:"claude-header-defaults" json:"claude-header-defaults"`

	// OpenAICompatibility 定义外部提供商的 OpenAI API 兼容配置。
	OpenAICompatibility []OpenAICompatibility `yaml:"openai-compatibility" json:"openai-compatibility"`

	// VertexCompatAPIKey 定义第三方提供商的 Vertex AI 兼容 API 密钥配置。
	// 用于使用 Vertex AI 风格路径但使用简单 API 密钥认证的服务。
	VertexCompatAPIKey []VertexCompatKey `yaml:"vertex-api-key" json:"vertex-api-key"`

	// AmpCode 包含 Amp CLI 上游配置、管理限制和模型映射。
	AmpCode AmpCode `yaml:"ampcode" json:"ampcode"`

	// OAuthExcludedModels 定义应用于 OAuth/文件认证条目的按提供商的全局模型排除。
	OAuthExcludedModels map[string][]string `yaml:"oauth-excluded-models,omitempty" json:"oauth-excluded-models,omitempty"`

	// OAuthModelAlias 定义 OAuth/文件认证渠道的全局模型名称别名。
	// 这些别名影响支持渠道的模型列表和模型路由：
	// gemini-cli, vertex, aistudio, antigravity, claude, codex, kimi, xai。
	//
	// 注意：这不适用于以下已有的每凭证模型别名功能：
	// gemini-api-key, codex-api-key, claude-api-key, openai-compatibility, vertex-api-key, 和 ampcode。
	OAuthModelAlias map[string][]OAuthModelAlias `yaml:"oauth-model-alias,omitempty" json:"oauth-model-alias,omitempty"`

	// Payload 定义提供商有效负载参数的默认值和覆盖规则。
	Payload PayloadConfig `yaml:"payload" json:"payload"`

	// legacyMigrationPending 标记是否有待处理的旧版迁移
	legacyMigrationPending bool `yaml:"-" json:"-"`
}

// ClaudeHeaderDefaults 配置注入到 Claude API 请求中的默认头值。
// 在传统模式下，UserAgent/PackageVersion/RuntimeVersion/Timeout 在客户端省略时作为回退，
// 而 OS/Arch 保持运行时派生。当启用稳定设备配置文件时，OS/Arch 成为固定的平台基线，
// 而 UserAgent/PackageVersion/RuntimeVersion 作为可升级的软件指纹种子。
type ClaudeHeaderDefaults struct {
	// UserAgent 是 HTTP User-Agent 头的默认值
	UserAgent string `yaml:"user-agent" json:"user-agent"`
	// PackageVersion 是 Claude 包版本的默认值
	PackageVersion string `yaml:"package-version" json:"package-version"`
	// RuntimeVersion 是运行时版本的默认值
	RuntimeVersion string `yaml:"runtime-version" json:"runtime-version"`
	// OS 是操作系统标识的默认值
	OS string `yaml:"os" json:"os"`
	// Arch 是架构标识的默认值
	Arch string `yaml:"arch" json:"arch"`
	// Timeout 是超时值的默认值
	Timeout string `yaml:"timeout" json:"timeout"`
	// StabilizeDeviceProfile 启用稳定设备配置文件模式
	StabilizeDeviceProfile *bool `yaml:"stabilize-device-profile,omitempty" json:"stabilize-device-profile,omitempty"`
}

// CodexHeaderDefaults 配置注入到 Codex 模型请求中的回退头值，
// 用于 OAuth/文件认证，当客户端省略时使用。
// UserAgent 适用于 HTTP 和 websocket 请求；BetaFeatures 仅适用于 websocket。
type CodexHeaderDefaults struct {
	// UserAgent 是 HTTP User-Agent 头的默认值
	UserAgent string `yaml:"user-agent" json:"user-agent"`
	// BetaFeatures 是 websocket Beta 特性头的默认值
	BetaFeatures string `yaml:"beta-features" json:"beta-features"`
}

// TLSConfig 保存 HTTPS 服务器设置。
type TLSConfig struct {
	// Enable 切换 HTTPS 服务器模式
	Enable bool `yaml:"enable" json:"enable"`
	// Cert 是 TLS 证书文件的路径
	Cert string `yaml:"cert" json:"cert"`
	// Key 是 TLS 私钥文件的路径
	Key string `yaml:"key" json:"key"`
}

// PprofConfig 保存 pprof HTTP 服务器设置。
type PprofConfig struct {
	// Enable 切换 pprof HTTP 调试服务器
	Enable bool `yaml:"enable" json:"enable"`
	// Addr 是 pprof HTTP 服务器的 host:port 地址
	Addr string `yaml:"addr" json:"addr"`
}

// RemoteManagement 保存 'remote-management' 下的管理 API 配置。
type RemoteManagement struct {
	// AllowRemote 切换管理 API 的远程（非 localhost）访问
	AllowRemote bool `yaml:"allow-remote"`
	// SecretKey 是管理密钥（明文或 bcrypt 哈希）。YAML 键名故意为 'secret-key'。
	SecretKey string `yaml:"secret-key"`
	// DisableControlPanel 为 true 时跳过提供和同步捆绑的管理 UI。
	DisableControlPanel bool `yaml:"disable-control-panel"`
	// DisableAutoUpdatePanel 禁用管理面板资源的自动定期后台更新。
	// 为 false（默认）时，后台更新器保持启用；为 true 时，面板仅在首次访问缺失时下载。
	DisableAutoUpdatePanel bool `yaml:"disable-auto-update-panel"`
	// PanelGitHubRepository 覆盖用于获取管理面板资源的 GitHub 仓库。
	// 接受仓库 URL（https://github.com/org/repo）或 API releases 端点。
	PanelGitHubRepository string `yaml:"panel-github-repository"`
}

// QuotaExceeded 定义 API 配额限制超出时的行为。
// 提供自动故障转移机制的配置选项。
type QuotaExceeded struct {
	// SwitchProject 指示配额超出时是否自动切换到另一个项目
	SwitchProject bool `yaml:"switch-project" json:"switch-project"`

	// SwitchPreviewModel 指示配额超出时是否自动切换到预览模型
	SwitchPreviewModel bool `yaml:"switch-preview-model" json:"switch-preview-model"`

	// AntigravityCredits 启用基于积分的最后手段回退，用于 Claude 模型。
	// 当所有免费层认证用尽（429/503）时，编排器使用具有可用 Google One AI 积分的认证重试。
	AntigravityCredits bool `yaml:"antigravity-credits" json:"antigravity-credits"`
}

// RoutingConfig 配置如何为请求选择凭证。
type RoutingConfig struct {
	// Strategy 选择凭证选择策略。
	// 支持的值："round-robin"（默认），"fill-first"。
	Strategy string `yaml:"strategy,omitempty" json:"strategy,omitempty"`

	// SessionAffinity 为所有客户端启用通用会话粘性路由。
	// 会话 ID 从多个来源提取：
	// metadata.user_id（Claude Code 会话格式），X-Session-ID，Session_id（Codex），
	// X-Amp-Thread-Id（Amp CLI 线程），X-Client-Request-Id（PI），metadata.user_id，
	// conversation_id 或消息哈希。
	// 当绑定的认证不可用时，自动故障转移始终启用。
	SessionAffinity bool `yaml:"session-affinity,omitempty" json:"session-affinity,omitempty"`

	// SessionAffinityTTL 指定会话到认证绑定的保留时间。
	// 默认：1h。接受持续时间字符串如 "30m"、"1h"、"2h30m"。
	SessionAffinityTTL string `yaml:"session-affinity-ttl,omitempty" json:"session-affinity-ttl,omitempty"`
}

// OAuthModelAlias 定义特定渠道的模型 ID 别名。
// 将上游模型名称（Name）映射到客户端可见的别名（Alias）。
// 当 Fork 为 true 时，别名作为附加模型添加到列表中，同时保持原始模型 ID 可用。
type OAuthModelAlias struct {
	// Name 是上游模型名称
	Name string `yaml:"name" json:"name"`
	// Alias 是客户端可见的别名
	Alias string `yaml:"alias" json:"alias"`
	// Fork 指示是否保留原始模型 ID
	Fork bool `yaml:"fork,omitempty" json:"fork,omitempty"`
}

// AmpModelMapping 定义 Amp CLI 请求的模型名称映射。
// 当 Amp 请求本地不可用的模型时，此映射允许路由到可用的替代模型。
type AmpModelMapping struct {
	// From 是 Amp CLI 请求的模型名称（如 "claude-opus-4.5"）
	From string `yaml:"from" json:"from"`

	// To 是要路由到的目标模型名称（如 "claude-sonnet-4"）。
	// 目标模型必须在注册表中有可用的提供商。
	To string `yaml:"to" json:"to"`

	// Regex 指示 'from' 字段是否应被解释为用于匹配模型名称的正则表达式。
	// 为 true 时，此映射在精确匹配之后按提供的顺序评估。默认为 false（精确匹配）。
	Regex bool `yaml:"regex,omitempty" json:"regex,omitempty"`
}

// AmpCode 将 Amp CLI 集成设置分组，包括上游路由、可选覆盖、管理路由限制和模型回退映射。
type AmpCode struct {
	// UpstreamURL 定义用于非提供商调用的上游 Amp 控制平面
	UpstreamURL string `yaml:"upstream-url" json:"upstream-url"`

	// UpstreamAPIKey 可选地在代理 Amp 上游调用时覆盖 Authorization 头
	UpstreamAPIKey string `yaml:"upstream-api-key" json:"upstream-api-key"`

	// UpstreamAPIKeys 将客户端 API 密钥（来自顶级 api-keys）映射到上游 API 密钥。
	// 当请求使用其中一个 APIKeys 认证时，对应的 UpstreamAPIKey 用于上游 Amp 请求。
	UpstreamAPIKeys []AmpUpstreamAPIKeyEntry `yaml:"upstream-api-keys,omitempty" json:"upstream-api-keys,omitempty"`

	// RestrictManagementToLocalhost 将 Amp 管理路由（/api/user、/api/threads 等）
	// 限制为仅接受来自 localhost（127.0.0.1、::1）的连接。为 true 时，防止浏览器攻击
	// 和远程访问管理端点。默认：false（API 密钥认证已足够）。
	RestrictManagementToLocalhost bool `yaml:"restrict-management-to-localhost" json:"restrict-management-to-localhost"`

	// ModelMappings 定义 Amp CLI 请求的模型名称映射。
	// 当 Amp 请求本地不可用的模型时，这些映射允许路由到可用的替代模型。
	ModelMappings []AmpModelMapping `yaml:"model-mappings" json:"model-mappings"`

	// ForceModelMappings 为 true 时，模型映射优先于本地 API 密钥。
	// 为 false（默认）时，如果可用，优先使用本地 API 密钥。
	ForceModelMappings bool `yaml:"force-model-mappings" json:"force-model-mappings"`
}

// AmpUpstreamAPIKeyEntry 将一组客户端 API 密钥映射到特定的上游 API 密钥。
// 当请求使用其中一个 APIKeys 认证时，对应的 UpstreamAPIKey 用于上游 Amp 请求。
type AmpUpstreamAPIKeyEntry struct {
	// UpstreamAPIKey 是代理到上游 Amp 时使用的 API 密钥
	UpstreamAPIKey string `yaml:"upstream-api-key" json:"upstream-api-key"`

	// APIKeys 是映射到此上游密钥的客户端 API 密钥（来自顶级 api-keys）
	APIKeys []string `yaml:"api-keys" json:"api-keys"`
}

// PayloadConfig 定义应用于提供商有效负载的默认值和覆盖参数规则。
type PayloadConfig struct {
	// Default 定义仅在参数缺失时设置的规则
	Default []PayloadRule `yaml:"default" json:"default"`
	// DefaultRaw 定义仅在缺失时设置原始 JSON 值的规则
	DefaultRaw []PayloadRule `yaml:"default-raw" json:"default-raw"`
	// Override 定义始终设置参数的规则，覆盖任何现有值
	Override []PayloadRule `yaml:"override" json:"override"`
	// OverrideRaw 定义始终设置原始 JSON 值的规则，覆盖任何现有值
	OverrideRaw []PayloadRule `yaml:"override-raw" json:"override-raw"`
	// Filter 定义通过 JSON 路径从有效负载中移除参数的规则
	Filter []PayloadFilterRule `yaml:"filter" json:"filter"`
}

// PayloadFilterRule 描述从匹配的模型有效负载中移除特定 JSON 路径的规则。
type PayloadFilterRule struct {
	// Models 列出带有名称模式和协议约束的模型条目
	Models []PayloadModelRule `yaml:"models" json:"models"`
	// Params 列出要从有效负载中移除的 JSON 路径（gjson/sjson 语法）
	Params []string `yaml:"params" json:"params"`
}

// PayloadRule 描述针对模型列表的单个参数更新规则。
type PayloadRule struct {
	// Models 列出带有名称模式和协议约束的模型条目
	Models []PayloadModelRule `yaml:"models" json:"models"`
	// Params 将 JSON 路径（gjson/sjson 语法）映射到写入有效负载的值。
	// 对于 *-raw 规则，值被视为原始 JSON 片段（字符串按原样使用）。
	Params map[string]any `yaml:"params" json:"params"`
}

// PayloadModelRule 将模型名称模式绑定到特定的翻译器协议。
type PayloadModelRule struct {
	// Name 是模型名称或通配符模式（如 "gpt-*"、"*-5"、"gemini-*-pro"）
	Name string `yaml:"name" json:"name"`
	// Protocol 将规则限制为特定的翻译器格式（如 "gemini"、"responses"）
	Protocol string `yaml:"protocol" json:"protocol"`
	// Headers 将规则限制为请求头匹配所有配置的通配符模式的请求
	Headers map[string]string `yaml:"headers" json:"headers"`
	// FromProtocol 将规则限制为特定的源协议（如 "gemini"、"responses"）
	FromProtocol string `yaml:"from-protocol" json:"from-protocol"`
	// Match 要求有效负载 JSON 路径等于配置的值
	Match []map[string]any `yaml:"match" json:"match"`
	// NotMatch 要求有效负载 JSON 路径不等于配置的值
	NotMatch []map[string]any `yaml:"not-match" json:"not-match"`
	// Exist 要求有效负载 JSON 路径存在且不为 null
	Exist []string `yaml:"exist" json:"exist"`
	// NotExist 要求有效负载 JSON 路径缺失或为 null
	NotExist []string `yaml:"not-exist" json:"not-exist"`
}

// CloakConfig 为非 Claude Code 客户端配置请求伪装。
// 伪装将 API 请求伪装为来自官方 Claude Code CLI 的请求。
type CloakConfig struct {
	// Mode 控制伪装行为："auto"（默认）、"always" 或 "never"。
	// - "auto"：仅在客户端不是 Claude Code 时伪装（基于 User-Agent）
	// - "always"：无论客户端如何都应用伪装
	// - "never"：从不应用伪装
	Mode string `yaml:"mode,omitempty" json:"mode,omitempty"`

	// StrictMode 控制伪装时系统提示的处理方式。
	// - false（默认）：将 Claude Code 提示添加到用户系统消息之前
	// - true：移除所有用户系统消息，仅保留 Claude Code 提示
	StrictMode bool `yaml:"strict-mode,omitempty" json:"strict-mode,omitempty"`

	// SensitiveWords 是要用零宽字符混淆的单词列表。
	// 这可以帮助绕过某些内容过滤器。
	SensitiveWords []string `yaml:"sensitive-words,omitempty" json:"sensitive-words,omitempty"`

	// CacheUserID 控制是否按 API 密钥缓存 Claude user_id 值。
	// 为 false 时，为每个请求生成新的随机 user_id。
	CacheUserID *bool `yaml:"cache-user-id,omitempty" json:"cache-user-id,omitempty"`
}

// ClaudeKey 表示 Claude API 密钥的配置，
// 包括 API 密钥本身和可选的 API 端点基础 URL。
type ClaudeKey struct {
	// APIKey 是访问 Claude API 服务的认证密钥
	APIKey string `yaml:"api-key" json:"api-key"`

	// Priority 控制多个凭证匹配时的选择偏好。
	// 值越高越优先；默认为 0。
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`

	// Prefix 可选地为此凭证的模型命名空间化（如 "teamA/claude-sonnet-4"）。
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL 是 Claude API 端点的基础 URL。
	// 如果为空，将使用默认的 Claude API URL。
	BaseURL string `yaml:"base-url" json:"base-url"`

	// ProxyURL 如果提供，覆盖此 API 密钥的全局代理设置。
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// Models 定义请求路由的上游模型名称和别名。
	Models []ClaudeModel `yaml:"models" json:"models"`

	// Headers 可选地为此密钥发送的请求添加额外的 HTTP 头。
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels 列出此提供商应排除的模型 ID。
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`

	// DisableCooling 为 true 时禁用此凭证的认证/模型冷却调度。
	DisableCooling bool `yaml:"disable-cooling,omitempty" json:"disable-cooling,omitempty"`

	// Cloak 为非 Claude Code 客户端配置请求伪装。
	Cloak *CloakConfig `yaml:"cloak,omitempty" json:"cloak,omitempty"`

	// ExperimentalCCHSigning 启用可选的最终体 cch 签名，用于伪装的
	// Claude /v1/messages 请求。默认禁用，因此上游种子更改不会改变代理的旧行为。
	ExperimentalCCHSigning bool `yaml:"experimental-cch-signing,omitempty" json:"experimental-cch-signing,omitempty"`
}

// GetAPIKey 返回 Claude API 密钥。
func (k ClaudeKey) GetAPIKey() string { return k.APIKey }

// GetAPIKey 返回 Claude API 的基础 URL。
func (k ClaudeKey) GetBaseURL() string { return k.BaseURL }

// ClaudeModel 描述别名与实际上游模型名称之间的映射。
type ClaudeModel struct {
	// Name 是发出请求时使用的上游模型标识符
	Name string `yaml:"name" json:"name"`

	// Alias 是映射到 Name 的客户端面向的模型名称
	Alias string `yaml:"alias" json:"alias"`
}

// GetName 返回上游模型名称。
func (m ClaudeModel) GetName() string { return m.Name }

// GetAlias 返回客户端面向的模型别名。
func (m ClaudeModel) GetAlias() string { return m.Alias }

// CodexKey 表示 Codex API 密钥的配置，
// 包括 API 密钥本身和可选的 API 端点基础 URL。
type CodexKey struct {
	// APIKey 是访问 Codex API 服务的认证密钥
	APIKey string `yaml:"api-key" json:"api-key"`

	// Priority 控制多个凭证匹配时的选择偏好。
	// 值越高越优先；默认为 0。
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`

	// Prefix 可选地为此凭证的模型命名空间化（如 "teamA/gpt-5-codex"）。
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL 是 Codex API 端点的基础 URL。
	// 如果为空，将使用默认的 Codex API URL。
	BaseURL string `yaml:"base-url" json:"base-url"`

	// Websockets 启用此凭证的 Responses API websocket 传输。
	Websockets bool `yaml:"websockets,omitempty" json:"websockets,omitempty"`

	// ProxyURL 如果提供，覆盖此 API 密钥的全局代理设置。
	ProxyURL string `yaml:"proxy-url" json:"proxy-url"`

	// Models 定义请求路由的上游模型名称和别名。
	Models []CodexModel `yaml:"models" json:"models"`

	// Headers 可选地为此密钥发送的请求添加额外的 HTTP 头。
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels 列出此提供商应排除的模型 ID。
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`

	// DisableCooling 为 true 时禁用此凭证的认证/模型冷却调度。
	DisableCooling bool `yaml:"disable-cooling,omitempty" json:"disable-cooling,omitempty"`
}

// GetAPIKey 返回 Codex API 密钥。
func (k CodexKey) GetAPIKey() string { return k.APIKey }

// GetBaseURL 返回 Codex API 的基础 URL。
func (k CodexKey) GetBaseURL() string { return k.BaseURL }

// CodexModel 描述别名与实际上游模型名称之间的映射。
type CodexModel struct {
	// Name 是发出请求时使用的上游模型标识符
	Name string `yaml:"name" json:"name"`

	// Alias 是映射到 Name 的客户端面向的模型名称
	Alias string `yaml:"alias" json:"alias"`
}

// GetName 返回上游模型名称。
func (m CodexModel) GetName() string { return m.Name }

// GetAlias 返回客户端面向的模型别名。
func (m CodexModel) GetAlias() string { return m.Alias }

// GeminiKey 表示 Gemini API 密钥的配置，
// 包括上游基础 URL、代理路由和头的可选覆盖。
type GeminiKey struct {
	// APIKey 是访问 Gemini API 服务的认证密钥
	APIKey string `yaml:"api-key" json:"api-key"`

	// Priority 控制多个凭证匹配时的选择偏好。
	// 值越高越优先；默认为 0。
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`

	// Prefix 可选地为此凭证的模型命名空间化（如 "teamA/gemini-3-pro-preview"）。
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL 可选地覆盖 Gemini API 端点。
	BaseURL string `yaml:"base-url,omitempty" json:"base-url,omitempty"`

	// ProxyURL 可选地覆盖此 API 密钥的全局代理。
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`

	// Models 定义请求路由的上游模型名称和别名。
	Models []GeminiModel `yaml:"models,omitempty" json:"models,omitempty"`

	// Headers 可选地为此密钥发送的请求添加额外的 HTTP 头。
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// ExcludedModels 列出此提供商应排除的模型 ID。
	ExcludedModels []string `yaml:"excluded-models,omitempty" json:"excluded-models,omitempty"`

	// DisableCooling 为 true 时禁用此凭证的认证/模型冷却调度。
	DisableCooling bool `yaml:"disable-cooling,omitempty" json:"disable-cooling,omitempty"`
}

// GetAPIKey 返回 Gemini API 密钥。
func (k GeminiKey) GetAPIKey() string { return k.APIKey }

// GetBaseURL 返回 Gemini API 的基础 URL。
func (k GeminiKey) GetBaseURL() string { return k.BaseURL }

// GeminiModel 描述别名与实际上游模型名称之间的映射。
type GeminiModel struct {
	// Name 是发出请求时使用的上游模型标识符
	Name string `yaml:"name" json:"name"`

	// Alias 是映射到 Name 的客户端面向的模型名称
	Alias string `yaml:"alias" json:"alias"`
}

// GetName 返回上游模型名称。
func (m GeminiModel) GetName() string { return m.Name }

// GetAlias 返回客户端面向的模型别名。
func (m GeminiModel) GetAlias() string { return m.Alias }

// OpenAICompatibility 表示与外部提供商的 OpenAI API 兼容配置，
// 允许模型别名通过 OpenAI API 格式路由。
type OpenAICompatibility struct {
	// Name 是此 OpenAI 兼容配置的标识符
	Name string `yaml:"name" json:"name"`

	// Priority 控制多个提供商或凭证匹配时的选择偏好。
	// 值越高越优先；默认为 0。
	Priority int `yaml:"priority,omitempty" json:"priority,omitempty"`

	// Disabled 阻止此提供商用于路由。
	Disabled bool `yaml:"disabled,omitempty" json:"disabled,omitempty"`

	// Prefix 可选地为此提供商的模型别名命名空间化（如 "teamA/kimi-k2"）。
	Prefix string `yaml:"prefix,omitempty" json:"prefix,omitempty"`

	// BaseURL 是外部 OpenAI 兼容 API 端点的基础 URL。
	BaseURL string `yaml:"base-url" json:"base-url"`

	// APIKeyEntries 定义带有可选的每密钥代理配置的 API 密钥。
	APIKeyEntries []OpenAICompatibilityAPIKey `yaml:"api-key-entries,omitempty" json:"api-key-entries,omitempty"`

	// Models 定义包括路由别名的模型配置。
	Models []OpenAICompatibilityModel `yaml:"models" json:"models"`

	// Headers 可选地为此提供商发送的请求添加额外的 HTTP 头。
	Headers map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`

	// DisableCooling 为 true 时禁用此提供商的认证/模型冷却调度。
	DisableCooling bool `yaml:"disable-cooling,omitempty" json:"disable-cooling,omitempty"`
}

// OpenAICompatibilityAPIKey 表示带有可选代理设置的 API 密钥配置。
type OpenAICompatibilityAPIKey struct {
	// APIKey 是访问外部 API 服务的认证密钥
	APIKey string `yaml:"api-key" json:"api-key"`

	// ProxyURL 如果提供，覆盖此 API 密钥的全局代理设置。
	ProxyURL string `yaml:"proxy-url,omitempty" json:"proxy-url,omitempty"`
}

// OpenAICompatibilityModel 表示 OpenAI 兼容的模型配置，
// 包括实际模型名称及其用于 API 路由的别名。
type OpenAICompatibilityModel struct {
	// Name 是外部提供商使用的实际模型名称
	Name string `yaml:"name" json:"name"`

	// Alias 是客户端用于引用此模型的模型名称别名
	Alias string `yaml:"alias" json:"alias"`

	// Image 标记此模型可通过 /v1/images/generations 和 /v1/images/edits 调用。
	Image bool `yaml:"image,omitempty" json:"image,omitempty"`

	// Thinking 配置此模型的思考/推理能力。
	// 如果为 nil，模型默认使用基于级别的推理，级别为 ["low", "medium", "high"]。
	Thinking *registry.ThinkingSupport `yaml:"thinking,omitempty" json:"thinking,omitempty"`
}

// GetName 返回实际模型名称。
func (m OpenAICompatibilityModel) GetName() string { return m.Name }

// GetAlias 返回模型别名。
func (m OpenAICompatibilityModel) GetAlias() string { return m.Alias }

// LoadConfig 从给定路径读取 YAML 配置文件，
// 将其反序列化为 Config 结构体，应用环境变量覆盖，并返回。
//
// 参数：
//   - configFile: YAML 配置文件的路径
//
// 返回：
//   - *Config: 加载的配置
//   - error: 配置无法加载时返回的错误
func LoadConfig(configFile string) (*Config, error) {
	return LoadConfigOptional(configFile, false)
}

// LoadConfigOptional 从 configFile 读取 YAML。
// 如果 optional 为 true 且文件不存在，返回空 Config。
// 如果 optional 为 true 且文件为空或无效，返回空 Config。
func LoadConfigOptional(configFile string, optional bool) (*Config, error) {
	// 将整个配置文件读入内存。
	data, err := os.ReadFile(configFile)
	if err != nil {
		if optional {
			if os.IsNotExist(err) || errors.Is(err, syscall.EISDIR) {
				// 缺失且可选：返回空配置（云部署待机模式）。
				return &Config{}, nil
			}
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 在云部署模式（optional=true）下，如果文件为空或仅包含空白，返回空配置。
	if optional && len(data) == 0 {
		return &Config{}, nil
	}

	// 将 YAML 数据反序列化为 Config 结构体。
	var cfg Config
	// 在反序列化前设置默认值，以便缺失的键保留默认值。
	cfg.Host = "" // 默认为空：绑定所有接口（IPv4 + IPv6）
	cfg.LoggingToFile = false
	cfg.LogsMaxTotalSizeMB = 0
	cfg.ErrorLogsMaxFiles = 10
	cfg.UsageStatisticsEnabled = false
	cfg.RedisUsageQueueRetentionSeconds = 60
	cfg.DisableCooling = false
	cfg.DisableImageGeneration = DisableImageGenerationOff
	cfg.Pprof.Enable = false
	cfg.Pprof.Addr = DefaultPprofAddr
	cfg.AmpCode.RestrictManagementToLocalhost = false // 默认为 false：API 密钥认证已足够
	cfg.RemoteManagement.PanelGitHubRepository = DefaultPanelGitHubRepository
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		if optional {
			// 在云部署模式下，如果 YAML 解析失败，返回空配置而不是错误。
			return &Config{}, nil
		}
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// 注意：启动时的旧版密钥迁移被故意禁用。
	// 原因：避免在服务器启动期间修改 config.yaml。
	// 如果需要再次自动启动迁移，请重新启用下面的代码块。
	// var legacy legacyConfigData
	// if errLegacy := yaml.Unmarshal(data, &legacy); errLegacy == nil {
	// 	if cfg.migrateLegacyGeminiKeys(legacy.LegacyGeminiKeys) {
	// 		cfg.legacyMigrationPending = true
	// 	}
	// 	if cfg.migrateLegacyOpenAICompatibilityKeys(legacy.OpenAICompat) {
	// 		cfg.legacyMigrationPending = true
	// 	}
	// 	if cfg.migrateLegacyAmpConfig(&legacy) {
	// 		cfg.legacyMigrationPending = true
	// 	}
	// }

	// 如果检测到明文远程管理密钥，则进行哈希处理（嵌套式）
	// 如果值看起来像 bcrypt 哈希（$2a$、$2b$ 或 $2y$ 前缀），则认为已经哈希过。
	if cfg.RemoteManagement.SecretKey != "" && !looksLikeBcrypt(cfg.RemoteManagement.SecretKey) {
		hashed, errHash := hashSecret(cfg.RemoteManagement.SecretKey)
		if errHash != nil {
			return nil, fmt.Errorf("failed to hash remote management key: %w", errHash)
		}
		cfg.RemoteManagement.SecretKey = hashed

		// 将哈希值持久化回配置文件，避免下次启动时重复哈希。
		// 保留 YAML 注释和顺序；仅更新嵌套键。
		_ = SaveConfigPreserveCommentsUpdateNestedScalar(configFile, []string{"remote-management", "secret-key"}, hashed)
	}

	cfg.RemoteManagement.PanelGitHubRepository = strings.TrimSpace(cfg.RemoteManagement.PanelGitHubRepository)
	if cfg.RemoteManagement.PanelGitHubRepository == "" {
		cfg.RemoteManagement.PanelGitHubRepository = DefaultPanelGitHubRepository
	}

	cfg.Pprof.Addr = strings.TrimSpace(cfg.Pprof.Addr)
	if cfg.Pprof.Addr == "" {
		cfg.Pprof.Addr = DefaultPprofAddr
	}

	if cfg.LogsMaxTotalSizeMB < 0 {
		cfg.LogsMaxTotalSizeMB = 0
	}

	if cfg.ErrorLogsMaxFiles < 0 {
		cfg.ErrorLogsMaxFiles = 10
	}

	if cfg.RedisUsageQueueRetentionSeconds <= 0 {
		cfg.RedisUsageQueueRetentionSeconds = 60
	} else if cfg.RedisUsageQueueRetentionSeconds > 3600 {
		log.WithField("value", cfg.RedisUsageQueueRetentionSeconds).Warn("redis-usage-queue-retention-seconds too large; clamping to 3600")
		cfg.RedisUsageQueueRetentionSeconds = 3600
	}

	if cfg.MaxRetryCredentials < 0 {
		cfg.MaxRetryCredentials = 0
	}

	// 清理 Gemini API 密钥配置并迁移旧版条目。
	cfg.SanitizeGeminiKeys()

	// 清理 Vertex 兼容 API 密钥。
	cfg.SanitizeVertexCompatKeys()

	// 清理 Codex 密钥：丢弃没有 base-url 的条目
	cfg.SanitizeCodexKeys()

	// 清理 Codex 头默认值。
	cfg.SanitizeCodexHeaderDefaults()

	// 清理 Claude 头默认值。
	cfg.SanitizeClaudeHeaderDefaults()

	// 清理 Claude 密钥头
	cfg.SanitizeClaudeKeys()

	// 清理 OpenAI 兼容提供商：丢弃没有 base-url 的条目
	cfg.SanitizeOpenAICompatibility()

	// 规范化 OAuth 提供商模型排除映射。
	cfg.OAuthExcludedModels = NormalizeOAuthExcludedModels(cfg.OAuthExcludedModels)

	// 规范化全局 OAuth 模型名称别名。
	cfg.SanitizeOAuthModelAlias()

	// 验证原始有效负载规则并丢弃无效条目。
	cfg.SanitizePayloadRules()

	// 注意：旧版迁移持久化与启动时旧版迁移一起被故意禁用，
	// 以保持启动时对 config.yaml 的只读性。
	// 如果需要再次自动启动迁移，请重新启用下面的代码块。
	// if cfg.legacyMigrationPending {
	// 	fmt.Println("Detected legacy configuration keys, attempting to persist the normalized config...")
	// 	if !optional && configFile != "" {
	// 		if err := SaveConfigPreserveComments(configFile, &cfg); err != nil {
	// 			return nil, fmt.Errorf("failed to persist migrated legacy config: %w", err)
	// 		}
	// 		fmt.Println("Legacy configuration normalized and persisted.")
	// 	} else {
	// 		fmt.Println("Legacy configuration normalized in memory; persistence skipped.")
	// 	}
	// }

	// 返回填充的配置结构体。
	return &cfg, nil
}

// SanitizePayloadRules 验证原始 JSON 有效负载规则参数并丢弃无效规则。
func (cfg *Config) SanitizePayloadRules() {
	if cfg == nil {
		return
	}
	cfg.Payload.DefaultRaw = sanitizePayloadRawRules(cfg.Payload.DefaultRaw, "default-raw")
	cfg.Payload.OverrideRaw = sanitizePayloadRawRules(cfg.Payload.OverrideRaw, "override-raw")
}

// sanitizePayloadRawRules 验证原始 JSON 有效负载规则参数并丢弃无效规则。
func sanitizePayloadRawRules(rules []PayloadRule, section string) []PayloadRule {
	if len(rules) == 0 {
		return rules
	}
	out := make([]PayloadRule, 0, len(rules))
	for i := range rules {
		rule := rules[i]
		if len(rule.Params) == 0 {
			continue
		}
		invalid := false
		for path, value := range rule.Params {
			raw, ok := payloadRawString(value)
			if !ok {
				continue
			}
			trimmed := bytes.TrimSpace(raw)
			if len(trimmed) == 0 || !json.Valid(trimmed) {
				log.WithFields(log.Fields{
					"section":    section,
					"rule_index": i + 1,
					"param":      path,
				}).Warn("payload rule dropped: invalid raw JSON")
				invalid = true
				break
			}
		}
		if invalid {
			continue
		}
		out = append(out, rule)
	}
	return out
}

// payloadRawString 将值转换为字节切片，用于原始 JSON 验证。
func payloadRawString(value any) ([]byte, bool) {
	switch typed := value.(type) {
	case string:
		return []byte(typed), true
	case []byte:
		return typed, true
	default:
		return nil, false
	}
}

// SanitizeCodexHeaderDefaults 修剪配置的 Codex 头回退值的周围空白。
func (cfg *Config) SanitizeCodexHeaderDefaults() {
	if cfg == nil {
		return
	}
	cfg.CodexHeaderDefaults.UserAgent = strings.TrimSpace(cfg.CodexHeaderDefaults.UserAgent)
	cfg.CodexHeaderDefaults.BetaFeatures = strings.TrimSpace(cfg.CodexHeaderDefaults.BetaFeatures)
}

// SanitizeClaudeHeaderDefaults 修剪配置的 Claude 指纹基线值的周围空白。
func (cfg *Config) SanitizeClaudeHeaderDefaults() {
	if cfg == nil {
		return
	}
	cfg.ClaudeHeaderDefaults.UserAgent = strings.TrimSpace(cfg.ClaudeHeaderDefaults.UserAgent)
	cfg.ClaudeHeaderDefaults.PackageVersion = strings.TrimSpace(cfg.ClaudeHeaderDefaults.PackageVersion)
	cfg.ClaudeHeaderDefaults.RuntimeVersion = strings.TrimSpace(cfg.ClaudeHeaderDefaults.RuntimeVersion)
	cfg.ClaudeHeaderDefaults.OS = strings.TrimSpace(cfg.ClaudeHeaderDefaults.OS)
	cfg.ClaudeHeaderDefaults.Arch = strings.TrimSpace(cfg.ClaudeHeaderDefaults.Arch)
	cfg.ClaudeHeaderDefaults.Timeout = strings.TrimSpace(cfg.ClaudeHeaderDefaults.Timeout)
}

// SanitizeOAuthModelAlias 规范化和去重全局 OAuth 模型名称别名。
// 修剪空白，将渠道键规范化为小写，丢弃空条目，
// 允许每个上游名称有多个别名，并确保每个渠道内的别名唯一。
func (cfg *Config) SanitizeOAuthModelAlias() {
	if cfg == nil || len(cfg.OAuthModelAlias) == 0 {
		return
	}
	out := make(map[string][]OAuthModelAlias, len(cfg.OAuthModelAlias))
	for rawChannel, aliases := range cfg.OAuthModelAlias {
		channel := strings.ToLower(strings.TrimSpace(rawChannel))
		if channel == "" || len(aliases) == 0 {
			continue
		}
		seenAlias := make(map[string]struct{}, len(aliases))
		clean := make([]OAuthModelAlias, 0, len(aliases))
		for _, entry := range aliases {
			name := strings.TrimSpace(entry.Name)
			alias := strings.TrimSpace(entry.Alias)
			if name == "" || alias == "" {
				continue
			}
			if strings.EqualFold(name, alias) {
				continue
			}
			aliasKey := strings.ToLower(alias)
			if _, ok := seenAlias[aliasKey]; ok {
				continue
			}
			seenAlias[aliasKey] = struct{}{}
			clean = append(clean, OAuthModelAlias{Name: name, Alias: alias, Fork: entry.Fork})
		}
		if len(clean) > 0 {
			out[channel] = clean
		}
	}
	cfg.OAuthModelAlias = out
}

// SanitizeOpenAICompatibility 移除不可操作的 OpenAI 兼容提供商条目，
// 特别是缺少 BaseURL 的条目。在评估前修剪空白，并保留剩余条目的相对顺序。
func (cfg *Config) SanitizeOpenAICompatibility() {
	if cfg == nil || len(cfg.OpenAICompatibility) == 0 {
		return
	}
	out := make([]OpenAICompatibility, 0, len(cfg.OpenAICompatibility))
	for i := range cfg.OpenAICompatibility {
		e := cfg.OpenAICompatibility[i]
		e.Name = strings.TrimSpace(e.Name)
		e.Prefix = normalizeModelPrefix(e.Prefix)
		e.BaseURL = strings.TrimSpace(e.BaseURL)
		e.Headers = NormalizeHeaders(e.Headers)
		if e.BaseURL == "" {
			// Skip providers with no base-url; treated as removed
			continue
		}
		out = append(out, e)
	}
	cfg.OpenAICompatibility = out
}

// SanitizeCodexKeys 移除缺少 BaseURL 的 Codex API 密钥条目。
// 修剪空白并保留剩余条目的顺序。
func (cfg *Config) SanitizeCodexKeys() {
	if cfg == nil || len(cfg.CodexKey) == 0 {
		return
	}
	out := make([]CodexKey, 0, len(cfg.CodexKey))
	for i := range cfg.CodexKey {
		e := cfg.CodexKey[i]
		e.Prefix = normalizeModelPrefix(e.Prefix)
		e.BaseURL = strings.TrimSpace(e.BaseURL)
		e.Headers = NormalizeHeaders(e.Headers)
		e.ExcludedModels = NormalizeExcludedModels(e.ExcludedModels)
		if e.BaseURL == "" {
			continue
		}
		out = append(out, e)
	}
	cfg.CodexKey = out
}

// SanitizeClaudeKeys 规范化 Claude 凭证的头。
func (cfg *Config) SanitizeClaudeKeys() {
	if cfg == nil || len(cfg.ClaudeKey) == 0 {
		return
	}
	for i := range cfg.ClaudeKey {
		entry := &cfg.ClaudeKey[i]
		entry.Prefix = normalizeModelPrefix(entry.Prefix)
		entry.Headers = NormalizeHeaders(entry.Headers)
		entry.ExcludedModels = NormalizeExcludedModels(entry.ExcludedModels)
	}
}

// SanitizeGeminiKeys 去重和规范化 Gemini 凭证。
// 使用 API 密钥 + 基础 URL 作为唯一性键。
func (cfg *Config) SanitizeGeminiKeys() {
	if cfg == nil {
		return
	}

	seen := make(map[string]struct{}, len(cfg.GeminiKey))
	out := cfg.GeminiKey[:0]
	for i := range cfg.GeminiKey {
		entry := cfg.GeminiKey[i]
		entry.APIKey = strings.TrimSpace(entry.APIKey)
		if entry.APIKey == "" {
			continue
		}
		entry.Prefix = normalizeModelPrefix(entry.Prefix)
		entry.BaseURL = strings.TrimSpace(entry.BaseURL)
		entry.ProxyURL = strings.TrimSpace(entry.ProxyURL)
		entry.Headers = NormalizeHeaders(entry.Headers)
		entry.ExcludedModels = NormalizeExcludedModels(entry.ExcludedModels)
		uniqueKey := entry.APIKey + "|" + entry.BaseURL
		if _, exists := seen[uniqueKey]; exists {
			continue
		}
		seen[uniqueKey] = struct{}{}
		out = append(out, entry)
	}
	cfg.GeminiKey = out
}

// normalizeModelPrefix 规范化模型前缀，修剪空白和斜杠。
func normalizeModelPrefix(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	trimmed = strings.Trim(trimmed, "/")
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "/") {
		return ""
	}
	return trimmed
}

// looksLikeBcrypt 如果提供的字符串看起来像 bcrypt 哈希则返回 true。
func looksLikeBcrypt(s string) bool {
	return len(s) > 4 && (s[:4] == "$2a$" || s[:4] == "$2b$" || s[:4] == "$2y$")
}

// NormalizeHeaders 修剪头键和值并移除空对。
func NormalizeHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	clean := make(map[string]string, len(headers))
	for k, v := range headers {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		clean[key] = val
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

// NormalizeExcludedModels 修剪、小写化和去重模型排除模式。
// 保留首次出现的顺序并丢弃空条目。
func NormalizeExcludedModels(models []string) []string {
	if len(models) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, raw := range models {
		trimmed := strings.ToLower(strings.TrimSpace(raw))
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// NormalizeOAuthExcludedModels 通过规范化提供商键和对每个条目应用模型排除规范化来清理提供商 -> 排除模型映射。
func NormalizeOAuthExcludedModels(entries map[string][]string) map[string][]string {
	if len(entries) == 0 {
		return nil
	}
	out := make(map[string][]string, len(entries))
	for provider, models := range entries {
		key := strings.ToLower(strings.TrimSpace(provider))
		if key == "" {
			continue
		}
		normalized := NormalizeExcludedModels(models)
		if len(normalized) == 0 {
			continue
		}
		out[key] = normalized
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// hashSecret 使用 bcrypt 对给定的密钥进行哈希。
func hashSecret(secret string) (string, error) {
	// 使用默认成本以保持简单。
	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(secret), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}

// SaveConfigPreserveComments 将配置写回 YAML，同时通过加载原始文件到 yaml.Node 树中
// 并原地更新值来保留现有注释和键顺序。
func SaveConfigPreserveComments(configFile string, cfg *Config) error {
	persistCfg := cfg
	// 加载原始 YAML 为节点树以保留注释和顺序。
	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}

	var original yaml.Node
	if err = yaml.Unmarshal(data, &original); err != nil {
		return err
	}
	if original.Kind != yaml.DocumentNode || len(original.Content) == 0 {
		return fmt.Errorf("invalid yaml document structure")
	}
	if original.Content[0] == nil || original.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("expected root mapping node")
	}

	// 将当前 cfg 序列化为 YAML，然后反序列化为我们可以从中合并的 yaml.Node。
	rendered, err := yaml.Marshal(persistCfg)
	if err != nil {
		return err
	}
	var generated yaml.Node
	if err = yaml.Unmarshal(rendered, &generated); err != nil {
		return err
	}
	if generated.Kind != yaml.DocumentNode || len(generated.Content) == 0 || generated.Content[0] == nil {
		return fmt.Errorf("invalid generated yaml structure")
	}
	if generated.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("expected generated root mapping node")
	}

	// 在合并回清理后的配置之前移除已弃用的部分。
	removeLegacyAuthBlock(original.Content[0])
	removeLegacyOpenAICompatAPIKeys(original.Content[0])
	removeLegacyAmpKeys(original.Content[0])
	removeLegacyGenerativeLanguageKeys(original.Content[0])

	pruneMappingToGeneratedKeys(original.Content[0], generated.Content[0], "oauth-excluded-models")
	pruneMappingToGeneratedKeys(original.Content[0], generated.Content[0], "oauth-model-alias")

	// 将生成的合并到原始的原地，保留现有节点的注释/顺序。
	mergeMappingPreserve(original.Content[0], generated.Content[0])
	normalizeCollectionNodeStyles(original.Content[0])

	// 写回文件。
	f, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err = enc.Encode(&original); err != nil {
		_ = enc.Close()
		return err
	}
	if err = enc.Close(); err != nil {
		return err
	}
	data = NormalizeCommentIndentation(buf.Bytes())
	_, err = f.Write(data)
	return err
}

// SaveConfigPreserveCommentsUpdateNestedScalar 更新嵌套标量键路径（如 ["a","b"]）
// 同时保留注释和位置。
func SaveConfigPreserveCommentsUpdateNestedScalar(configFile string, path []string, value string) error {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return err
	}
	var root yaml.Node
	if err = yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("invalid yaml document structure")
	}
	node := root.Content[0]
	// 沿路径下降映射节点
	for i, key := range path {
		if i == len(path)-1 {
			// 设置最终标量
			v := getOrCreateMapValue(node, key)
			v.Kind = yaml.ScalarNode
			v.Tag = "!!str"
			v.Value = value
		} else {
			next := getOrCreateMapValue(node, key)
			if next.Kind != yaml.MappingNode {
				next.Kind = yaml.MappingNode
				next.Tag = "!!map"
			}
			node = next
		}
	}
	f, err := os.Create(configFile)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err = enc.Encode(&root); err != nil {
		_ = enc.Close()
		return err
	}
	if err = enc.Close(); err != nil {
		return err
	}
	data = NormalizeCommentIndentation(buf.Bytes())
	_, err = f.Write(data)
	return err
}

// NormalizeCommentIndentation 从独立的 YAML 注释行中移除缩进，使其保持左对齐。
func NormalizeCommentIndentation(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	changed := false
	for i, line := range lines {
		trimmed := bytes.TrimLeft(line, " \t")
		if len(trimmed) == 0 || trimmed[0] != '#' {
			continue
		}
		if len(trimmed) == len(line) {
			continue
		}
		lines[i] = append([]byte(nil), trimmed...)
		changed = true
	}
	if !changed {
		return data
	}
	return bytes.Join(lines, []byte("\n"))
}

// getOrCreateMapValue 在映射节点中查找给定键的值节点。
// 如果未找到，追加新的键/值对并返回新的值节点。
func getOrCreateMapValue(mapNode *yaml.Node, key string) *yaml.Node {
	if mapNode.Kind != yaml.MappingNode {
		mapNode.Kind = yaml.MappingNode
		mapNode.Tag = "!!map"
		mapNode.Content = nil
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		k := mapNode.Content[i]
		if k.Value == key {
			return mapNode.Content[i+1]
		}
	}
	// 追加新的键/值
	mapNode.Content = append(mapNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key})
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: ""}
	mapNode.Content = append(mapNode.Content, val)
	return val
}

// mergeMappingPreserve 将 src 中的键合并到 dst 映射节点中，同时保留
// dst 中现有键的键顺序和注释。仅当新键的值非零且不是已知默认值时才添加，
// 以避免用默认值污染配置。
func mergeMappingPreserve(dst, src *yaml.Node, path ...[]string) {
	var currentPath []string
	if len(path) > 0 {
		currentPath = path[0]
	}

	if dst == nil || src == nil {
		return
	}
	if dst.Kind != yaml.MappingNode || src.Kind != yaml.MappingNode {
		// 如果类型不匹配，倾向于用 src 语义原地替换 dst
		// 但保留 dst 节点对象以保留父级附加的任何注释。
		copyNodeShallow(dst, src)
		return
	}
	for i := 0; i+1 < len(src.Content); i += 2 {
		sk := src.Content[i]
		sv := src.Content[i+1]
		idx := findMapKeyIndex(dst, sk.Value)
		childPath := appendPath(currentPath, sk.Value)
		if idx >= 0 {
			// 合并到现有值节点（始终更新，即使是零值）
			dv := dst.Content[idx+1]
			mergeNodePreserve(dv, sv, childPath)
		} else {
			// 新键：仅在值非零且不是已知默认值时添加
			candidate := deepCopyNode(sv)
			pruneKnownDefaultsInNewNode(childPath, candidate)
			if isKnownDefaultValue(childPath, candidate) {
				continue
			}
			dst.Content = append(dst.Content, deepCopyNode(sk), candidate)
		}
	}
}

// mergeNodePreserve 将 src 合并到 dst 中用于标量、映射和序列，同时
// 重用目标节点以保留注释和锚点。对于序列，按索引原地更新。
func mergeNodePreserve(dst, src *yaml.Node, path ...[]string) {
	var currentPath []string
	if len(path) > 0 {
		currentPath = path[0]
	}

	if dst == nil || src == nil {
		return
	}
	switch src.Kind {
	case yaml.MappingNode:
		if dst.Kind != yaml.MappingNode {
			copyNodeShallow(dst, src)
		}
		mergeMappingPreserve(dst, src, currentPath)
	case yaml.SequenceNode:
		// 如果 dst 为 null 且 src 为空序列，保留显式 null 样式
		if dst.Kind == yaml.ScalarNode && dst.Tag == "!!null" && len(src.Content) == 0 {
			// 保持为 null 以保留原始样式
			return
		}
		if dst.Kind != yaml.SequenceNode {
			dst.Kind = yaml.SequenceNode
			dst.Tag = "!!seq"
			dst.Content = nil
		}
		reorderSequenceForMerge(dst, src)
		// 原地更新元素
		minContent := len(dst.Content)
		if len(src.Content) < minContent {
			minContent = len(src.Content)
		}
		for i := 0; i < minContent; i++ {
			if dst.Content[i] == nil {
				dst.Content[i] = deepCopyNode(src.Content[i])
				continue
			}
			mergeNodePreserve(dst.Content[i], src.Content[i], currentPath)
			if dst.Content[i] != nil && src.Content[i] != nil &&
				dst.Content[i].Kind == yaml.MappingNode && src.Content[i].Kind == yaml.MappingNode {
				pruneMissingMapKeys(dst.Content[i], src.Content[i])
			}
		}
		// 追加 src 中的额外项
		for i := len(dst.Content); i < len(src.Content); i++ {
			dst.Content = append(dst.Content, deepCopyNode(src.Content[i]))
		}
		// 如果 dst 有 src 中没有的额外项，则截断
		if len(src.Content) < len(dst.Content) {
			dst.Content = dst.Content[:len(src.Content)]
		}
	case yaml.ScalarNode, yaml.AliasNode:
		// 对于标量，更新 Tag 和 Value 但从 dst 保留 Style 以保留引号
		dst.Kind = src.Kind
		dst.Tag = src.Tag
		dst.Value = src.Value
		// 故意保留 dst.Style 不变
	case 0:
		// 未知/空类型；不执行任何操作
	default:
		// 回退：浅层替换
		copyNodeShallow(dst, src)
	}
}

// findMapKeyIndex 返回 dst 映射中键节点的索引（键的索引，不是值）。
// 未找到时返回 -1。
func findMapKeyIndex(mapNode *yaml.Node, key string) int {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode {
		return -1
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i] != nil && mapNode.Content[i].Value == key {
			return i
		}
	}
	return -1
}

// appendPath 将键追加到路径，返回新的切片以避免修改原始切片。
func appendPath(path []string, key string) []string {
	if len(path) == 0 {
		return []string{key}
	}
	newPath := make([]string, len(path)+1)
	copy(newPath, path)
	newPath[len(path)] = key
	return newPath
}

// isKnownDefaultValue 如果给定节点在指定路径处表示不应写入配置文件的已知默认值则返回 true。
// 这防止非零默认值污染配置。
func isKnownDefaultValue(path []string, node *yaml.Node) bool {
	// 首先检查是否为零值
	if isZeroValueNode(node) {
		return true
	}

	// 通过精确的点分路径匹配已知的非零默认值。
	if len(path) == 0 {
		return false
	}

	fullPath := strings.Join(path, ".")

	// 检查字符串默认值
	if node.Kind == yaml.ScalarNode && node.Tag == "!!str" {
		switch fullPath {
		case "pprof.addr":
			return node.Value == DefaultPprofAddr
		case "remote-management.panel-github-repository":
			return node.Value == DefaultPanelGitHubRepository
		case "routing.strategy":
			return node.Value == "round-robin"
		}
	}

	// 检查整数默认值
	if node.Kind == yaml.ScalarNode && node.Tag == "!!int" {
		switch fullPath {
		case "error-logs-max-files":
			return node.Value == "10"
		}
	}

	return false
}

// pruneKnownDefaultsInNewNode 在新节点追加到目标 YAML 树之前移除默认值后代。
func pruneKnownDefaultsInNewNode(path []string, node *yaml.Node) {
	if node == nil {
		return
	}

	switch node.Kind {
	case yaml.MappingNode:
		filtered := make([]*yaml.Node, 0, len(node.Content))
		for i := 0; i+1 < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valueNode := node.Content[i+1]
			if keyNode == nil || valueNode == nil {
				continue
			}

			childPath := appendPath(path, keyNode.Value)
			if isKnownDefaultValue(childPath, valueNode) {
				continue
			}

			pruneKnownDefaultsInNewNode(childPath, valueNode)
			if (valueNode.Kind == yaml.MappingNode || valueNode.Kind == yaml.SequenceNode) &&
				len(valueNode.Content) == 0 {
				continue
			}

			filtered = append(filtered, keyNode, valueNode)
		}
		node.Content = filtered
	case yaml.SequenceNode:
		for _, child := range node.Content {
			pruneKnownDefaultsInNewNode(path, child)
		}
	}
}

// isZeroValueNode 如果 YAML 节点表示不应作为新键写入以保持配置整洁的零/默认值则返回 true。
// 对于映射和序列，递归检查所有子节点是否为零值。
func isZeroValueNode(node *yaml.Node) bool {
	if node == nil {
		return true
	}
	switch node.Kind {
	case yaml.ScalarNode:
		switch node.Tag {
		case "!!bool":
			return node.Value == "false"
		case "!!int", "!!float":
			return node.Value == "0" || node.Value == "0.0"
		case "!!str":
			return node.Value == ""
		case "!!null":
			return true
		}
	case yaml.SequenceNode:
		if len(node.Content) == 0 {
			return true
		}
		// 检查所有元素是否为零值
		for _, child := range node.Content {
			if !isZeroValueNode(child) {
				return false
			}
		}
		return true
	case yaml.MappingNode:
		if len(node.Content) == 0 {
			return true
		}
		// 检查所有值是否为零值（值位于奇数索引）
		for i := 1; i < len(node.Content); i += 2 {
			if !isZeroValueNode(node.Content[i]) {
				return false
			}
		}
		return true
	}
	return false
}

// deepCopyNode 创建 yaml.Node 图的深拷贝。
func deepCopyNode(n *yaml.Node) *yaml.Node {
	if n == nil {
		return nil
	}
	cp := *n
	if len(n.Content) > 0 {
		cp.Content = make([]*yaml.Node, len(n.Content))
		for i := range n.Content {
			cp.Content[i] = deepCopyNode(n.Content[i])
		}
	}
	return &cp
}

// copyNodeShallow 复制类型/标签/值并重置内容以匹配 src，但
// 保留相同的目标节点指针以保留父关系/注释。
func copyNodeShallow(dst, src *yaml.Node) {
	if dst == nil || src == nil {
		return
	}
	dst.Kind = src.Kind
	dst.Tag = src.Tag
	dst.Value = src.Value
	// 用 src 的深拷贝替换内容
	if len(src.Content) > 0 {
		dst.Content = make([]*yaml.Node, len(src.Content))
		for i := range src.Content {
			dst.Content[i] = deepCopyNode(src.Content[i])
		}
	} else {
		dst.Content = nil
	}
}

// reorderSequenceForMerge 为合并重新排序序列，匹配现有元素与源元素。
func reorderSequenceForMerge(dst, src *yaml.Node) {
	if dst == nil || src == nil {
		return
	}
	if len(dst.Content) == 0 {
		return
	}
	if len(src.Content) == 0 {
		return
	}
	original := append([]*yaml.Node(nil), dst.Content...)
	used := make([]bool, len(original))
	ordered := make([]*yaml.Node, len(src.Content))
	for i := range src.Content {
		if idx := matchSequenceElement(original, used, src.Content[i]); idx >= 0 {
			ordered[i] = original[idx]
			used[idx] = true
		}
	}
	dst.Content = ordered
}

// matchSequenceElement 在原始序列中匹配目标元素，返回匹配的索引。
func matchSequenceElement(original []*yaml.Node, used []bool, target *yaml.Node) int {
	if target == nil {
		return -1
	}
	switch target.Kind {
	case yaml.MappingNode:
		id := sequenceElementIdentity(target)
		if id != "" {
			for i := range original {
				if used[i] || original[i] == nil || original[i].Kind != yaml.MappingNode {
					continue
				}
				if sequenceElementIdentity(original[i]) == id {
					return i
				}
			}
		}
	case yaml.ScalarNode:
		val := strings.TrimSpace(target.Value)
		if val != "" {
			for i := range original {
				if used[i] || original[i] == nil || original[i].Kind != yaml.ScalarNode {
					continue
				}
				if strings.TrimSpace(original[i].Value) == val {
					return i
				}
			}
		}
	default:
	}
	// 回退到结构相等性以保留缺少显式标识符的节点。
	for i := range original {
		if used[i] || original[i] == nil {
			continue
		}
		if nodesStructurallyEqual(original[i], target) {
			return i
		}
	}
	return -1
}

// sequenceElementIdentity 返回序列元素的标识字符串，用于匹配。
func sequenceElementIdentity(node *yaml.Node) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}
	identityKeys := []string{"id", "name", "alias", "api-key", "api_key", "apikey", "key", "provider", "model"}
	for _, k := range identityKeys {
		if v := mappingScalarValue(node, k); v != "" {
			return k + "=" + v
		}
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		if keyNode == nil || valNode == nil || valNode.Kind != yaml.ScalarNode {
			continue
		}
		val := strings.TrimSpace(valNode.Value)
		if val != "" {
			return strings.ToLower(strings.TrimSpace(keyNode.Value)) + "=" + val
		}
	}
	return ""
}

// mappingScalarValue 从映射节点中获取指定键的标量值。
func mappingScalarValue(node *yaml.Node, key string) string {
	if node == nil || node.Kind != yaml.MappingNode {
		return ""
	}
	lowerKey := strings.ToLower(key)
	for i := 0; i+1 < len(node.Content); i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]
		if keyNode == nil || valNode == nil || valNode.Kind != yaml.ScalarNode {
			continue
		}
		if strings.ToLower(strings.TrimSpace(keyNode.Value)) == lowerKey {
			return strings.TrimSpace(valNode.Value)
		}
	}
	return ""
}

// nodesStructurallyEqual 检查两个 YAML 节点是否结构相等。
func nodesStructurallyEqual(a, b *yaml.Node) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case yaml.MappingNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := 0; i+1 < len(a.Content); i += 2 {
			if !nodesStructurallyEqual(a.Content[i], b.Content[i]) {
				return false
			}
			if !nodesStructurallyEqual(a.Content[i+1], b.Content[i+1]) {
				return false
			}
		}
		return true
	case yaml.SequenceNode:
		if len(a.Content) != len(b.Content) {
			return false
		}
		for i := range a.Content {
			if !nodesStructurallyEqual(a.Content[i], b.Content[i]) {
				return false
			}
		}
		return true
	case yaml.ScalarNode:
		return strings.TrimSpace(a.Value) == strings.TrimSpace(b.Value)
	case yaml.AliasNode:
		return nodesStructurallyEqual(a.Alias, b.Alias)
	default:
		return strings.TrimSpace(a.Value) == strings.TrimSpace(b.Value)
	}
}

// removeMapKey 从映射节点中移除指定键的键值对。
func removeMapKey(mapNode *yaml.Node, key string) {
	if mapNode == nil || mapNode.Kind != yaml.MappingNode || key == "" {
		return
	}
	for i := 0; i+1 < len(mapNode.Content); i += 2 {
		if mapNode.Content[i] != nil && mapNode.Content[i].Value == key {
			mapNode.Content = append(mapNode.Content[:i], mapNode.Content[i+2:]...)
			return
		}
	}
}

// pruneMappingToGeneratedKeys 修剪映射以仅保留生成的键。
func pruneMappingToGeneratedKeys(dstRoot, srcRoot *yaml.Node, key string) {
	if key == "" || dstRoot == nil || srcRoot == nil {
		return
	}
	if dstRoot.Kind != yaml.MappingNode || srcRoot.Kind != yaml.MappingNode {
		return
	}
	dstIdx := findMapKeyIndex(dstRoot, key)
	if dstIdx < 0 || dstIdx+1 >= len(dstRoot.Content) {
		return
	}
	srcIdx := findMapKeyIndex(srcRoot, key)
	if srcIdx < 0 {
		// 当之前存在 oauth-model-alias 时，为其保留显式空映射。
		// 当用户通过管理 API 删除 oauth-model-alias 中的最后一个渠道时，
		// 我们希望该删除在热重载和重启之间持续存在。
		if key == "oauth-model-alias" {
			dstRoot.Content[dstIdx+1] = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			return
		}
		removeMapKey(dstRoot, key)
		return
	}
	if srcIdx+1 >= len(srcRoot.Content) {
		return
	}
	srcVal := srcRoot.Content[srcIdx+1]
	dstVal := dstRoot.Content[dstIdx+1]
	if srcVal == nil {
		dstRoot.Content[dstIdx+1] = nil
		return
	}
	if srcVal.Kind != yaml.MappingNode {
		dstRoot.Content[dstIdx+1] = deepCopyNode(srcVal)
		return
	}
	if dstVal == nil || dstVal.Kind != yaml.MappingNode {
		dstRoot.Content[dstIdx+1] = deepCopyNode(srcVal)
		return
	}
	pruneMissingMapKeys(dstVal, srcVal)
}

// pruneMissingMapKeys 从目标映射中移除源映射中不存在的键。
func pruneMissingMapKeys(dstMap, srcMap *yaml.Node) {
	if dstMap == nil || srcMap == nil || dstMap.Kind != yaml.MappingNode || srcMap.Kind != yaml.MappingNode {
		return
	}
	keep := make(map[string]struct{}, len(srcMap.Content)/2)
	for i := 0; i+1 < len(srcMap.Content); i += 2 {
		keyNode := srcMap.Content[i]
		if keyNode == nil {
			continue
		}
		key := strings.TrimSpace(keyNode.Value)
		if key == "" {
			continue
		}
		keep[key] = struct{}{}
	}
	for i := 0; i+1 < len(dstMap.Content); {
		keyNode := dstMap.Content[i]
		if keyNode == nil {
			i += 2
			continue
		}
		key := strings.TrimSpace(keyNode.Value)
		if _, ok := keep[key]; !ok {
			dstMap.Content = append(dstMap.Content[:i], dstMap.Content[i+2:]...)
			continue
		}
		i += 2
	}
}

// normalizeCollectionNodeStyles 强制 YAML 集合使用块表示法，保持
// 列表和映射可读。空序列保留流样式（[]），使空列表标记保持紧凑。
func normalizeCollectionNodeStyles(node *yaml.Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.MappingNode:
		node.Style = 0
		for i := range node.Content {
			normalizeCollectionNodeStyles(node.Content[i])
		}
	case yaml.SequenceNode:
		if len(node.Content) == 0 {
			node.Style = yaml.FlowStyle
		} else {
			node.Style = 0
		}
		for i := range node.Content {
			normalizeCollectionNodeStyles(node.Content[i])
		}
	default:
		// 标量保留其现有样式以保留引号
	}
}

// 旧版迁移辅助函数（将已弃用的配置键移入结构化字段）。
type legacyConfigData struct {
	LegacyGeminiKeys      []string                    `yaml:"generative-language-api-key"`
	OpenAICompat          []legacyOpenAICompatibility `yaml:"openai-compatibility"`
	AmpUpstreamURL        string                      `yaml:"amp-upstream-url"`
	AmpUpstreamAPIKey     string                      `yaml:"amp-upstream-api-key"`
	AmpRestrictManagement *bool                       `yaml:"amp-restrict-management-to-localhost"`
	AmpModelMappings      []AmpModelMapping           `yaml:"amp-model-mappings"`
}

// legacyOpenAICompatibility 表示旧版 OpenAI 兼容配置。
type legacyOpenAICompatibility struct {
	Name    string   `yaml:"name"`
	BaseURL string   `yaml:"base-url"`
	APIKeys []string `yaml:"api-keys"`
}

// migrateLegacyGeminiKeys 迁移旧版 Gemini API 密钥到新格式。
func (cfg *Config) migrateLegacyGeminiKeys(legacy []string) bool {
	if cfg == nil || len(legacy) == 0 {
		return false
	}
	changed := false
	seen := make(map[string]struct{}, len(cfg.GeminiKey))
	for i := range cfg.GeminiKey {
		key := strings.TrimSpace(cfg.GeminiKey[i].APIKey)
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	for _, raw := range legacy {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		cfg.GeminiKey = append(cfg.GeminiKey, GeminiKey{APIKey: key})
		seen[key] = struct{}{}
		changed = true
	}
	return changed
}

// migrateLegacyOpenAICompatibilityKeys 迁移旧版 OpenAI 兼容密钥到新格式。
func (cfg *Config) migrateLegacyOpenAICompatibilityKeys(legacy []legacyOpenAICompatibility) bool {
	if cfg == nil || len(cfg.OpenAICompatibility) == 0 || len(legacy) == 0 {
		return false
	}
	changed := false
	for _, legacyEntry := range legacy {
		if len(legacyEntry.APIKeys) == 0 {
			continue
		}
		target := findOpenAICompatTarget(cfg.OpenAICompatibility, legacyEntry.Name, legacyEntry.BaseURL)
		if target == nil {
			continue
		}
		if mergeLegacyOpenAICompatAPIKeys(target, legacyEntry.APIKeys) {
			changed = true
		}
	}
	return changed
}

// mergeLegacyOpenAICompatAPIKeys 合并旧版 OpenAI 兼容 API 密钥。
func mergeLegacyOpenAICompatAPIKeys(entry *OpenAICompatibility, keys []string) bool {
	if entry == nil || len(keys) == 0 {
		return false
	}
	changed := false
	existing := make(map[string]struct{}, len(entry.APIKeyEntries))
	for i := range entry.APIKeyEntries {
		key := strings.TrimSpace(entry.APIKeyEntries[i].APIKey)
		if key == "" {
			continue
		}
		existing[key] = struct{}{}
	}
	for _, raw := range keys {
		key := strings.TrimSpace(raw)
		if key == "" {
			continue
		}
		if _, ok := existing[key]; ok {
			continue
		}
		entry.APIKeyEntries = append(entry.APIKeyEntries, OpenAICompatibilityAPIKey{APIKey: key})
		existing[key] = struct{}{}
		changed = true
	}
	return changed
}

// findOpenAICompatTarget 在 OpenAI 兼容条目中查找目标条目。
func findOpenAICompatTarget(entries []OpenAICompatibility, legacyName, legacyBase string) *OpenAICompatibility {
	nameKey := strings.ToLower(strings.TrimSpace(legacyName))
	baseKey := strings.ToLower(strings.TrimSpace(legacyBase))
	if nameKey != "" && baseKey != "" {
		for i := range entries {
			if strings.ToLower(strings.TrimSpace(entries[i].Name)) == nameKey &&
				strings.ToLower(strings.TrimSpace(entries[i].BaseURL)) == baseKey {
				return &entries[i]
			}
		}
	}
	if baseKey != "" {
		for i := range entries {
			if strings.ToLower(strings.TrimSpace(entries[i].BaseURL)) == baseKey {
				return &entries[i]
			}
		}
	}
	if nameKey != "" {
		for i := range entries {
			if strings.ToLower(strings.TrimSpace(entries[i].Name)) == nameKey {
				return &entries[i]
			}
		}
	}
	return nil
}

// migrateLegacyAmpConfig 迁移旧版 Amp 配置到新格式。
func (cfg *Config) migrateLegacyAmpConfig(legacy *legacyConfigData) bool {
	if cfg == nil || legacy == nil {
		return false
	}
	changed := false
	if cfg.AmpCode.UpstreamURL == "" {
		if val := strings.TrimSpace(legacy.AmpUpstreamURL); val != "" {
			cfg.AmpCode.UpstreamURL = val
			changed = true
		}
	}
	if cfg.AmpCode.UpstreamAPIKey == "" {
		if val := strings.TrimSpace(legacy.AmpUpstreamAPIKey); val != "" {
			cfg.AmpCode.UpstreamAPIKey = val
			changed = true
		}
	}
	if legacy.AmpRestrictManagement != nil {
		cfg.AmpCode.RestrictManagementToLocalhost = *legacy.AmpRestrictManagement
		changed = true
	}
	if len(cfg.AmpCode.ModelMappings) == 0 && len(legacy.AmpModelMappings) > 0 {
		cfg.AmpCode.ModelMappings = append([]AmpModelMapping(nil), legacy.AmpModelMappings...)
		changed = true
	}
	return changed
}

// removeLegacyOpenAICompatAPIKeys 从 YAML 树中移除旧版 OpenAI 兼容 API 密钥。
func removeLegacyOpenAICompatAPIKeys(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	idx := findMapKeyIndex(root, "openai-compatibility")
	if idx < 0 || idx+1 >= len(root.Content) {
		return
	}
	seq := root.Content[idx+1]
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return
	}
	for i := range seq.Content {
		if seq.Content[i] != nil && seq.Content[i].Kind == yaml.MappingNode {
			removeMapKey(seq.Content[i], "api-keys")
		}
	}
}

// removeLegacyAmpKeys 从 YAML 树中移除旧版 Amp 配置键。
func removeLegacyAmpKeys(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	removeMapKey(root, "amp-upstream-url")
	removeMapKey(root, "amp-upstream-api-key")
	removeMapKey(root, "amp-restrict-management-to-localhost")
	removeMapKey(root, "amp-model-mappings")
}

// removeLegacyGenerativeLanguageKeys 从 YAML 树中移除旧版 Generative Language API 密钥。
func removeLegacyGenerativeLanguageKeys(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	removeMapKey(root, "generative-language-api-key")
}

// removeLegacyAuthBlock 从 YAML 树中移除旧版 auth 配置块。
func removeLegacyAuthBlock(root *yaml.Node) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	removeMapKey(root, "auth")
}
