// Package codex - openai.go
// 定义 Codex OAuth 认证流程中使用的核心数据结构。
// 包括 PKCE 代码对、OAuth 令牌数据和认证数据包，
// 用于在 OAuth2 认证完成后聚合和管理所有认证相关数据。
package codex

// PKCECodes 存储 OAuth2 PKCE（Proof Key for Code Exchange）流程的验证代码。
// PKCE 是授权码流程的扩展，用于防止 CSRF 和授权码注入攻击。
// 持有用于关联授权请求与令牌请求的密码验证器及其 SHA256 挑战码。
type PKCECodes struct {
	// CodeVerifier is the cryptographically random string used to correlate
	// the authorization request to the token request
	CodeVerifier string `json:"code_verifier"`
	// CodeChallenge is the SHA256 hash of the code verifier, base64url-encoded
	CodeChallenge string `json:"code_challenge"`
}

// CodexTokenData holds the OAuth token information obtained from OpenAI.
// It includes the ID token, access token, refresh token, and associated user details.
type CodexTokenData struct {
	// IDToken is the JWT ID token containing user claims
	IDToken string `json:"id_token"`
	// AccessToken is the OAuth2 access token for API access
	AccessToken string `json:"access_token"`
	// RefreshToken is used to obtain new access tokens
	RefreshToken string `json:"refresh_token"`
	// AccountID is the OpenAI account identifier
	AccountID string `json:"account_id"`
	// Email is the OpenAI account email
	Email string `json:"email"`
	// Expire is the timestamp of the token expire
	Expire string `json:"expired"`
}

// CodexAuthBundle aggregates all authentication-related data after the OAuth flow is complete.
// This includes the API key, token data, and the timestamp of the last refresh.
type CodexAuthBundle struct {
	// APIKey is the OpenAI API key obtained from token exchange
	APIKey string `json:"api_key"`
	// TokenData contains the OAuth tokens from the authentication flow
	TokenData CodexTokenData `json:"token_data"`
	// LastRefresh is the timestamp of the last token refresh
	LastRefresh string `json:"last_refresh"`
}
