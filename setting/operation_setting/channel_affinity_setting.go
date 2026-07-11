// channel_affinity_setting.go — 渠道亲和性（Channel Affinity）配置
// 职责：管理请求到渠道的亲和性绑定规则。当同一用户/会话的多次请求
// 到达时，亲和性规则可以确保请求被路由到同一上游渠道，以保持会话一致性。
// 支持基于请求体内容（gjson 路径）、上下文键等来源提取亲和性键，
// 并可配置 TTL、最大条目数、参数覆盖模板等。

package operation_setting

import "github.com/c1cada/NexusTok/setting/config"

// ChannelAffinityKeySource 定义亲和性键的提取来源
type ChannelAffinityKeySource struct {
	// Type 键来源类型，可选值：
	//   "context_int"    - 从请求上下文中获取整型键
	//   "context_string" - 从请求上下文中获取字符串键
	//   "request_header" - 从 HTTP 请求头中获取字符串键
	//   "gjson"          - 使用 gjson 路径从请求体 JSON 中提取
	Type string `json:"type"` // context_int, context_string, request_header, gjson
	// Key 上下文键名（当 Type 为 context_int/context_string 时使用）
	Key string `json:"key,omitempty"`
	// Path gjson 路径表达式（当 Type 为 gjson 时使用）
	Path string `json:"path,omitempty"`
}

// ChannelAffinityRule 定义一条亲和性匹配规则
type ChannelAffinityRule struct {
	// Name 规则的唯一名称，用于日志和调试
	Name string `json:"name"`
	// ModelRegex 模型名匹配的正则表达式列表（满足任一即匹配）
	ModelRegex []string `json:"model_regex"`
	// PathRegex 请求路径匹配的正则表达式列表（满足任一即匹配）
	PathRegex []string `json:"path_regex"`
	// UserAgentInclude User-Agent 包含过滤列表（为空则不限制）
	UserAgentInclude []string `json:"user_agent_include,omitempty"`
	// KeySources 亲和性键的提取来源列表，多来源的键会拼接使用
	KeySources []ChannelAffinityKeySource `json:"key_sources"`

	// ValueRegex 对提取的亲和性值进行正则校验（为空则不校验）
	ValueRegex string `json:"value_regex"`
	// TTLSeconds 亲和性绑定的过期时间（秒），0 表示使用默认 TTL
	TTLSeconds int `json:"ttl_seconds"`

	// ParamOverrideTemplate 参数覆盖模板，匹配时可覆盖请求参数
	ParamOverrideTemplate map[string]interface{} `json:"param_override_template,omitempty"`

	// SkipRetryOnFailure 标记该规则是否在失败时跳过重试
	SkipRetryOnFailure bool `json:"skip_retry_on_failure"`

	// IncludeUsingGroup 是否在亲和性键中包含当前使用的分组信息
	IncludeUsingGroup bool `json:"include_using_group"`
	// IncludeModelName 是否在亲和性键中包含模型名称
	IncludeModelName bool `json:"include_model_name"`
	// IncludeRuleName 是否在亲和性键中包含规则名称
	IncludeRuleName bool `json:"include_rule_name"`
}

// ChannelAffinitySetting 渠道亲和性全局配置
type ChannelAffinitySetting struct {
	// Enabled 控制是否启用渠道亲和性功能
	Enabled bool `json:"enabled"`
	// SwitchOnSuccess 控制是否仅在请求成功时更新亲和性绑定
	SwitchOnSuccess bool `json:"switch_on_success"`
	// KeepOnChannelDisabled 控制亲和命中的渠道不可用时是否保留旧缓存。
	// 默认 false：当亲和渠道已禁用、已删除或不再支持当前分组/模型时，清理本次命中的缓存，
	// 让后续请求重新选择健康渠道。开启后保留旧缓存，适用于管理员希望等待渠道恢复的场景。
	KeepOnChannelDisabled bool `json:"keep_on_channel_disabled"`
	// MaxEntries 亲和性缓存的最大条目数
	MaxEntries int `json:"max_entries"`
	// DefaultTTLSeconds 默认的亲和性绑定过期时间（秒）
	DefaultTTLSeconds int `json:"default_ttl_seconds"`
	// Rules 亲和性规则列表
	Rules []ChannelAffinityRule `json:"rules"`
}

