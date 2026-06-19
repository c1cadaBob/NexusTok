// auth - conductor_update_test.go
// 测试 Manager.Update 方法在更新认证信息时是否正确保留 ModelStates 状态数据。
// 覆盖场景包括：活跃认证继承、禁用认证不继承、活跃转禁用不继承、禁用转活跃不继承。
package auth

import (
	"context"
	"testing"
	"time"
)

// TestManager_Update_PreservesModelStates 测试更新活跃认证时保留已有 ModelStates。
func TestManager_Update_PreservesModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	model := "test-model"
	backoffLevel := 7

	if _, errRegister := m.Register(context.Background(), &Auth{
		ID:       "auth-1",
		Provider: "claude",
		Metadata: map[string]any{"k": "v"},
		ModelStates: map[string]*ModelState{
			model: {
				Quota: QuotaState{BackoffLevel: backoffLevel},
			},
		},
	}); errRegister != nil {
		t.Fatalf("register auth: %v", errRegister)
	}

	if _, errUpdate := m.Update(context.Background(), &Auth{
		ID:       "auth-1",
		Provider: "claude",
		Metadata: map[string]any{"k": "v2"},
	}); errUpdate != nil {
		t.Fatalf("update auth: %v", errUpdate)
	}

	updated, ok := m.GetByID("auth-1")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) == 0 {
		t.Fatalf("expected ModelStates to be preserved")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatalf("expected model state to be present")
	}
	if state.Quota.BackoffLevel != backoffLevel {
		t.Fatalf("expected BackoffLevel to be %d, got %d", backoffLevel, state.Quota.BackoffLevel)
	}
}

// TestManager_Update_DisabledExistingDoesNotInheritModelStates 测试更新已禁用认证时不继承旧的 ModelStates。
func TestManager_Update_DisabledExistingDoesNotInheritModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	// Register a disabled auth with existing ModelStates.
	if _, err := m.Register(context.Background(), &Auth{
		ID:       "auth-disabled",
		Provider: "claude",
		Disabled: true,
		Status:   StatusDisabled,
		ModelStates: map[string]*ModelState{
			"stale-model": {
				Quota: QuotaState{BackoffLevel: 5},
			},
		},
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// Update with empty ModelStates — should NOT inherit stale states.
	if _, err := m.Update(context.Background(), &Auth{
		ID:       "auth-disabled",
		Provider: "claude",
		Disabled: true,
		Status:   StatusDisabled,
	}); err != nil {
		t.Fatalf("update auth: %v", err)
	}

	updated, ok := m.GetByID("auth-disabled")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) != 0 {
		t.Fatalf("expected disabled auth NOT to inherit ModelStates, got %d entries", len(updated.ModelStates))
	}
}

// TestManager_Update_ActiveToDisabledDoesNotInheritModelStates 测试从活跃状态转为禁用时不继承 ModelStates。
func TestManager_Update_ActiveToDisabledDoesNotInheritModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	// Register an active auth with ModelStates (simulates existing live auth).
	if _, err := m.Register(context.Background(), &Auth{
		ID:       "auth-a2d",
		Provider: "claude",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			"stale-model": {
				Quota: QuotaState{BackoffLevel: 9},
			},
		},
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// File watcher deletes config → synthesizes Disabled=true auth → Update.
	// Even though existing is active, incoming auth is disabled → skip inheritance.
	if _, err := m.Update(context.Background(), &Auth{
		ID:       "auth-a2d",
		Provider: "claude",
		Disabled: true,
		Status:   StatusDisabled,
	}); err != nil {
		t.Fatalf("update auth: %v", err)
	}

	updated, ok := m.GetByID("auth-a2d")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) != 0 {
		t.Fatalf("expected active→disabled transition NOT to inherit ModelStates, got %d entries", len(updated.ModelStates))
	}
}

// TestManager_Update_DisabledToActiveDoesNotInheritStaleModelStates 测试从禁用状态转为活跃时不继承过期的 ModelStates。
func TestManager_Update_DisabledToActiveDoesNotInheritStaleModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	// Register a disabled auth with stale ModelStates.
	if _, err := m.Register(context.Background(), &Auth{
		ID:       "auth-d2a",
		Provider: "claude",
		Disabled: true,
		Status:   StatusDisabled,
		ModelStates: map[string]*ModelState{
			"stale-model": {
				Quota: QuotaState{BackoffLevel: 4},
			},
		},
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// Re-enable: incoming auth is active, existing is disabled → skip inheritance.
	if _, err := m.Update(context.Background(), &Auth{
		ID:       "auth-d2a",
		Provider: "claude",
		Status:   StatusActive,
	}); err != nil {
		t.Fatalf("update auth: %v", err)
	}

	updated, ok := m.GetByID("auth-d2a")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) != 0 {
		t.Fatalf("expected disabled→active transition NOT to inherit stale ModelStates, got %d entries", len(updated.ModelStates))
	}
}

