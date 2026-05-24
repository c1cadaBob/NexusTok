// auth - home_websocket_reuse_test.go
// Home WebSocket 连接复用测试
// 验证 Home 集群模式下的 WebSocket 连接复用机制：
// - 固定认证（pinned auth）的 WebSocket 连接复用
// - 会话级别的认证隔离
// - 已尝试认证的排除
// - 首次 Home 尝试后的认证清理
// - 非 WebSocket 认证的复用限制
// - Home 禁用时的清理行为
// - 会话关闭时的清理行为
package auth

import (
	"context"
	"errors"
	"net/http"
	"testing"

	internalconfig "github.com/router-for-me/CLIProxyAPI/v7/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

// TestPickNextViaHomeReusesPinnedWebsocketAuthWithoutHomeDispatch 验证：
// 当使用固定认证（pinned auth）且该认证支持 WebSocket 时，
// pickNextViaHome 能够复用该认证而无需通过 Home 集群调度。
func TestPickNextViaHomeReusesPinnedWebsocketAuthWithoutHomeDispatch(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	manager.RegisterExecutor(schedulerTestExecutor{})

	auth := &Auth{
		ID:       "home-auth-1",
		Provider: "test",
		Status:   StatusActive,
		Attributes: map[string]string{
			"websockets":                  "true",
			homeUpstreamModelAttributeKey: "upstream-model",
		},
		Metadata: map[string]any{"email": "home@example.com"},
	}
	auth.EnsureIndex()
	manager.rememberHomeRuntimeAuth("session-1", auth)
	cachedAuth, ok := manager.GetExecutionSessionAuthByID("session-1", "home-auth-1")
	if !ok || cachedAuth == nil || !authWebsocketsEnabled(cachedAuth) {
		t.Fatalf("GetExecutionSessionAuthByID() did not expose remembered websocket home auth: auth=%#v ok=%v", cachedAuth, ok)
	}

	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	opts := cliproxyexecutor.Options{
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "session-1",
			cliproxyexecutor.PinnedAuthMetadataKey:       "home-auth-1",
		},
		Headers: http.Header{"Authorization": {"Bearer client-key"}},
	}

	got, executor, provider, errPick := manager.pickNextViaHome(ctx, "gpt-5.4", opts, nil)
	if errPick != nil {
		t.Fatalf("pickNextViaHome() error = %v", errPick)
	}
	if got == nil || got.ID != "home-auth-1" {
		t.Fatalf("pickNextViaHome() auth = %#v, want home-auth-1", got)
	}
	if executor == nil {
		t.Fatal("pickNextViaHome() executor is nil")
	}
	if provider != "test" {
		t.Fatalf("pickNextViaHome() provider = %q, want test", provider)
	}
}

// TestPickNextViaHomeKeepsSameAuthIDPayloadSessionScoped 验证：
// 同一认证 ID 在不同会话中可以有不同的载荷（upstream model），
// 会话级别的认证隔离确保各会话独立。
func TestPickNextViaHomeKeepsSameAuthIDPayloadSessionScoped(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	manager.RegisterExecutor(schedulerTestExecutor{})

	manager.rememberHomeRuntimeAuth("session-1", &Auth{
		ID:       "home-auth-1",
		Provider: "test",
		Status:   StatusActive,
		Attributes: map[string]string{
			"websockets":                  "true",
			homeUpstreamModelAttributeKey: "upstream-model-a",
		},
	})
	manager.rememberHomeRuntimeAuth("session-2", &Auth{
		ID:       "home-auth-1",
		Provider: "test",
		Status:   StatusActive,
		Attributes: map[string]string{
			"websockets":                  "true",
			homeUpstreamModelAttributeKey: "upstream-model-b",
		},
	})

	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	optsSession1 := cliproxyexecutor.Options{
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "session-1",
			cliproxyexecutor.PinnedAuthMetadataKey:       "home-auth-1",
		},
	}
	optsSession2 := cliproxyexecutor.Options{
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "session-2",
			cliproxyexecutor.PinnedAuthMetadataKey:       "home-auth-1",
		},
	}

	gotSession1, _, _, errSession1 := manager.pickNextViaHome(ctx, "gpt-5.4", optsSession1, nil)
	if errSession1 != nil {
		t.Fatalf("pickNextViaHome(session-1) error = %v", errSession1)
	}
	if got := gotSession1.Attributes[homeUpstreamModelAttributeKey]; got != "upstream-model-a" {
		t.Fatalf("pickNextViaHome(session-1) upstream model = %q, want upstream-model-a", got)
	}

	gotSession2, _, _, errSession2 := manager.pickNextViaHome(ctx, "gpt-5.4", optsSession2, nil)
	if errSession2 != nil {
		t.Fatalf("pickNextViaHome(session-2) error = %v", errSession2)
	}
	if got := gotSession2.Attributes[homeUpstreamModelAttributeKey]; got != "upstream-model-b" {
		t.Fatalf("pickNextViaHome(session-2) upstream model = %q, want upstream-model-b", got)
	}
}

