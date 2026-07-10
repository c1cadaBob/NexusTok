package dto

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	// AdvancedCustomConverterNone 表示不做协议转换，按请求原格式转发到配置的上游路径。
	AdvancedCustomConverterNone = "none"
	// AdvancedCustomConverterAnthropicMessagesToOpenAIChatCompletions 将 Claude Messages 请求转换为 OpenAI Chat Completions。
	AdvancedCustomConverterAnthropicMessagesToOpenAIChatCompletions = "anthropic_messages_to_openai_chat_completions"
	// AdvancedCustomConverterOpenAIChatCompletionsToAnthropicMessages 将 OpenAI Chat Completions 请求转换为 Claude Messages。
	AdvancedCustomConverterOpenAIChatCompletionsToAnthropicMessages = "openai_chat_completions_to_anthropic_messages"
	// AdvancedCustomConverterOpenAIChatCompletionsToOpenAIResponses 将 OpenAI Chat Completions 请求转换为 OpenAI Responses。
	AdvancedCustomConverterOpenAIChatCompletionsToOpenAIResponses = "openai_chat_completions_to_openai_responses"
	// AdvancedCustomConverterOpenAIResponsesToOpenAIChatCompletions 将 OpenAI Responses 请求转换为 OpenAI Chat Completions。
	AdvancedCustomConverterOpenAIResponsesToOpenAIChatCompletions = "openai_responses_to_openai_chat_completions"
	// AdvancedCustomConverterGeminiGenerateContentToOpenAIChatCompletions 将 Gemini GenerateContent 请求转换为 OpenAI Chat Completions。
	AdvancedCustomConverterGeminiGenerateContentToOpenAIChatCompletions = "gemini_generate_content_to_openai_chat_completions"
	// AdvancedCustomConverterOpenAIChatCompletionsToGeminiGenerateContent 将 OpenAI Chat Completions 请求转换为 Gemini GenerateContent。
	AdvancedCustomConverterOpenAIChatCompletionsToGeminiGenerateContent = "openai_chat_completions_to_gemini_generate_content"
)

const (
	// AdvancedCustomAuthTypeNone 表示该 route 不自动注入渠道密钥。
	AdvancedCustomAuthTypeNone = "none"
	// AdvancedCustomAuthTypeHeader 表示将渠道密钥按模板写入指定请求头。
	AdvancedCustomAuthTypeHeader = "header"
	// AdvancedCustomAuthTypeQuery 表示将渠道密钥按模板写入指定查询参数。
	AdvancedCustomAuthTypeQuery = "query"
)

const advancedCustomModelPlaceholder = "{model}"

// AdvancedCustomConfig 定义高级自定义渠道的路由集合。
//
// 每个 route 描述一个客户端入口路径、一个上游目标路径、可选协议转换器以及认证注入方式。
// 该结构存储在渠道 settings JSON 的 advanced_custom 字段中，因此必须在保存和 relay 前都校验。
type AdvancedCustomConfig struct {
	Routes []AdvancedCustomRoute `json:"advanced_routes,omitempty"`
}

// AdvancedCustomRoute 定义单条高级路由。
type AdvancedCustomRoute struct {
	IncomingPath string                   `json:"incoming_path,omitempty"`
	UpstreamPath string                   `json:"upstream_path,omitempty"`
	Converter    string                   `json:"converter,omitempty"`
	Auth         *AdvancedCustomRouteAuth `json:"auth,omitempty"`
}

// AdvancedCustomRouteAuth 定义单条路由的认证注入方式。
type AdvancedCustomRouteAuth struct {
	Type  string `json:"type,omitempty"`
	Name  string `json:"name,omitempty"`
	Value string `json:"value,omitempty"`
}

// MatchPath 返回第一条匹配 requestPath 的 route。
//
// 匹配规则与 relay adaptor 保持一致：支持精确路径、单路径段 {model} 占位符，以及
// Gemini generateContent 与 streamGenerateContent 的等价匹配。requestPath 必须是不含 query
// 的 URL path，调用方应在进入本函数前去掉查询串。
func (c *AdvancedCustomConfig) MatchPath(requestPath string) (AdvancedCustomRoute, bool) {
	if c == nil {
		return AdvancedCustomRoute{}, false
	}
	for _, route := range c.Routes {
		if matchAdvancedCustomIncomingPath(strings.TrimSpace(route.IncomingPath), requestPath) {
			return route, true
		}
	}
	return AdvancedCustomRoute{}, false
}