// codexCliPassThroughHeaders Codex CLI 需要透传的 HTTP Header 列表
var codexCliPassThroughHeaders = []string{
	"Originator",
	"Session_id",
	"User-Agent",
	"X-Codex-Beta-Features",
	"X-Codex-Turn-Metadata",
}

// claudeCliPassThroughHeaders Claude CLI 需要透传的 HTTP Header 列表
var claudeCliPassThroughHeaders = []string{
	"X-Stainless-Arch",
	"X-Stainless-Lang",
	"X-Stainless-Os",
	"X-Stainless-Package-Version",
	"X-Stainless-Retry-Count",
	"X-Stainless-Runtime",
	"X-Stainless-Runtime-Version",
	"X-Stainless-Timeout",
	"User-Agent",
	"X-App",
	"Anthropic-Beta",
	"Anthropic-Dangerous-Direct-Browser-Access",
	"Anthropic-Version",
}

// buildPassHeaderTemplate 构建透传 Header 的参数覆盖模板
// 参数：
//   - headers: 需要透传的 Header 名称列表
//
// 返回值：包含 pass_headers 操作的模板 Map
func buildPassHeaderTemplate(headers []string) map[string]interface{} {
	// 克隆 Header 列表，避免修改原始数据
	clonedHeaders := make([]string, 0, len(headers))
	clonedHeaders = append(clonedHeaders, headers...)
	return map[string]interface{}{
		"operations": []map[string]interface{}{
			{
				"mode":        "pass_headers",
				"value":       clonedHeaders,
				"keep_origin": true,
			},
		},
	}
}

// channelAffinitySetting 是全局渠道亲和性配置实例，默认包含 Codex CLI 和 Claude CLI 两条规则
var channelAffinitySetting = ChannelAffinitySetting{
	Enabled:               true,    // 默认启用
	SwitchOnSuccess:       true,    // 仅在请求成功时更新绑定
	KeepOnChannelDisabled: false,   // 亲和渠道不可用时默认清理缓存，避免反复命中过期渠道
	MaxEntries:            100_000, // 最大缓存 10 万条
	DefaultTTLSeconds:     3600,    // 默认 TTL 1 小时
	Rules: []ChannelAffinityRule{
		{
			// Codex CLI 追踪规则：匹配所有 gpt-* 模型的 /v1/responses 请求
			Name:       "codex cli trace",
			ModelRegex: []string{"^gpt-.*$"},
			PathRegex:  []string{"/v1/responses"},
			KeySources: []ChannelAffinityKeySource{
				{Type: "gjson", Path: "prompt_cache_key"},
			},
			ValueRegex:            "",
			TTLSeconds:            0, // 使用默认 TTL
			ParamOverrideTemplate: buildPassHeaderTemplate(codexCliPassThroughHeaders),
			SkipRetryOnFailure:    true,
			IncludeUsingGroup:     true,
			IncludeRuleName:       true,
			UserAgentInclude:      nil,
		},
		{
			// Claude CLI 追踪规则：匹配所有 claude-* 模型的 /v1/messages 请求
			Name:       "claude cli trace",
			ModelRegex: []string{"^claude-.*$"},
			PathRegex:  []string{"/v1/messages"},
			KeySources: []ChannelAffinityKeySource{
				{Type: "gjson", Path: "metadata.user_id"},
			},
			ValueRegex:            "",
			TTLSeconds:            0, // 使用默认 TTL
			ParamOverrideTemplate: buildPassHeaderTemplate(claudeCliPassThroughHeaders),
			SkipRetryOnFailure:    true,
			IncludeUsingGroup:     true,
			IncludeRuleName:       true,
			UserAgentInclude:      nil,
		},
	},
}

// init 注册渠道亲和性配置到全局配置管理系统
func init() {
	config.GlobalConfig.Register("channel_affinity_setting", &channelAffinitySetting)
}

// GetChannelAffinitySetting 获取当前渠道亲和性配置的指针
// 返回值：指向当前配置的指针
func GetChannelAffinitySetting() *ChannelAffinitySetting {
	return &channelAffinitySetting
}