// TestPickNextViaHomeDoesNotReuseTriedPinnedWebsocketAuth 验证：
// 已尝试过的固定认证不会被复用，返回 home_unavailable 错误。
func TestPickNextViaHomeDoesNotReuseTriedPinnedWebsocketAuth(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	manager.RegisterExecutor(schedulerTestExecutor{})

	auth := &Auth{
		ID:       "home-auth-1",
		Provider: "test",
		Status:   StatusActive,
		Attributes: map[string]string{
			"websockets": "true",
		},
	}
	manager.rememberHomeRuntimeAuth("session-1", auth)

	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	opts := cliproxyexecutor.Options{
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "session-1",
			cliproxyexecutor.PinnedAuthMetadataKey:       "home-auth-1",
		},
	}
	tried := map[string]struct{}{"home-auth-1": {}}

	got, executor, provider, errPick := manager.pickNextViaHome(ctx, "gpt-5.4", opts, tried)
	if errPick == nil {
		t.Fatal("pickNextViaHome() error is nil, want home unavailable error")
	}
	var authErr *Error
	if !errors.As(errPick, &authErr) || authErr.Code != "home_unavailable" {
		t.Fatalf("pickNextViaHome() error = %v, want home_unavailable", errPick)
	}
	if got != nil || executor != nil || provider != "" {
		t.Fatalf("pickNextViaHome() reused tried auth: auth=%#v executor=%#v provider=%q", got, executor, provider)
	}
}

// TestPickNextViaHomeDoesNotReusePinnedWebsocketAuthAfterFirstHomeAttempt 验证：
// 首次 Home 尝试后，固定认证不会被复用。
func TestPickNextViaHomeDoesNotReusePinnedWebsocketAuthAfterFirstHomeAttempt(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	manager.RegisterExecutor(schedulerTestExecutor{})

	auth := &Auth{
		ID:       "home-auth-1",
		Provider: "test",
		Status:   StatusActive,
		Attributes: map[string]string{
			"websockets": "true",
		},
	}
	manager.rememberHomeRuntimeAuth("session-1", auth)

	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	opts := withHomeAuthCount(cliproxyexecutor.Options{
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "session-1",
			cliproxyexecutor.PinnedAuthMetadataKey:       "home-auth-1",
		},
	}, 2)

	got, executor, provider, errPick := manager.pickNextViaHome(ctx, "gpt-5.4", opts, nil)
	if errPick == nil {
		t.Fatal("pickNextViaHome() error is nil, want home unavailable error")
	}
	var authErr *Error
	if !errors.As(errPick, &authErr) || authErr.Code != "home_unavailable" {
		t.Fatalf("pickNextViaHome() error = %v, want home_unavailable", errPick)
	}
	if got != nil || executor != nil || provider != "" {
		t.Fatalf("pickNextViaHome() reused auth after first home attempt: auth=%#v executor=%#v provider=%q", got, executor, provider)
	}
}

