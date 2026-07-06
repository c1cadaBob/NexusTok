// 本文件定义了 Codex 渠道的 OAuth 密钥结构体和解析函数。
// Codex 渠道使用 JSON 格式的 OAuth 密钥进行认证，包含访问令牌、
// 刷新令牌、账户 ID 等信息。
package codex

// 标准库导入
import (
	"errors"
	"fmt"
	"strconv"
	"strings"

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
	var payload map[string]any
	if err := common.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, errors.New("codex channel: invalid oauth key json")
	}
	return &OAuthKey{
		IDToken:      readOAuthKeyString(payload, "id_token", "idToken"),
		AccessToken:  readOAuthKeyString(payload, "access_token", "accessToken"),
		RefreshToken: readOAuthKeyString(payload, "refresh_token", "refreshToken"),
		AccountID:    readOAuthKeyString(payload, "account_id", "accountId", "chatgpt_account_id"),
		LastRefresh:  readOAuthKeyString(payload, "last_refresh", "lastRefresh", "last_refreshed_at", "lastRefreshedAt"),
		Email:        readOAuthKeyString(payload, "email"),
		Type:         firstNonEmptyOAuthKeyString(readOAuthKeyString(payload, "type"), readOAuthKeyString(payload, "provider"), readOAuthKeyString(payload, "platform")),
		Expired:      readOAuthKeyString(payload, "expired", "expires_at", "expiresAt", "expiry", "expires"),
	}, nil
}

// readOAuthKeyString 兼容不同导入来源的字段类型。
// Sub2api 可能把 expired/expires_at 写成数字，直接解到 string 会失败；
// relay 热路径只需要稳定提取字符串形式，因此这里统一转成字符串。
func readOAuthKeyString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if trimmed := strings.TrimSpace(typed); trimmed != "" {
				return trimmed
			}
		case fmt.Stringer:
			if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
				return trimmed
			}
		case float64:
			if typed > 0 {
				return strconv.FormatInt(int64(typed), 10)
			}
		case int:
			if typed > 0 {
				return strconv.Itoa(typed)
			}
		case int64:
			if typed > 0 {
				return strconv.FormatInt(typed, 10)
			}
		case bool:
			return strconv.FormatBool(typed)
		default:
			text := strings.TrimSpace(fmt.Sprintf("%v", typed))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

// firstNonEmptyOAuthKeyString 返回第一个非空字符串。
// 用于兼容不同凭据导入工具对凭据类型字段的命名差异。
func firstNonEmptyOAuthKeyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
