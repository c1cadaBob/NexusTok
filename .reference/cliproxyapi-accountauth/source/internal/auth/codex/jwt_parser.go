// codex - jwt_parser.go
// 包 codex 提供 OpenAI Codex API 的认证和令牌管理功能。
// 该文件实现了 JWT（JSON Web Token）解析功能，
// 用于从 ID 令牌中提取用户信息和认证相关的声明。
package codex

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// JWTClaims 表示 JSON Web Token（JWT）的声明部分。
// 包含标准声明（如发行者、主题、过期时间）以及 OpenAI 认证特定的自定义声明。
type JWTClaims struct {
	// AtHash 是访问令牌的哈希值
	AtHash string `json:"at_hash"`
	// Aud 是令牌的受众列表
	Aud []string `json:"aud"`
	// AuthProvider 是认证提供商标识
	AuthProvider string `json:"auth_provider"`
	// AuthTime 是认证发生的时间
	AuthTime int `json:"auth_time"`
	// Email 是用户的电子邮件地址
	Email string `json:"email"`
	// EmailVerified 指示电子邮件是否已验证
	EmailVerified bool `json:"email_verified"`
	// Exp 是令牌的过期时间
	Exp int `json:"exp"`
	// CodexAuthInfo 包含 Codex 特定的认证信息
	CodexAuthInfo CodexAuthInfo `json:"https://api.openai.com/auth"`
	// Iat 是令牌的签发时间
	Iat int `json:"iat"`
	// Iss 是令牌的发行者
	Iss string `json:"iss"`
	// Jti 是令牌的唯一标识符
	Jti string `json:"jti"`
	// Rat 是令牌的请求时间
	Rat int `json:"rat"`
	// Sid 是会话标识符
	Sid string `json:"sid"`
	// Sub 是令牌的主题（通常是用户标识符）
	Sub string `json:"sub"`
}

// Organizations 定义了 JWT 声明中组织详细信息的结构。
// 包含用户的组织信息，如 ID、角色和头衔。
type Organizations struct {
	// ID 是组织的唯一标识符
	ID string `json:"id"`
	// IsDefault 指示是否为默认组织
	IsDefault bool `json:"is_default"`
	// Role 是用户在组织中的角色
	Role string `json:"role"`
	// Title 是用户在组织中的头衔
	Title string `json:"title"`
}

// CodexAuthInfo 包含 Codex 特定的认证相关信息。
// 包括 ChatGPT 账户信息、订阅状态和用户/组织 ID。
type CodexAuthInfo struct {
	// ChatgptAccountID 是 ChatGPT 账户标识符
	ChatgptAccountID string `json:"chatgpt_account_id"`
	// ChatgptPlanType 是 ChatGPT 订阅计划类型
	ChatgptPlanType string `json:"chatgpt_plan_type"`
	// ChatgptSubscriptionActiveStart 是订阅激活开始时间
	ChatgptSubscriptionActiveStart any `json:"chatgpt_subscription_active_start"`
	// ChatgptSubscriptionActiveUntil 是订阅激活结束时间
	ChatgptSubscriptionActiveUntil any `json:"chatgpt_subscription_active_until"`
	// ChatgptSubscriptionLastChecked 是最后检查订阅状态的时间
	ChatgptSubscriptionLastChecked time.Time `json:"chatgpt_subscription_last_checked"`
	// ChatgptUserID 是 ChatGPT 用户标识符
	ChatgptUserID string `json:"chatgpt_user_id"`
	// Groups 是用户所属的组列表
	Groups []any `json:"groups"`
	// Organizations 是用户所属的组织列表
	Organizations []Organizations `json:"organizations"`
	// UserID 是用户标识符
	UserID string `json:"user_id"`
}

// ParseJWTToken 解析 JWT 令牌字符串并提取其声明，不执行加密签名验证。
// 此功能用于在认证服务器验证令牌后，从 ID 令牌中检索用户信息。
//
// 参数：
//   - token: JWT 令牌字符串
//
// 返回：
//   - *JWTClaims: 解析后的 JWT 声明
//   - error: 解析失败时返回的错误
func ParseJWTToken(token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid JWT token format: expected 3 parts, got %d", len(parts))
	}

	// Decode the claims (payload) part
	claimsData, err := base64URLDecode(parts[1])
	if err != nil {
		return nil, fmt.Errorf("failed to decode JWT claims: %w", err)
	}

	var claims JWTClaims
	if err = json.Unmarshal(claimsData, &claims); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JWT claims: %w", err)
	}

	return &claims, nil
}

// base64URLDecode 解码 Base64 URL 编码的字符串，必要时添加填充。
// JWT 使用 URL 安全的 Base64 字母表并省略填充，因此此函数在解码前确保正确添加填充。
//
// 参数：
//   - data: Base64 URL 编码的字符串
//
// 返回：
//   - []byte: 解码后的字节数据
//   - error: 解码失败时返回的错误
func base64URLDecode(data string) ([]byte, error) {
	// Add padding if necessary
	switch len(data) % 4 {
	case 2:
		data += "=="
	case 3:
		data += "="
	}

	return base64.URLEncoding.DecodeString(data)
}

// GetUserEmail 从 JWT 声明中提取用户的电子邮件地址。
//
// 返回：
//   - string: 用户的电子邮件地址
func (c *JWTClaims) GetUserEmail() string {
	return c.Email
}

// GetAccountID 从 JWT 声明中提取用户的账户 ID（主题）。
// 获取用户 ChatGPT 账户的唯一标识符。
//
// 返回：
//   - string: 用户的 ChatGPT 账户 ID
func (c *JWTClaims) GetAccountID() string {
	return c.CodexAuthInfo.ChatgptAccountID
}
