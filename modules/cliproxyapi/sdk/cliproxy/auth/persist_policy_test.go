// auth - persist_policy_test.go
// 测试持久化策略机制，验证 WithSkipPersist 标记能正确阻止 Manager 的 Update 和 Register 操作触发持久化。
package auth

import (
	"context"
	"sync/atomic"
	"testing"
)

// countingStore 是一个用于测试的计数型存储实现，记录 Save 方法的调用次数。
type countingStore struct {
	// saveCount 记录 Save 方法被调用的次数
	saveCount atomic.Int32
}

// List 返回空列表，满足 Store 接口要求。
func (s *countingStore) List(context.Context) ([]*Auth, error) { return nil, nil }

// Save 递增 saveCount 计数器，模拟持久化操作。
func (s *countingStore) Save(context.Context, *Auth) (string, error) {
	s.saveCount.Add(1)
	return "", nil
}

// Delete 空操作，满足 Store 接口要求。
func (s *countingStore) Delete(context.Context, string) error { return nil }

// TestWithSkipPersist_DisablesUpdatePersistence 测试 WithSkipPersist 标记能阻止 Update 操作触发持久化。
func TestWithSkipPersist_DisablesUpdatePersistence(t *testing.T) {
	store := &countingStore{}
	mgr := NewManager(store, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{"type": "antigravity"},
	}

	if _, err := mgr.Update(context.Background(), auth); err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if got := store.saveCount.Load(); got != 1 {
		t.Fatalf("expected 1 Save call, got %d", got)
	}

	ctxSkip := WithSkipPersist(context.Background())
	if _, err := mgr.Update(ctxSkip, auth); err != nil {
		t.Fatalf("Update(skipPersist) returned error: %v", err)
	}
	if got := store.saveCount.Load(); got != 1 {
		t.Fatalf("expected Save call count to remain 1, got %d", got)
	}
}

// TestWithSkipPersist_DisablesRegisterPersistence 测试 WithSkipPersist 标记能阻止 Register 操作触发持久化。
func TestWithSkipPersist_DisablesRegisterPersistence(t *testing.T) {
	store := &countingStore{}
	mgr := NewManager(store, nil, nil)
	auth := &Auth{
		ID:       "auth-1",
		Provider: "antigravity",
		Metadata: map[string]any{"type": "antigravity"},
	}

	if _, err := mgr.Register(WithSkipPersist(context.Background()), auth); err != nil {
		t.Fatalf("Register(skipPersist) returned error: %v", err)
	}
	if got := store.saveCount.Load(); got != 0 {
		t.Fatalf("expected 0 Save calls, got %d", got)
	}
}
