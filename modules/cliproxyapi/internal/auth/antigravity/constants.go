// Package antigravity - constants.go
// 定义 Antigravity 提供商的 OAuth2 认证常量和配置。
// 包括 OAuth 客户端凭据、回调端口、所需权限范围、
// 以及 Google OAuth2 和 Antigravity API 的端点地址。
//
// Package antigravity provides OAuth2 authentication functionality for the Antigravity provider.
package antigravity

import "os"

// OAuth 客户端凭据，从环境变量读取
var (
	// ClientID 是 Antigravity OAuth 客户端 ID
	ClientID = os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_ID")
	// ClientSecret 是 Antigravity OAuth 客户端密钥
	ClientSecret = os.Getenv("ANTIGRAVITY_OAUTH_CLIENT_SECRET")
)

const (
	// CallbackPort 是 OAuth 回调监听端口
	CallbackPort = 51121
)

// Scopes defines the OAuth scopes required for Antigravity authentication
var Scopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

// OAuth2 endpoints for Google authentication
const (
	TokenEndpoint    = "https://oauth2.googleapis.com/token"
	AuthEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	UserInfoEndpoint = "https://www.googleapis.com/oauth2/v2/userinfo?alt=json"
)

// Antigravity API configuration
const (
	APIEndpoint = "https://cloudcode-pa.googleapis.com"
	APIVersion  = "v1internal"
)
