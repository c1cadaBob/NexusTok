// Package oauth - types.go
// 该文件定义了 OAuth 认证过程中使用的数据类型
//
// 主要类型：
// - OAuthToken：OAuth 令牌（访问令牌、刷新令牌等）
// - OAuthUser：OAuth 用户信息（用户 ID、用户名、邮箱等）
// - OAuthError：可国际化的 OAuth 错误
// - AccessDeniedError：访问拒绝错误（直接面向用户）
//
// 错误处理：
// - OAuthError 支持 i18n 消息键和参数
// - AccessDeniedError 用于访问策略拒绝场景
package oauth

// OAuthToken 表示从 OAuth 提供商获取的令牌响应
// 包含访问令牌、刷新令牌等信息
// 不同提供商返回的字段可能不同，使用 omitempty 标记可选字段
type OAuthToken struct {
	AccessToken  string `json:"access_token"`              // 访问令牌，用于调用提供商 API
	TokenType    string `json:"token_type"`                // 令牌类型，通常为 "Bearer"
	RefreshToken string `json:"refresh_token,omitempty"`   // 刷新令牌，用于在访问令牌过期后获取新的访问令牌
	ExpiresIn    int    `json:"expires_in,omitempty"`      // 访问令牌的过期时间（秒）
	Scope        string `json:"scope,omitempty"`           // 授权范围，表示令牌可访问的资源
	IDToken      string `json:"id_token,omitempty"`        // OIDC ID 令牌（JWT 格式），包含用户身份信息
}

// OAuthUser 表示从 OAuth 提供商获取的用户信息
// 所有提供商的用户信息都会被统一映射到此结构体
type OAuthUser struct {
	// ProviderUserID 是提供商侧的用户唯一标识
	// 对于 GitHub 是数字 ID（如 "12345678"），对于 Discord 是雪花 ID
	ProviderUserID string
	// Username 是提供商的用户名（如 GitHub 的 login 字段）
	// 用于自动生成系统用户名或展示
	Username string
	// DisplayName 是提供商的显示名称（用户的昵称/全名）
	DisplayName string
	// Email 是提供商返回的邮箱地址（部分提供商可能不返回或用户未公开）
	Email string
	// Extra 存储提供商特有的额外数据
	// 例如 LinuxDo 的 trust_level、active、silenced 等字段
	Extra map[string]any
}

// OAuthError 表示可国际化的 OAuth 错误
// 支持 i18n 消息键和模板参数，前端可根据用户语言偏好显示对应翻译
// 同时保留原始错误信息用于日志记录和调试
type OAuthError struct {
	// MsgKey 是 i18n 消息键（如 "oauth.invalid_code"）
	// 对应 i18n/keys.go 中定义的常量
	MsgKey string
	// Params 包含消息模板的可选参数（如 {"Provider": "GitHub"}）
	Params map[string]any
	// RawError 是底层原始错误信息，仅用于日志记录，不面向用户展示
	RawError string
}

// Error 实现 error 接口
// 优先返回原始错误信息（便于日志调试），如果没有则返回消息键
func (e *OAuthError) Error() string {
	if e.RawError != "" {
		return e.RawError
	}
	return e.MsgKey
}

// NewOAuthError 创建一个新的 OAuth 错误（不包含原始错误信息）
// 适用于不需要记录底层错误的场景，如参数校验失败
// 参数：
//   - msgKey：i18n 消息键
//   - params：消息模板参数（可为 nil）
func NewOAuthError(msgKey string, params map[string]any) *OAuthError {
	return &OAuthError{
		MsgKey: msgKey,
		Params: params,
	}
}

// NewOAuthErrorWithRaw 创建一个包含原始错误信息的 OAuth 错误
// 适用于需要记录底层错误的场景，如网络请求失败
// 原始错误信息会写入日志，但不会直接展示给用户
// 参数：
//   - msgKey：i18n 消息键
//   - params：消息模板参数（可为 nil）
//   - rawError：原始错误信息字符串（用于日志记录）
func NewOAuthErrorWithRaw(msgKey string, params map[string]any, rawError string) *OAuthError {
	return &OAuthError{
		MsgKey:   msgKey,
		Params:   params,
		RawError: rawError,
	}
}

// AccessDeniedError 表示访问策略拒绝错误
// 当用户的 OAuth 账号不满足自定义提供商的访问策略（Access Policy）时抛出
// 与 OAuthError 不同，此错误的消息直接面向用户，无需 i18n 翻译
type AccessDeniedError struct {
	// Message 是直接面向用户的拒绝消息
	// 可包含模板渲染后的内容（如具体的策略条件）
	Message string
}

// Error 实现 error 接口，返回面向用户的拒绝消息
func (e *AccessDeniedError) Error() string {
	return e.Message
}
