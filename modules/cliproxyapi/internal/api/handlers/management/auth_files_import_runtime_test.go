// management - auth_files_import_runtime_test.go
// 测试认证文件重新导入时对旧运行态的清理，避免用户覆盖有效凭据后仍被旧错误拦截。
package management

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	coreauth "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/auth"
)

// TestWriteAuthFile_ExistingRecordClearsImportedRuntimeState 验证覆盖同名认证文件后，
// 管理器中的旧 Unauthorized、cooldown、ModelStates 和失败计数都会被清理。
// 这对应管理端“重新导入账号凭证”的语义：磁盘文件是新的事实来源，运行时错误需要重新观察。
func TestWriteAuthFile_ExistingRecordClearsImportedRuntimeState(t *testing.T) {
	authDir := t.TempDir()
	manager := coreauth.NewManager(nil, nil, nil)
	h := NewHandlerWithoutConfigFilePath(&config.Config{AuthDir: authDir}, manager)

	createdAt := time.Now().Add(-time.Hour)
	retryAt := time.Now().Add(time.Hour)
	existing := &coreauth.Auth{
		ID:               "codex.json",
		FileName:         "codex.json",
		Provider:         "codex",
		Status:           coreauth.StatusError,
		StatusMessage:    "Unauthorized",
		Unavailable:      true,
		CreatedAt:        createdAt,
		LastRefreshedAt:  time.Now().Add(-30 * time.Minute),
		NextRefreshAfter: retryAt,
		NextRetryAfter:   retryAt,
		Quota:            coreauth.QuotaState{Exceeded: true, Reason: "auth", BackoffLevel: 4, NextRecoverAt: retryAt},
		LastError:        &coreauth.Error{Code: "unauthorized", Message: "Unauthorized", HTTPStatus: 401},
		ModelStates: map[string]*coreauth.ModelState{
			"gpt-5.5": {
				Status:         coreauth.StatusError,
				StatusMessage:  "Unauthorized",
				Unavailable:    true,
				NextRetryAfter: retryAt,
				LastError:      &coreauth.Error{Code: "unauthorized", Message: "Unauthorized", HTTPStatus: 401},
				Quota:          coreauth.QuotaState{Exceeded: true, Reason: "auth", BackoffLevel: 3, NextRecoverAt: retryAt},
				UpdatedAt:      retryAt,
			},
		},
		Runtime: any("stale-runtime"),
		Success: 2,
		Failed:  5,
		Attributes: map[string]string{
			"path": filepath.Join(authDir, "codex.json"),
		},
		Metadata: map[string]any{"type": "codex", "email": "old@example.com"},
	}
	if _, errRegister := manager.Register(coreauth.WithSkipPersist(context.Background()), existing); errRegister != nil {
		t.Fatalf("register existing auth: %v", errRegister)
	}

	data := []byte(`{"type":"codex","email":"new@example.com","access_token":"new-token","refresh_token":"fresh-token"}`)
	if err := h.writeAuthFile(coreauth.WithSkipPersist(context.Background()), "codex.json", data); err != nil {
		t.Fatalf("writeAuthFile returned error: %v", err)
	}

	updated, ok := manager.GetByID("codex.json")
	if !ok || updated == nil {
		t.Fatalf("expected auth record to exist after import")
	}
	if updated.Status != coreauth.StatusActive {
		t.Fatalf("status = %q, want %q", updated.Status, coreauth.StatusActive)
	}
	if updated.StatusMessage != "" || updated.Unavailable {
		t.Fatalf("expected clean availability, status_message=%q unavailable=%v", updated.StatusMessage, updated.Unavailable)
	}
	if updated.LastError != nil {
		t.Fatalf("expected LastError to be cleared, got %#v", updated.LastError)
	}
	if !updated.NextRefreshAfter.IsZero() || !updated.NextRetryAfter.IsZero() {
		t.Fatalf("expected refresh/retry cooldowns to be cleared, refresh=%v retry=%v", updated.NextRefreshAfter, updated.NextRetryAfter)
	}
	if !updated.LastRefreshedAt.IsZero() {
		t.Fatalf("expected LastRefreshedAt not to be inherited, got %v", updated.LastRefreshedAt)
	}
	if updated.Quota != (coreauth.QuotaState{}) {
		t.Fatalf("expected quota state to be cleared, got %#v", updated.Quota)
	}
	if len(updated.ModelStates) != 0 {
		t.Fatalf("expected ModelStates to be cleared, got %d entries", len(updated.ModelStates))
	}
	if updated.Runtime != nil {
		t.Fatalf("expected Runtime to be cleared, got %#v", updated.Runtime)
	}
	if updated.Success != 0 || updated.Failed != 0 {
		t.Fatalf("expected counters to be reset, success=%d failed=%d", updated.Success, updated.Failed)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected CreatedAt to be preserved, got %v want %v", updated.CreatedAt, createdAt)
	}
	if got, _ := updated.Metadata["email"].(string); got != "new@example.com" {
		t.Fatalf("metadata email = %q, want new@example.com", got)
	}
	if got, _ := updated.Metadata["refresh_token"].(string); got != "fresh-token" {
		t.Fatalf("metadata refresh_token = %q, want fresh-token", got)
	}
}