// TestManager_Update_ActiveInheritsModelStates 测试活跃认证更新时继承 ModelStates。
func TestManager_Update_ActiveInheritsModelStates(t *testing.T) {
	m := NewManager(nil, nil, nil)

	model := "active-model"
	backoffLevel := 3

	// Register an active auth with ModelStates.
	if _, err := m.Register(context.Background(), &Auth{
		ID:       "auth-active",
		Provider: "claude",
		Status:   StatusActive,
		ModelStates: map[string]*ModelState{
			model: {
				Quota: QuotaState{BackoffLevel: backoffLevel},
			},
		},
	}); err != nil {
		t.Fatalf("register auth: %v", err)
	}

	// Update with empty ModelStates — both sides active → SHOULD inherit.
	if _, err := m.Update(context.Background(), &Auth{
		ID:       "auth-active",
		Provider: "claude",
		Status:   StatusActive,
	}); err != nil {
		t.Fatalf("update auth: %v", err)
	}

	updated, ok := m.GetByID("auth-active")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if len(updated.ModelStates) == 0 {
		t.Fatalf("expected active auth to inherit ModelStates")
	}
	state := updated.ModelStates[model]
	if state == nil {
		t.Fatalf("expected model state to be present")
	}
	if state.Quota.BackoffLevel != backoffLevel {
		t.Fatalf("expected BackoffLevel to be %d, got %d", backoffLevel, state.Quota.BackoffLevel)
	}
}

// TestManager_UpdateFromImportedFile_ClearsRuntimeState 测试认证文件重导时会清理旧的运行态错误。
// 普通 Update 需要保留活跃认证的 ModelStates；但文件重导代表用户重新提供了凭据，
// 旧的 Unauthorized、重试冷却和失败计数不能继续阻止账号被选择器使用。
func TestManager_UpdateFromImportedFile_ClearsRuntimeState(t *testing.T) {
	m := NewManager(nil, nil, nil)

	createdAt := time.Now().Add(-time.Hour)
	retryAt := time.Now().Add(time.Hour)
	model := "gpt-5.5"
	auth := &Auth{
		ID:             "codex.json",
		Provider:       "codex",
		Status:         StatusError,
		StatusMessage:  "Unauthorized",
		Unavailable:    true,
		CreatedAt:      createdAt,
		NextRetryAfter: retryAt,
		Quota:          QuotaState{Exceeded: true, Reason: "auth", BackoffLevel: 4, NextRecoverAt: retryAt},
		LastError:      &Error{Code: "unauthorized", Message: "Unauthorized", HTTPStatus: 401},
		ModelStates: map[string]*ModelState{
			model: {
				Status:         StatusError,
				StatusMessage:  "Unauthorized",
				Unavailable:    true,
				NextRetryAfter: retryAt,
				LastError:      &Error{Code: "unauthorized", Message: "Unauthorized", HTTPStatus: 401},
				Quota:          QuotaState{Exceeded: true, Reason: "auth", BackoffLevel: 3, NextRecoverAt: retryAt},
				UpdatedAt:      retryAt,
			},
		},
		Metadata: map[string]any{"type": "codex", "email": "old@example.com"},
	}
	if _, errRegister := m.Register(WithSkipPersist(context.Background()), auth); errRegister != nil {
		t.Fatalf("register auth: %v", errRegister)
	}
	m.MarkResult(context.Background(), Result{AuthID: "codex.json", Provider: "codex", Model: model, Success: false})

	imported := &Auth{
		ID:       "codex.json",
		Provider: "codex",
		Metadata: map[string]any{"type": "codex", "email": "new@example.com", "refresh_token": "fresh"},
	}
	if _, errUpdate := m.UpdateFromImportedFile(WithSkipPersist(context.Background()), imported); errUpdate != nil {
		t.Fatalf("UpdateFromImportedFile returned error: %v", errUpdate)
	}

	updated, ok := m.GetByID("codex.json")
	if !ok || updated == nil {
		t.Fatalf("expected auth to be present")
	}
	if updated.Status != StatusActive {
		t.Fatalf("status = %q, want %q", updated.Status, StatusActive)
	}
	if updated.StatusMessage != "" || updated.Unavailable {
		t.Fatalf("expected auth availability to be clean, status_message=%q unavailable=%v", updated.StatusMessage, updated.Unavailable)
	}
	if updated.LastError != nil {
		t.Fatalf("expected LastError to be cleared, got %#v", updated.LastError)
	}
	if !updated.NextRetryAfter.IsZero() || !updated.NextRefreshAfter.IsZero() {
		t.Fatalf("expected retry/refresh cooldowns to be cleared, retry=%v refresh=%v", updated.NextRetryAfter, updated.NextRefreshAfter)
	}
	if updated.Quota != (QuotaState{}) {
		t.Fatalf("expected quota state to be cleared, got %#v", updated.Quota)
	}
	if len(updated.ModelStates) != 0 {
		t.Fatalf("expected ModelStates to be cleared, got %d entries", len(updated.ModelStates))
	}
	if updated.Success != 0 || updated.Failed != 0 {
		t.Fatalf("expected counters to be reset, success=%d failed=%d", updated.Success, updated.Failed)
	}
	var bucketSuccess int64
	var bucketFailed int64
	for _, bucket := range updated.RecentRequestsSnapshot(time.Now()) {
		bucketSuccess += bucket.Success
		bucketFailed += bucket.Failed
	}
	if bucketSuccess != 0 || bucketFailed != 0 {
		t.Fatalf("expected recent request buckets to be reset, success=%d failed=%d", bucketSuccess, bucketFailed)
	}
	if !updated.CreatedAt.Equal(createdAt) {
		t.Fatalf("expected CreatedAt to be preserved, got %v want %v", updated.CreatedAt, createdAt)
	}
	if got, _ := updated.Metadata["email"].(string); got != "new@example.com" {
		t.Fatalf("metadata email = %q, want new@example.com", got)
	}
}