// SupportsPath 判断配置中是否存在能处理 requestPath 的 route。
func (c *AdvancedCustomConfig) SupportsPath(requestPath string) bool {
	_, ok := c.MatchPath(requestPath)
	return ok
}

func matchAdvancedCustomIncomingPath(configuredPath string, requestPath string) bool {
	if matchAdvancedCustomIncomingPathTemplate(configuredPath, requestPath) {
		return true
	}
	if strings.Contains(configuredPath, ":generateContent") {
		streamPath := strings.Replace(configuredPath, ":generateContent", ":streamGenerateContent", 1)
		return matchAdvancedCustomIncomingPathTemplate(streamPath, requestPath)
	}
	return false
}

func matchAdvancedCustomIncomingPathTemplate(configuredPath string, requestPath string) bool {
	if !strings.Contains(configuredPath, advancedCustomModelPlaceholder) {
		return configuredPath == requestPath
	}

	parts := strings.Split(configuredPath, advancedCustomModelPlaceholder)
	if len(parts) != 2 {
		return false
	}
	if !strings.HasPrefix(requestPath, parts[0]) || !strings.HasSuffix(requestPath, parts[1]) {
		return false
	}

	model := strings.TrimSuffix(strings.TrimPrefix(requestPath, parts[0]), parts[1])
	return model != "" && !strings.Contains(model, "/")
}

// IsAdvancedCustomConverterAllowed 判断 converter 是否在后端注册表内。
func IsAdvancedCustomConverterAllowed(converter string) bool {
	switch converter {
	case AdvancedCustomConverterNone,
		AdvancedCustomConverterAnthropicMessagesToOpenAIChatCompletions,
		AdvancedCustomConverterOpenAIChatCompletionsToAnthropicMessages,
		AdvancedCustomConverterOpenAIChatCompletionsToOpenAIResponses,
		AdvancedCustomConverterOpenAIResponsesToOpenAIChatCompletions,
		AdvancedCustomConverterGeminiGenerateContentToOpenAIChatCompletions,
		AdvancedCustomConverterOpenAIChatCompletionsToGeminiGenerateContent:
		return true
	default:
		return false
	}
}

// Validate 校验高级自定义渠道配置。
//
// 不变量：
//   - 至少存在一条 route；
//   - incoming_path 必须是无 query 的绝对路径，且同一配置内唯一；
//   - upstream_path 必须是 http/https 绝对 URL，或以单个 "/" 开头的相对路径；
//   - converter 必须已注册，并且只能用于与自身协议匹配的入口路径；
//   - header/query 认证必须有非空 name/value，且 name 不能包含换行等危险字符。
func (c *AdvancedCustomConfig) Validate() error {
	if c == nil {
		return fmt.Errorf("advanced_custom is required")
	}
	if len(c.Routes) == 0 {
		return fmt.Errorf("advanced_custom requires at least one route")
	}

	seenPaths := make(map[string]struct{}, len(c.Routes))
	for i := range c.Routes {
		route := &c.Routes[i]
		route.IncomingPath = strings.TrimSpace(route.IncomingPath)
		route.UpstreamPath = strings.TrimSpace(route.UpstreamPath)
		route.Converter = strings.TrimSpace(route.Converter)
		if route.Converter == "" {
			route.Converter = AdvancedCustomConverterNone
		}

		if route.IncomingPath == "" {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].incoming_path is required", i)
		}
		if !strings.HasPrefix(route.IncomingPath, "/") {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].incoming_path must start with /", i)
		}
		if strings.Contains(route.IncomingPath, "?") {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].incoming_path must not include query", i)
		}
		if _, exists := seenPaths[route.IncomingPath]; exists {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].incoming_path must be unique: %s", i, route.IncomingPath)
		}
		seenPaths[route.IncomingPath] = struct{}{}

		if route.UpstreamPath == "" {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].upstream_path is required", i)
		}
		if err := validateAdvancedCustomUpstreamTarget(i, route.UpstreamPath); err != nil {
			return err
		}

		if !IsAdvancedCustomConverterAllowed(route.Converter) {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].converter is not registered: %s", i, route.Converter)
		}
		if err := validateAdvancedCustomConverterPath(i, route.IncomingPath, route.Converter); err != nil {
			return err
		}
		if err := validateAdvancedCustomRouteAuth(i, route.Auth); err != nil {
			return err
		}
	}

	return nil
}

