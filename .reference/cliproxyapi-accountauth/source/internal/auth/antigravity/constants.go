// antigravity - constants.go
// 包 antigravity 提供 Antigravity 提供商的 OAuth2 认证功能。
// 该文件定义了 OAuth2 认证流程所需的常量，包括客户端凭证、
// 权限范围、OAuth2 端点 URL 和 API 配置等。
package antigravity

// OAuth 客户端凭证和配置常量。
// 包含 Google OAuth2 应用的客户端 ID、客户端密钥和本地回调端口。
const (
	// ClientID 是 Google OAuth2 应用的客户端标识符
	ClientID = "REDACTED_GOOGLE_OAUTH_CLIENT_ID"
	// ClientSecret 是 Google OAuth2 应用的客户端密钥
	ClientSecret = "REDACTED_GOOGLE_OAUTH_CLIENT_SECRET"
	// CallbackPort 是本地 OAuth 回调服务器监听的端口号
	CallbackPort = 51121
)

// Scopes 定义了 Antigravity 认证所需的 OAuth 权限范围。
// 包括云平台访问、用户信息读取、日志记录和实验配置等权限。
var Scopes = []string{
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/userinfo.profile",
	"https://www.googleapis.com/auth/cclog",
	"https://www.googleapis.com/auth/experimentsandconfigs",
}

// OAuth2 端点 URL 常量。
// 定义了 Google OAuth2 认证流程中使用的各个端点地址。
const (
	// TokenEndpoint 是用于交换授权码或刷新令牌的令牌端点
	TokenEndpoint = "https://oauth2.googleapis.com/token"
	// AuthEndpoint 是用户授权的认证端点
	AuthEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	// UserInfoEndpoint 是获取用户信息的端点
	UserInfoEndpoint = "https://www.googleapis.com/oauth2/v2/userinfo?alt=json"
)

// Antigravity API 配置常量。
// 定义了 Antigravity 云代码 API 的基础 URL 和 API 版本。
const (
	// APIEndpoint 是 Antigravity 云代码 API 的基础 URL
	APIEndpoint = "https://cloudcode-pa.googleapis.com"
	// APIVersion 是使用的 API 版本号
	APIVersion = "v1internal"
)
