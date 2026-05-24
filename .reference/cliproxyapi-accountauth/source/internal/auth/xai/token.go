// xai - token.go
// 包 xai 提供 xAI Grok 的 OAuth2 认证功能。
// 该文件实现了 xAI OAuth 令牌的存储、序列化和持久化功能。
package xai

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	log "github.com/sirupsen/logrus"
)

// TokenStorage 在磁盘上存储 xAI OAuth 凭证。
type TokenStorage struct {
	// Type 是认证提供商类型
	Type string `json:"type"`
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
	// LastRefresh 是最后刷新的时间戳
	LastRefresh string `json:"last_refresh,omitempty"`
	// Email 是用户的电子邮件地址
	Email string `json:"email,omitempty"`
	// Subject 是用户的主题标识符
	Subject string `json:"sub,omitempty"`
	// BaseURL 是 API 基础 URL
	BaseURL string `json:"base_url,omitempty"`
	// RedirectURI 是 OAuth 重定向 URI
	RedirectURI string `json:"redirect_uri,omitempty"`
	// TokenEndpoint 是令牌端点 URL
	TokenEndpoint string `json:"token_endpoint,omitempty"`
	// AuthKind 是认证类型（如 "oauth"）
	AuthKind string `json:"auth_kind,omitempty"`

	// Metadata 保存通过钩子注入的任意键值对
	Metadata map[string]any `json:"-"`
}

// SetMetadata 允许令牌存储在保存前合并状态字段。
//
// 参数：
//   - meta: 要注入的元数据键值对
func (ts *TokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// SaveTokenToFile 将 xAI 凭证写入 JSON 认证文件。
//
// 参数：
//   - authFilePath: 凭证文件应保存的完整路径
//
// 返回：
//   - error: 操作失败时返回的错误，成功时返回 nil
func (ts *TokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "xai"
	ts.AuthKind = "oauth"
	if errMkdirAll := os.MkdirAll(filepath.Dir(authFilePath), 0o700); errMkdirAll != nil {
		return fmt.Errorf("xai token storage: create directory: %w", errMkdirAll)
	}
	file, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("xai token storage: create token file: %w", err)
	}
	defer func() {
		if errClose := file.Close(); errClose != nil {
			log.Errorf("xai token storage: close token file error: %v", errClose)
		}
	}()

	data, errMerge := misc.MergeMetadata(ts, ts.Metadata)
	if errMerge != nil {
		return fmt.Errorf("xai token storage: merge metadata: %w", errMerge)
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(data); err != nil {
		return fmt.Errorf("xai token storage: write token file: %w", err)
	}
	return nil
}

// CredentialFileName 返回用于 xAI 凭证的文件名。
// 使用电子邮件或主题标识符作为文件名的一部分。
//
// 参数：
//   - email: 用户的电子邮件地址
//   - subject: 用户的主题标识符
//
// 返回：
//   - string: 凭证文件的完整文件名
func CredentialFileName(email, subject string) string {
	email = sanitizeFileSegment(email)
	if email != "" {
		return fmt.Sprintf("xai-%s.json", email)
	}
	subject = sanitizeFileSegment(subject)
	if subject != "" {
		return fmt.Sprintf("xai-%s.json", subject)
	}
	return fmt.Sprintf("xai-%d.json", time.Now().UnixMilli())
}

// sanitizeFileSegment 清理文件名段，移除不安全的字符。
// 仅保留字母、数字和常见的安全字符（@、.、_、-）。
//
// 参数：
//   - value: 要清理的字符串
//
// 返回：
//   - string: 清理后的字符串
func sanitizeFileSegment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '@' || r == '.' || r == '_' || r == '-':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