// TestPickNextViaHomeDoesNotReusePinnedNonWebsocketAuth 验证：
// 不支持 WebSocket 的固定认证不会被复用。
func TestPickNextViaHomeDoesNotReusePinnedNonWebsocketAuth(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	manager.RegisterExecutor(schedulerTestExecutor{})

	manager.mu.Lock()
	manager.homeRuntimeAuths["session-1"] = map[string]*Auth{
		"home-auth-1": &Auth{
			ID:       "home-auth-1",
			Provider: "test",
			Status:   StatusActive,
		},
	}
	manager.mu.Unlock()

	ctx := cliproxyexecutor.WithDownstreamWebsocket(context.Background())
	opts := cliproxyexecutor.Options{
		Metadata: map[string]any{
			cliproxyexecutor.ExecutionSessionMetadataKey: "session-1",
			cliproxyexecutor.PinnedAuthMetadataKey:       "home-auth-1",
		},
		Headers: http.Header{"Authorization": {"Bearer client-key"}},
	}

	got, executor, provider, errPick := manager.pickNextViaHome(ctx, "gpt-5.4", opts, nil)
	if errPick == nil {
		t.Fatal("pickNextViaHome() error is nil, want home unavailable error")
	}
	var authErr *Error
	if !errors.As(errPick, &authErr) || authErr.Code != "home_unavailable" {
		t.Fatalf("pickNextViaHome() error = %v, want home_unavailable", errPick)
	}
	if got != nil || executor != nil || provider != "" {
		t.Fatalf("pickNextViaHome() reused non-websocket auth: auth=%#v executor=%#v provider=%q", got, executor, provider)
	}
}

// TestHomeRuntimeAuthsClearWhenHomeDisabled 验证：
// 当 Home 功能被禁用时，所有已记住的 Home 运行时认证被清除。
func TestHomeRuntimeAuthsClearWhenHomeDisabled(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	manager.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
	manager.rememberHomeRuntimeAuth("session-1", &Auth{
		ID:       "home-auth-1",
		Provider: "test",
		Attributes: map[string]string{
			"websockets": "true",
		},
	})

	if _, ok := manager.GetExecutionSessionAuthByID("session-1", "home-auth-1"); !ok {
		t.Fatal("expected remembered home auth before disabling home")
	}

	manager.SetConfig(&internalconfig.Config{})
	if _, ok := manager.GetExecutionSessionAuthByID("session-1", "home-auth-1"); ok {
		t.Fatal("remembered home auth was not cleared when home was disabled")
	}
}

// TestCloseExecutionSessionClearsHomeRuntimeAuthForSession 验证：
// 关闭执行会话时，该会话的 Home 运行时认证被清除，
// 不影响其他会话的认证。
func TestCloseExecutionSessionClearsHomeRuntimeAuthForSession(t *testing.T) {
	manager := NewManager(nil, nil, nil)
	auth := &Auth{
		ID:       "home-auth-1",
		Provider: "test",
		Attributes: map[string]string{
			"websockets": "true",
		},
	}

	manager.rememberHomeRuntimeAuth("session-1", auth)
	manager.rememberHomeRuntimeAuth("session-2", auth)

	manager.CloseExecutionSession("session-1")
	if _, ok := manager.GetExecutionSessionAuthByID("session-1", "home-auth-1"); ok {
		t.Fatal("home auth for closed session was not cleared")
	}
	if _, ok := manager.GetExecutionSessionAuthByID("session-2", "home-auth-1"); !ok {
		t.Fatal("home auth for another session was cleared")
	}

	manager.CloseExecutionSession("session-2")
	if _, ok := manager.GetExecutionSessionAuthByID("session-2", "home-auth-1"); ok {
		t.Fatal("home auth was not cleared when its last session closed")
	}
}