func validateAdvancedCustomUpstreamTarget(index int, upstreamPath string) error {
	if strings.HasPrefix(upstreamPath, "/") {
		if strings.HasPrefix(upstreamPath, "//") {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].upstream_path must be a full URL or a path starting with /", index)
		}
		return nil
	}

	parsedURL, err := url.Parse(upstreamPath)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return fmt.Errorf("advanced_custom.advanced_routes[%d].upstream_path must be a full URL or a path starting with /", index)
	}
	if parsedURL.User != nil {
		return fmt.Errorf("advanced_custom.advanced_routes[%d].upstream_path must not include user info", index)
	}
	if !strings.EqualFold(parsedURL.Scheme, "http") && !strings.EqualFold(parsedURL.Scheme, "https") {
		return fmt.Errorf("advanced_custom.advanced_routes[%d].upstream_path must use http or https", index)
	}
	return nil
}

func validateAdvancedCustomConverterPath(index int, incomingPath string, converter string) error {
	switch converter {
	case AdvancedCustomConverterNone:
		return nil
	case AdvancedCustomConverterAnthropicMessagesToOpenAIChatCompletions:
		if incomingPath == "/v1/messages" {
			return nil
		}
	case AdvancedCustomConverterOpenAIChatCompletionsToAnthropicMessages,
		AdvancedCustomConverterOpenAIChatCompletionsToOpenAIResponses,
		AdvancedCustomConverterOpenAIChatCompletionsToGeminiGenerateContent:
		if incomingPath == "/v1/chat/completions" {
			return nil
		}
	case AdvancedCustomConverterOpenAIResponsesToOpenAIChatCompletions:
		if incomingPath == "/v1/responses" {
			return nil
		}
	case AdvancedCustomConverterGeminiGenerateContentToOpenAIChatCompletions:
		if strings.Contains(incomingPath, ":generateContent") || strings.Contains(incomingPath, ":streamGenerateContent") {
			return nil
		}
	}
	return fmt.Errorf("advanced_custom.advanced_routes[%d].converter does not match incoming_path: %s", index, converter)
}

func validateAdvancedCustomRouteAuth(index int, auth *AdvancedCustomRouteAuth) error {
	if auth == nil {
		return nil
	}
	auth.Type = strings.TrimSpace(auth.Type)
	auth.Name = strings.TrimSpace(auth.Name)
	auth.Value = strings.TrimSpace(auth.Value)

	switch auth.Type {
	case AdvancedCustomAuthTypeNone:
		return nil
	case AdvancedCustomAuthTypeHeader:
		if auth.Name == "" {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].auth.name is required", index)
		}
		if !isSafeAdvancedCustomHeaderName(auth.Name) {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].auth.name is not a valid header name", index)
		}
		if auth.Value == "" {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].auth.value is required", index)
		}
		return nil
	case AdvancedCustomAuthTypeQuery:
		if auth.Name == "" {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].auth.name is required", index)
		}
		if strings.ContainsAny(auth.Name, "\r\n\t ") {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].auth.name is not a valid query name", index)
		}
		if auth.Value == "" {
			return fmt.Errorf("advanced_custom.advanced_routes[%d].auth.value is required", index)
		}
		return nil
	default:
		return fmt.Errorf("advanced_custom.advanced_routes[%d].auth.type is invalid: %s", index, auth.Type)
	}
}

func isSafeAdvancedCustomHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r <= 32 || r >= 127 {
			return false
		}
		switch r {
		case '(', ')', '<', '>', '@', ',', ';', ':', '\\', '"', '/', '[', ']', '?', '=', '{', '}':
			return false
		}
	}
	return true
}
