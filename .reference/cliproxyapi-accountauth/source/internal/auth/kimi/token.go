// kimi - token.go
// 包 kimi 提供 Kimi（Moonshot AI）服务的认证和令牌管理功能。
// 该文件实现了 OAuth2 设备流程令牌的存储、序列化和持久化功能，
// 用于维护与 Kimi API 的认证会话。
package kimi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
)

// KimiTokenStorage 存储 Kimi API 认证的 OAuth2 令牌信息。
type KimiTokenStorage struct {
	// AccessToken 是用于认证 API 请求的 OAuth2 访问令牌
	AccessToken string `json:"access_token"`
	// RefreshToken 是用于获取新访问令牌的 OAuth2 刷新令牌
	RefreshToken string `json:"refresh_token"`
	// TokenType 是令牌类型，通常为 "Bearer"
	TokenType string `json:"token_type"`
	// Scope 是授予令牌的 OAuth2 权限范围
	Scope string `json:"scope,omitempty"`
	// DeviceID 是用于 Kimi 请求的 OAuth 设备流程标识符
	DeviceID string `json:"device_id,omitempty"`
	// Expired 是访问令牌过期的 RFC3339 时间戳
	Expired string `json:"expired,omitempty"`
	// Type 表示认证提供商类型，此存储始终为 "kimi"
	Type string `json:"type"`

	// Metadata 保存通过钩子注入的任意键值对。
	// 不直接导出到 JSON，以允许在序列化时进行扁平化处理。
	Metadata map[string]any `json:"-"`
}

// SetMetadata 允许外部调用者在保存之前向存储注入元数据。
//
// 参数：
//   - meta: 要注入的元数据键值对
func (ts *KimiTokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// KimiTokenData 保存从 Kimi 获取的原始 OAuth 令牌响应。
type KimiTokenData struct {
	// AccessToken 是 OAuth2 访问令牌
	AccessToken string `json:"access_token"`
	// RefreshToken 是 OAuth2 刷新令牌
	RefreshToken string `json:"refresh_token"`
	// TokenType 是令牌类型，通常为 "Bearer"
	TokenType string `json:"token_type"`
	// ExpiresAt 是令牌过期的 Unix 时间戳
	ExpiresAt int64 `json:"expires_at"`
	// Scope 是授予令牌的 OAuth2 权限范围
	Scope string `json:"scope"`
}

// KimiAuthBundle 为存储捆绑认证数据。
type KimiAuthBundle struct {
	// TokenData 包含 OAuth 令牌信息
	TokenData *KimiTokenData
	// DeviceID 是 OAuth 设备流程期间使用的设备标识符
	DeviceID string
}

// DeviceCodeResponse 表示 Kimi 的设备代码响应。
type DeviceCodeResponse struct {
	// DeviceCode 是设备验证代码
	DeviceCode string `json:"device_code"`
	// UserCode 是用户必须在验证 URI 处输入的代码
	UserCode string `json:"user_code"`
	// VerificationURI 是用户应输入代码的 URL
	VerificationURI string `json:"verification_uri,omitempty"`
	// VerificationURIComplete 是预填充代码的 URL
	VerificationURIComplete string `json:"verification_uri_complete"`
	// ExpiresIn 是设备代码过期的秒数
	ExpiresIn int `json:"expires_in"`
	// Interval 是轮询请求之间的最小等待秒数
	Interval int `json:"interval"`
}

// SaveTokenToFile 将 Kimi 令牌存储序列化为 JSON 文件。
//
// 参数：
//   - authFilePath: 令牌文件应保存的完整路径
//
// 返回：
//   - error: 操作失败时返回的错误，成功时返回 nil
func (ts *KimiTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "kimi"

	if err := os.MkdirAll(filepath.Dir(authFilePath), 0700); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	f, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	// Merge metadata using helper
	data, errMerge := misc.MergeMetadata(ts, ts.Metadata)
	if errMerge != nil {
		return fmt.Errorf("failed to merge metadata: %w", errMerge)
	}

	encoder := json.NewEncoder(f)
	encoder.SetIndent("", "  ")
	if err = encoder.Encode(data); err != nil {
		return fmt.Errorf("failed to write token to file: %w", err)
	}
	return nil
}

// IsExpired 检查令牌是否已过期。
// 如果在刷新阈值内，则认为令牌已过期。
//
// 返回：
//   - bool: 令牌已过期返回 true
func (ts *KimiTokenStorage) IsExpired() bool {
	if ts.Expired == "" {
		return false // No expiry set, assume valid
	}
	t, err := time.Parse(time.RFC3339, ts.Expired)
	if err != nil {
		return true // Has expiry string but can't parse
	}
	// Consider expired if within refresh threshold
	return time.Now().Add(time.Duration(refreshThresholdSeconds) * time.Second).After(t)
}

// NeedsRefresh 检查令牌是否需要刷新。
// 如果没有刷新令牌，则无法刷新；否则检查令牌是否已过期。
//
// 返回：
//   - bool: 令牌需要刷新返回 true
func (ts *KimiTokenStorage) NeedsRefresh() bool {
	if ts.RefreshToken == "" {
		return false // Can't refresh without refresh token
	}
	return ts.IsExpired()
}
