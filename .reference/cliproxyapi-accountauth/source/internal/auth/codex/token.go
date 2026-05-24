// codex - token.go
// 包 codex 提供 OpenAI Codex API 的认证和令牌管理功能。
// 该文件实现了 OAuth2 令牌的存储、序列化和持久化功能，
// 用于维护与 Codex API 的认证会话。
package codex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
)

// CodexTokenStorage 存储 OpenAI Codex API 认证的 OAuth2 令牌信息。
// 维护与现有认证系统的兼容性，同时添加 Codex 特定的字段，
// 用于管理访问令牌、刷新令牌和用户账户信息。
type CodexTokenStorage struct {
	// IDToken 是包含用户声明和身份信息的 JWT ID 令牌
	IDToken string `json:"id_token"`
	// AccessToken 是用于认证 API 请求的 OAuth2 访问令牌
	AccessToken string `json:"access_token"`
	// RefreshToken 是用于在当前令牌过期时获取新访问令牌的刷新令牌
	RefreshToken string `json:"refresh_token"`
	// AccountID 是与此令牌关联的 OpenAI 账户标识符
	AccountID string `json:"account_id"`
	// LastRefresh 是最后执行令牌刷新操作的时间戳
	LastRefresh string `json:"last_refresh"`
	// Email 是与此令牌关联的 OpenAI 账户电子邮件地址
	Email string `json:"email"`
	// Type 表示认证提供商类型，此存储始终为 "codex"
	Type string `json:"type"`
	// Expire 是当前访问令牌过期的时间戳
	Expire string `json:"expired"`

	// Metadata 保存通过钩子注入的任意键值对。
	// 不直接导出到 JSON，以允许在序列化时进行扁平化处理。
	Metadata map[string]any `json:"-"`
}

// SetMetadata 允许外部调用者在保存之前向存储注入元数据。
//
// 参数：
//   - meta: 要注入的元数据键值对
func (ts *CodexTokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// SaveTokenToFile 将 Codex 令牌存储序列化为 JSON 文件。
// 创建必要的目录结构，并以 JSON 格式将令牌数据写入指定的文件路径进行持久化存储。
// 将注入的元数据合并到顶层 JSON 对象中。
//
// 参数：
//   - authFilePath: 令牌文件应保存的完整路径
//
// 返回：
//   - error: 操作失败时返回的错误，成功时返回 nil
func (ts *CodexTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "codex"
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

	if err = json.NewEncoder(f).Encode(data); err != nil {
		return fmt.Errorf("failed to write token to file: %w", err)
	}
	return nil

}
