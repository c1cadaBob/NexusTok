// xai - types.go
// 包 xai 提供 xAI Grok 的 OAuth2 认证辅助功能。
// 该文件定义了 xAI OAuth 认证流程中使用的核心数据类型，
// 包括常量、PKCE 代码、发现结果、令牌数据和认证包等。
package xai

import "time"

const (
	// DefaultAPIBaseURL 是默认的 xAI Responses API 基础 URL
	DefaultAPIBaseURL = "https://api.x.ai/v1"
	// Issuer 是 xAI 的 OAuth 发行者
	Issuer = "https://auth.x.ai"
	// DiscoveryURL 是用于解析 OAuth 端点的 OIDC 发现端点
	DiscoveryURL = Issuer + "/.well-known/openid-configuration"
	// ClientID 是公开的 xAI Grok CLI OAuth 客户端 ID
	ClientID = "b1a00492-073a-47ea-816f-4c329264a828"
	// Scope 是 xAI API 访问所需的 OAuth 权限范围
	Scope = "openid profile email offline_access grok-cli:access api:access"
	// RedirectHost 是 xAI OAuth 使用的回环主机
	RedirectHost = "127.0.0.1"
	// CallbackPort 是首选的回环回调端口
	CallbackPort = 56121
	// RedirectPath 是 xAI 客户端注册的回环回调路径
	RedirectPath = "/callback"
)

// refreshLead 是 xAI OAuth 凭证的刷新提前时间
var refreshLead = 5 * time.Minute

// RefreshLead 返回 xAI OAuth 凭证的刷新提前时间。
//
// 返回：
//   - time.Duration: 刷新提前时间
func RefreshLead() time.Duration {
	return refreshLead
}

// PKCECodes 保存 PKCE 验证器/挑战码对。
type PKCECodes struct {
	// CodeVerifier 是代码验证器
	CodeVerifier string
	// CodeChallenge 是代码挑战码
	CodeChallenge string
}

// AuthorizeURLParams 包含用于构建 xAI OAuth URL 的值。
type AuthorizeURLParams struct {
	// AuthorizationEndpoint 是授权端点 URL
	AuthorizationEndpoint string
	// RedirectURI 是重定向 URI
	RedirectURI string
	// CodeChallenge 是 PKCE 代码挑战码
	CodeChallenge string
	// State 是用于 CSRF 防护的状态参数
	State string
	// Nonce 是用于防止重放攻击的随机数
	Nonce string
}

// Discovery 包含从 xAI OIDC 发现解析的 OAuth 端点。
type Discovery struct {
	// AuthorizationEndpoint 是授权端点 URL
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	// TokenEndpoint 是令牌端点 URL
	TokenEndpoint string `json:"token_endpoint"`
}

// TokenData 保存 xAI OAuth 令牌数据。
type TokenData struct {
	// AccessToken 是 OAuth2 访问令牌
	AccessToken string `json:"access_token"`
	// RefreshToken 是 OAuth2 刷新令牌
	RefreshToken string `json:"refresh_token"`
	// IDToken 是 JWT ID 令牌
	IDToken string `json:"id_token,omitempty"`
	// TokenType 是令牌类型
	TokenType string `json:"token_type,omitempty"`
	// ExpiresIn 是令牌过期时间（秒）
	ExpiresIn int `json:"expires_in,omitempty"`
	// Expire 是令牌过期的 RFC3339 时间戳
	Expire string `json:"expired,omitempty"`
	// Email 是用户的电子邮件地址
	Email string `json:"email,omitempty"`
	// Subject 是用户的主题标识符
	Subject string `json:"sub,omitempty"`
}

// AuthBundle 聚合令牌数据和 OAuth 元数据以进行持久化。
type AuthBundle struct {
	// TokenData 包含令牌数据
	TokenData TokenData
	// LastRefresh 是最后刷新的时间戳
	LastRefresh string
	// BaseURL 是 API 基础 URL
	BaseURL string
	// RedirectURI 是 OAuth 重定向 URI
	RedirectURI string
	// TokenEndpoint 是令牌端点 URL
	TokenEndpoint string
}
