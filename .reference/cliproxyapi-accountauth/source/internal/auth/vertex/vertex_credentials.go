// vertex - vertex_credentials.go
// 包 vertex 提供通过服务账户凭证访问 Google Vertex AI Gemini 的令牌存储功能。
// 将服务账户 JSON 序列化为运行时执行器使用的认证文件。
package vertex

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/misc"
	log "github.com/sirupsen/logrus"
)

// VertexCredentialStorage 存储用于 Vertex AI 访问的服务账户 JSON。
// 内容在 "service_account" 键下原样持久化，同时包含项目、位置和电子邮件等辅助字段，
// 以改善日志记录和发现。
type VertexCredentialStorage struct {
	// ServiceAccount 保存解析后的服务账户 JSON 内容
	ServiceAccount map[string]any `json:"service_account"`

	// ProjectID 从服务账户 JSON 中派生（project_id）
	ProjectID string `json:"project_id"`

	// Email 是服务账户 JSON 中的 client_email
	Email string `json:"email"`

	// Location 可选地为 Vertex 端点设置默认区域（如 us-central1）
	Location string `json:"location,omitempty"`

	// Type 是与凭证一起存储的提供商标识符，始终为 "vertex"
	Type string `json:"type"`

	// Prefix 可选地为此凭证的模型命名空间化（如 "teamA"）。
	// 这将导致模型名称如 "teamA/gemini-2.0-flash"。
	Prefix string `json:"prefix,omitempty"`
}

// SaveTokenToFile 以 JSON 格式将凭证有效负载写入给定的文件路径。
// 确保父目录存在，并记录操作以保持透明度。
//
// 参数：
//   - authFilePath: 凭证文件应保存的完整路径
//
// 返回：
//   - error: 操作失败时返回的错误，成功时返回 nil
func (s *VertexCredentialStorage) SaveTokenToFile(authFilePath string) error {
	misc.LogSavingCredentials(authFilePath)
	if s == nil {
		return fmt.Errorf("vertex credential: storage is nil")
	}
	if s.ServiceAccount == nil {
		return fmt.Errorf("vertex credential: service account content is empty")
	}
	// Ensure we tag the file with the provider type.
	s.Type = "vertex"

	if err := os.MkdirAll(filepath.Dir(authFilePath), 0o700); err != nil {
		return fmt.Errorf("vertex credential: create directory failed: %w", err)
	}
	f, err := os.Create(authFilePath)
	if err != nil {
		return fmt.Errorf("vertex credential: create file failed: %w", err)
	}
	defer func() {
		if errClose := f.Close(); errClose != nil {
			log.Errorf("vertex credential: failed to close file: %v", errClose)
		}
	}()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(s); err != nil {
		return fmt.Errorf("vertex credential: encode failed: %w", err)
	}
	return nil
}
