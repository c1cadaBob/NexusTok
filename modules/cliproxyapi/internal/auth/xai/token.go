// xai - token.go
// 提供 xAI OAuth 凭证的磁盘持久化功能，包括 TokenStorage 结构体的序列化、
// 文件写入、凭证文件名生成等。
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

// TokenStorage 将 xAI OAuth 凭证存储到磁盘。
type TokenStorage struct {
	Type          string `json:"type"`
	AccessToken   string `json:"access_token"`
	RefreshToken  string `json:"refresh_token"`
	IDToken       string `json:"id_token,omitempty"`
	TokenType     string `json:"token_type,omitempty"`
	ExpiresIn     int    `json:"expires_in,omitempty"`
	Expire        string `json:"expired,omitempty"`
	LastRefresh   string `json:"last_refresh,omitempty"`
	Email         string `json:"email,omitempty"`
	Subject       string `json:"sub,omitempty"`
	BaseURL       string `json:"base_url,omitempty"`
	RedirectURI   string `json:"redirect_uri,omitempty"`
	TokenEndpoint string `json:"token_endpoint,omitempty"`
	AuthKind      string `json:"auth_kind,omitempty"`

	Metadata map[string]any `json:"-"`
}

// SetMetadata 允许 token 存储在保存前合并状态字段。
func (ts *TokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// SaveTokenToFile 将 xAI 凭证写入 JSON 认证文件。
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

// CredentialFileName 返回 xAI 凭证使用的文件名，优先使用邮箱，其次使用 subject，最后使用时间戳。
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

// sanitizeFileSegment 清理文件名段，只保留字母、数字和少量安全字符。
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
