// auth - auto_refresh_loop_test.go
// 测试自动刷新循环的调度逻辑，验证 nextRefreshCheckAt 函数在各种场景下的行为。
// 覆盖场景包括：禁用认证调度、API Key 跳过、NextRefreshAfter 门控、首选间隔、提供商提前量、刷新评估器回退。
package auth

import (
	"strings"
	"testing"
	"time"
)

// testRefreshEvaluator 是一个始终返回 false 的刷新评估器，用于测试回退逻辑。
type testRefreshEvaluator struct{}

// ShouldRefresh 始终返回 false，表示不需要刷新。
func (testRefreshEvaluator) ShouldRefresh(time.Time, *Auth) bool { return false }

// setRefreshLeadFactory 是测试辅助函数，为指定 provider 设置自定义的刷新提前量工厂函数。
// 测试结束后自动恢复原始值。
func setRefreshLeadFactory(t *testing.T, provider string, factory func() *time.Duration) {
	t.Helper()
	key := strings.ToLower(strings.TrimSpace(provider))
	refreshLeadMu.Lock()
	prev, hadPrev := refreshLeadFactories[key]
	if factory == nil {
		delete(refreshLeadFactories, key)
	} else {
		refreshLeadFactories[key] = factory
	}
	refreshLeadMu.Unlock()
	t.Cleanup(func() {
		refreshLeadMu.Lock()
		if hadPrev {
			refreshLeadFactories[key] = prev
		} else {
			delete(refreshLeadFactories, key)
		}
		refreshLeadMu.Unlock()
	})
}

// TestNextRefreshCheckAt_DisabledUnschedule 测试禁用的认证仍按过期时间减去提前量计算刷新时间。
func TestNextRefreshCheckAt_DisabledUnschedule(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	expiry := now.Add(time.Hour)
	lead := 10 * time.Minute
	setRefreshLeadFactory(t, "disabled-schedule", func() *time.Duration {
		d := lead
		return &d
	})

	auth := &Auth{
		ID:       "a1",
		Provider: "disabled-schedule",
		Disabled: true,
		Status:   StatusDisabled,
		Metadata: map[string]any{
			"email":      "x@example.com",
			"expires_at": expiry.Format(time.RFC3339),
		},
	}

	got, ok := nextRefreshCheckAt(now, auth, 15*time.Minute)
	if !ok {
		t.Fatalf("nextRefreshCheckAt() ok = false, want true")
	}
	want := expiry.Add(-lead)
	if !got.Equal(want) {
		t.Fatalf("nextRefreshCheckAt() = %s, want %s", got, want)
	}
}

// TestNextRefreshCheckAt_APIKeyUnschedule 测试使用 API Key 的认证不需要自动刷新调度。
func TestNextRefreshCheckAt_APIKeyUnschedule(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	auth := &Auth{ID: "a1", Provider: "test", Attributes: map[string]string{"api_key": "k"}}
	if _, ok := nextRefreshCheckAt(now, auth, 15*time.Minute); ok {
		t.Fatalf("nextRefreshCheckAt() ok = true, want false")
	}
}

// TestNextRefreshCheckAt_AccessTokenOnlyUnschedule 测试 AT-only 凭据不进入自动刷新调度。
//
// 这类凭据仍会参与真实请求调度，但没有 refresh_token，后台刷新没有可执行动作；
// 是否失效由真实请求的 Unauthorized 结果来标记。
func TestNextRefreshCheckAt_AccessTokenOnlyUnschedule(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	auth := &Auth{
		ID:       "codex-at-only",
		Provider: "codex",
		Attributes: map[string]string{
			"credential_mode": "access_token_only",
			"refreshable":     "false",
		},
		Metadata: map[string]any{
			"email":             "x@example.com",
			"access_token":      "access-token",
			"access_token_only": true,
			"expired":           now.Add(time.Hour).Format(time.RFC3339),
		},
	}
	if _, ok := nextRefreshCheckAt(now, auth, 15*time.Minute); ok {
		t.Fatalf("nextRefreshCheckAt() ok = true, want false")
	}
}

// TestNextRefreshCheckAt_NextRefreshAfterGate 测试 NextRefreshAfter 门控时间优先于其他计算。
func TestNextRefreshCheckAt_NextRefreshAfterGate(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	nextAfter := now.Add(30 * time.Minute)
	auth := &Auth{
		ID:               "a1",
		Provider:         "test",
		NextRefreshAfter: nextAfter,
		Metadata:         map[string]any{"email": "x@example.com"},
	}
	got, ok := nextRefreshCheckAt(now, auth, 15*time.Minute)
	if !ok {
		t.Fatalf("nextRefreshCheckAt() ok = false, want true")
	}
	if !got.Equal(nextAfter) {
		t.Fatalf("nextRefreshCheckAt() = %s, want %s", got, nextAfter)
	}
}

// TestNextRefreshCheckAt_PreferredInterval_PicksEarliestCandidate 测试首选间隔与过期提前量
// 取最早时间作为刷新检查时间。
func TestNextRefreshCheckAt_PreferredInterval_PicksEarliestCandidate(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	expiry := now.Add(20 * time.Minute)
	auth := &Auth{
		ID:              "a1",
		Provider:        "test",
		LastRefreshedAt: now,
		Metadata: map[string]any{
			"email":                    "x@example.com",
			"expires_at":               expiry.Format(time.RFC3339),
			"refresh_interval_seconds": 900, // 15m
		},
	}
	got, ok := nextRefreshCheckAt(now, auth, 15*time.Minute)
	if !ok {
		t.Fatalf("nextRefreshCheckAt() ok = false, want true")
	}
	want := expiry.Add(-15 * time.Minute)
	if !got.Equal(want) {
		t.Fatalf("nextRefreshCheckAt() = %s, want %s", got, want)
	}
}

// TestNextRefreshCheckAt_ProviderLead_Expiry 测试提供商自定义提前量应用于过期时间计算。
func TestNextRefreshCheckAt_ProviderLead_Expiry(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	expiry := now.Add(time.Hour)
	lead := 10 * time.Minute
	setRefreshLeadFactory(t, "provider-lead-expiry", func() *time.Duration {
		d := lead
		return &d
	})

	auth := &Auth{
		ID:       "a1",
		Provider: "provider-lead-expiry",
		Metadata: map[string]any{
			"email":      "x@example.com",
			"expires_at": expiry.Format(time.RFC3339),
		},
	}

	got, ok := nextRefreshCheckAt(now, auth, 15*time.Minute)
	if !ok {
		t.Fatalf("nextRefreshCheckAt() ok = false, want true")
	}
	want := expiry.Add(-lead)
	if !got.Equal(want) {
		t.Fatalf("nextRefreshCheckAt() = %s, want %s", got, want)
	}
}

// TestNextRefreshCheckAt_RefreshEvaluatorFallback 测试当存在刷新评估器但无过期时间时，
// 使用首选间隔作为回退。
func TestNextRefreshCheckAt_RefreshEvaluatorFallback(t *testing.T) {
	now := time.Date(2026, 4, 12, 0, 0, 0, 0, time.UTC)
	interval := 15 * time.Minute
	auth := &Auth{
		ID:       "a1",
		Provider: "test",
		Metadata: map[string]any{"email": "x@example.com"},
		Runtime:  testRefreshEvaluator{},
	}
	got, ok := nextRefreshCheckAt(now, auth, interval)
	if !ok {
		t.Fatalf("nextRefreshCheckAt() ok = false, want true")
	}
	want := now.Add(interval)
	if !got.Equal(want) {
		t.Fatalf("nextRefreshCheckAt() = %s, want %s", got, want)
	}
}
