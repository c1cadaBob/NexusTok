// auth - filestore_disabled_test.go
// 文件令牌存储禁用状态持久化测试
// 验证当认证信息被标记为禁用(disabled)时，FileTokenStore 的 Save 方法
// 能够正确地将 disabled 标志持久化到磁盘文件中。
package auth

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// testTokenStorage 是用于测试的令牌存储实现，
// 实现了 cliproxyauth.Auth 的 Storage 接口，
// 将元数据序列化为 JSON 后写入指定文件路径。
type testTokenStorage struct {
	meta map[string]any
}

// SetMetadata 设置存储的元数据，供后续 SaveTokenToFile 使用。
func (s *testTokenStorage) SetMetadata(meta map[string]any) { s.meta = meta }

// SaveTokenToFile 将当前元数据序列化为 JSON 并写入指定的认证文件路径。
// 文件权限设置为 0o600（仅所有者可读写）。
func (s *testTokenStorage) SaveTokenToFile(authFilePath string) error {
	raw, err := json.Marshal(s.meta)
	if err != nil {
		return err
	}
	return os.WriteFile(authFilePath, raw, 0o600)
}

// TestFileTokenStore_Save_DisabledPersistsFlagForTokenStorage 测试场景：
// 当认证信息的 Disabled 字段为 true 时，调用 Save 方法后，
// 磁盘上的认证文件应包含 "disabled": true 字段。
// 这确保了禁用状态在重启后能够被正确恢复。
func TestFileTokenStore_Save_DisabledPersistsFlagForTokenStorage(t *testing.T) {
	ctx := context.Background()
	baseDir := t.TempDir()
	path := filepath.Join(baseDir, "disabled.json")

	if err := os.WriteFile(path, []byte(`{"type":"test","disabled":true}`), 0o600); err != nil {
		t.Fatalf("seed auth file: %v", err)
	}

	store := NewFileTokenStore()
	store.SetBaseDir(baseDir)
	storage := &testTokenStorage{}

	auth := &cliproxyauth.Auth{
		ID:       "disabled.json",
		Provider: "test",
		FileName: "disabled.json",
		Disabled: true,
		Storage:  storage,
		Metadata: map[string]any{"type": "test"},
	}

	if _, err := store.Save(ctx, auth); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

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
