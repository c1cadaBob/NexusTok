// auth - filestore_test.go
// 本文件包含 FileTokenStore 的单元测试，验证文件令牌存储的基本功能。
package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// testTokenStorage 是用于测试的 TokenStorage 模拟实现。
// 它将元数据保存到文件中，模拟真实的令牌存储行为。
type testTokenStorage struct {
	// meta 是注入的元数据映射。
	meta map[string]any
}

// SetMetadata 实现 metadataSetter 接口，接收注入的元数据。
func (s *testTokenStorage) SetMetadata(meta map[string]any) { s.meta = meta }

// SaveTokenToFile 将元数据序列化为 JSON 并写入指定文件。
func (s *testTokenStorage) SaveTokenToFile(authFilePath string) error {
	raw, err := json.Marshal(s.meta)
	if err != nil {
		return err
	}
	return os.WriteFile(authFilePath, raw, 0o600)
}

// TestFileTokenStore_Save_DisabledPersistsFlagForTokenStorage 验证禁用状态的认证记录
// 在通过 TokenStorage 保存时，disabled 标志能正确持久化到文件中。
func TestFileTokenStore_Save_DisabledPersistsFlagForTokenStorage(t *testing.T) {
	ctx := context.Background()
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "disabled.json")

	// 预置一个已禁用的认证文件
	if err := os.WriteFile(path, []byte(`{"type":"test","disabled":true}`), 0o600); err != nil {
		t.Fatalf("seed auth file: %v", err)
	}

	// 创建文件令牌存储并设置基础目录
	store := NewFileTokenStore()
	store.SetBaseDir(baseDir)
	storage := &testTokenStorage{}

	// 构建禁用状态的认证记录
	auth := &cliproxyauth.Auth{
		ID:       "disabled.json",
		Provider: "test",
		FileName: "disabled.json",
		Disabled: true,
		Storage:  storage,
		Metadata: map[string]any{"type": "test"},
	}

	// 执行保存操作
	if _, err := store.Save(ctx, auth); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// 读取保存后的文件并验证 disabled 标志
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read auth file: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("unmarshal auth file: %v", err)
	}
	if disabled, _ := meta["disabled"].(bool); !disabled {
		t.Fatalf("disabled=%v, want true (raw=%s)", meta["disabled"], string(raw))
	}
}
