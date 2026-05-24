// claude - anthropic.go
// 定义 Claude/Anthropic OAuth2 认证所需的核心数据类型，包括 PKCE 码对、
// OAuth Token 数据和认证包结构体。
package claude

// PKCECodes 保存 OAuth2 PKCE 流程中的验证器/挑战码对。
type PKCECodes struct {
	// CodeVerifier 是加密随机字符串，用于将授权请求与 Token 请求关联
	CodeVerifier string `json:"code_verifier"`
	// CodeChallenge 是 CodeVerifier 的 SHA256 哈希值，经 URL 安全 Base64 编码
	CodeChallenge string `json:"code_challenge"`
}

// ClaudeTokenData 保存 Anthropic OAuth Token 信息。
type ClaudeTokenData struct {
	// AccessToken 是用于 API 访问的 OAuth2 访问令牌
	AccessToken string `json:"access_token"`
	// RefreshToken 用于获取新的访问令牌
	RefreshToken string `json:"refresh_token"`
	// Email 是 Anthropic 账户邮箱地址
	Email string `json:"email"`
	// Expire 是 Token 过期时间戳
	Expire string `json:"expired"`
}

// ClaudeAuthBundle 聚合 OAuth 流程完成后的认证数据。
type ClaudeAuthBundle struct {
	// APIKey 是通过 Token 交换获取的 Anthropic API 密钥
	APIKey string `json:"api_key"`
	// TokenData 包含认证流程中的 OAuth Token 数据
	TokenData ClaudeTokenData `json:"token_data"`
	// LastRefresh 是最近一次 Token 刷新的时间戳
	LastRefresh string `json:"last_refresh"`
}
