// 本文件定义了 Codex 渠道的 OAuth 密钥结构体和解析函数。
// Codex 渠道使用 JSON 格式的 OAuth 密钥进行认证，包含访问令牌、
// 刷新令牌、账户 ID 等信息。
package codex

// 标准库导入
import (
	"errors"

	"github.com/c1cada/NexusTok/common" // 项目公共工具，包含 JSON 序列化函数
)

// OAuthKey 表示 Codex 渠道的 OAuth 认证密钥结构体。
// 密钥以 JSON 字符串形式存储在渠道配置的 API Key 字段中。
type OAuthKey struct {
	IDToken      string `json:"id_token,omitempty"`      // OpenID Connect ID Token
	AccessToken  string `json:"access_token,omitempty"`  // OAuth 访问令牌，用于 API 认证
	RefreshToken string `json:"refresh_token,omitempty"` // OAuth 刷新令牌，用于获取新的访问令牌

	AccountID   string `json:"account_id,omitempty"`   // ChatGPT 账户 ID，请求头必需
	LastRefresh string `json:"last_refresh,omitempty"` // 上次刷新令牌的时间
	Email       string `json:"email,omitempty"`        // 关联的邮箱地址
	Type        string `json:"type,omitempty"`         // 密钥类型
	Expired     string `json:"expired,omitempty"`      // 令牌过期时间
}

// ParseOAuthKey 将 JSON 格式的原始字符串解析为 OAuthKey 结构体。
// 参数：raw - JSON 格式的 OAuth 密钥字符串。
// 返回值：解析后的 OAuthKey 指针和可能的错误。
// 错误情况包括：空字符串输入、JSON 格式无效。
func ParseOAuthKey(raw string) (*OAuthKey, error) {
	if raw == "" {
		return nil, errors.New("codex channel: empty oauth key")
	}
	var key OAuthKey
	if err := common.Unmarshal([]byte(raw), &key); err != nil {
		return nil, errors.New("codex channel: invalid oauth key json")
	}
	return &key, nil
}
