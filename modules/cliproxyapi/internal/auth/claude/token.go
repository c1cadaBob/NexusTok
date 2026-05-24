// claude - token.go
// 提供 Claude/Anthropic OAuth 凭证的磁盘持久化功能，包括 ClaudeTokenStorage 结构体的
// 序列化、文件写入、元数据合并等。
package claude

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
)

// ClaudeTokenStorage 存储 Anthropic Claude API 认证所需的 OAuth2 Token 信息。
// 它在保持与现有认证系统兼容的同时，增加了 Claude 特有的字段用于管理
// 访问令牌、刷新令牌和用户账户信息。
type ClaudeTokenStorage struct {
	// IDToken 是包含用户声明和身份信息的 JWT ID 令牌
	IDToken string `json:"id_token"`

	// AccessToken 是用于认证 API 请求的 OAuth2 访问令牌
	AccessToken string `json:"access_token"`

	// RefreshToken 用于在当前访问令牌过期时获取新的访问令牌
	RefreshToken string `json:"refresh_token"`

	// LastRefresh 是最近一次 Token 刷新操作的时间戳
	LastRefresh string `json:"last_refresh"`

	// Email 是与此 Token 关联的 Anthropic 账户邮箱地址
	Email string `json:"email"`

	// Type 标识认证提供者类型，此存储始终为 "claude"
	Type string `json:"type"`

	// Expire 是当前访问令牌的过期时间戳
	Expire string `json:"expired"`

	// Metadata 保存通过钩子注入的任意键值对。
	// 不直接导出到 JSON，以便在序列化时进行扁平化处理。
	Metadata map[string]any `json:"-"`
}

// SetMetadata 允许外部调用者在保存前向存储注入元数据。
func (ts *ClaudeTokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// SaveTokenToFile 将 Claude Token 存储序列化为 JSON 文件。
// 该方法创建必要的目录结构，并以 JSON 格式将 Token 数据写入指定文件路径进行持久化存储。
// 它会将注入的元数据合并到顶层 JSON 对象中。
//
// 参数:
//   - authFilePath: Token 文件的完整保存路径
//
// 返回值:
//   - error: 操作失败时返回错误，成功时返回 nil
func (ts *ClaudeTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "claude"

	// 如果目录不存在则创建目录结构
	if err := os.MkdirAll(filepath.Dir(authFilePath), 0700); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	// 创建 Token 文件
	f, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	// 使用辅助函数合并元数据
	data, errMerge := misc.MergeMetadata(ts, ts.Metadata)
	if errMerge != nil {
		return fmt.Errorf("failed to merge metadata: %w", errMerge)
	}

	// 将 Token 数据编码为 JSON 并写入文件
	if err = json.NewEncoder(f).Encode(data); err != nil {
		return fmt.Errorf("failed to write token to file: %w", err)
	}
	return nil
}
