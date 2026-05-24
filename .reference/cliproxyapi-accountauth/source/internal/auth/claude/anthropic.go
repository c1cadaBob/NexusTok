// claude - anthropic.go
// 包 claude 提供 Anthropic Claude API 的 OAuth2 认证功能。
// 该文件定义了 Claude 认证流程中使用的核心数据结构，
// 包括 PKCE 代码、令牌数据和认证包等。
package claude

// PKCECodes 保存 OAuth2 PKCE 流程的验证代码。
// PKCE（Proof Key for Code Exchange）是 OAuth2 的安全扩展，
// 用于防止 CSRF 和授权码注入攻击。
type PKCECodes struct {
	// CodeVerifier 是用于关联授权请求和令牌请求的加密随机字符串
	CodeVerifier string `json:"code_verifier"`
	// CodeChallenge 是代码验证器的 SHA256 哈希值，经过 base64url 编码
	CodeChallenge string `json:"code_challenge"`
}

// ClaudeTokenData 保存从 Anthropic 获取的 OAuth 令牌信息。
// 包含访问令牌、刷新令牌和关联的用户账户信息。
type ClaudeTokenData struct {
	// AccessToken 是用于 API 访问的 OAuth2 访问令牌
	AccessToken string `json:"access_token"`
	// RefreshToken 是用于获取新访问令牌的刷新令牌
	RefreshToken string `json:"refresh_token"`
	// Email 是 Anthropic 账户的电子邮件地址
	Email string `json:"email"`
	// Expire 是令牌过期的时间戳
	Expire string `json:"expired"`
}

// ClaudeAuthBundle 聚合了 OAuth 流程完成后的认证数据。
// 包含 API 密钥、令牌数据和最后刷新时间戳。
type ClaudeAuthBundle struct {
	// APIKey 是从令牌交换中获取的 Anthropic API 密钥
	APIKey string `json:"api_key"`
	// TokenData 包含认证流程中的 OAuth 令牌
	TokenData ClaudeTokenData `json:"token_data"`
	// LastRefresh 是最后刷新令牌的时间戳
	LastRefresh string `json:"last_refresh"`
}
