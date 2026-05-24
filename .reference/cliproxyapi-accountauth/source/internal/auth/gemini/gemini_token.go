// gemini - gemini_token.go
// 包 gemini 提供 Google Gemini AI 服务的认证和令牌管理功能。
// 该文件实现了 OAuth2 令牌的存储、序列化和持久化功能，
// 用于维护与 Gemini API 的认证会话。
package gemini

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	log "github.com/sirupsen/logrus"
)

// GeminiTokenStorage 存储 Google Gemini API 认证的 OAuth2 令牌信息。
// 维护与现有认证系统的兼容性，同时添加 Gemini 特定的字段，
// 用于管理访问令牌、刷新令牌和用户账户信息。
type GeminiTokenStorage struct {
	// Token 保存原始的 OAuth2 令牌数据，包括访问令牌和刷新令牌
	Token any `json:"token"`

	// ProjectID 是与此令牌关联的 Google Cloud 项目 ID
	ProjectID string `json:"project_id"`

	// Email 是已认证用户的电子邮件地址
	Email string `json:"email"`

	// Auto 指示项目 ID 是否为自动选择
	Auto bool `json:"auto"`

	// Checked 指示关联的 Cloud AI API 是否已验证为已启用
	Checked bool `json:"checked"`

	// Type 表示认证提供商类型，此存储始终为 "gemini"
	Type string `json:"type"`

	// Metadata 保存通过钩子注入的任意键值对。
	// 不直接导出到 JSON，以允许在序列化时进行扁平化处理。
	Metadata map[string]any `json:"-"`
}

// SetMetadata 允许外部调用者在保存之前向存储注入元数据。
//
// 参数：
//   - meta: 要注入的元数据键值对
func (ts *GeminiTokenStorage) SetMetadata(meta map[string]any) {
	ts.Metadata = meta
}

// SaveTokenToFile 将 Gemini 令牌存储序列化为 JSON 文件。
// 创建必要的目录结构，并以 JSON 格式将令牌数据写入指定的文件路径进行持久化存储。
// 将注入的元数据合并到顶层 JSON 对象中。
//
// 参数：
//   - authFilePath: 令牌文件应保存的完整路径
//
// 返回：
//   - error: 操作失败时返回的错误，成功时返回 nil
func (ts *GeminiTokenStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	ts.Type = "gemini"
	// Merge metadata using helper
	data, errMerge := misc.MergeMetadata(ts, ts.Metadata)
	if errMerge != nil {
		return fmt.Errorf("failed to merge metadata: %w", errMerge)
	}
	if err := os.MkdirAll(filepath.Dir(authFilePath), 0700); err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}

	f, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("failed to create token file: %w", err)
	}
	defer func() {
		if errClose := f.Close(); errClose != nil {
			log.Errorf("failed to close file: %v", errClose)
		}
	}()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		return fmt.Errorf("failed to write token to file: %w", err)
	}
	return nil
}

// CredentialFileName 返回用于持久化 Gemini CLI 凭证的文件名。
// 当 projectID 表示多个项目（逗号分隔或字面量 ALL）时，
// 后缀规范化为 "all"，并强制使用 "gemini-" 前缀，
// 以保持 Web 和 CLI 生成的文件一致性。
//
// 参数：
//   - email: 用户的电子邮件地址
//   - projectID: Google Cloud 项目 ID
//   - includeProviderPrefix: 是否包含提供商前缀
//
// 返回：
//   - string: 凭证文件的完整文件名
func CredentialFileName(email, projectID string, includeProviderPrefix bool) string {
	email = strings.TrimSpace(email)
	project := strings.TrimSpace(projectID)
	if strings.EqualFold(project, "all") || strings.Contains(project, ",") {
		return fmt.Sprintf("gemini-%s-all.json", email)
	}
	prefix := ""
	if includeProviderPrefix {
		prefix = "gemini-"
	}
	return fmt.Sprintf("%s%s-%s.json", prefix, email, project)
}
