// auth - conductor_executor_replace_test.go
// 执行器替换与会话清理测试
// 验证 Manager 的执行器注册替换机制：
// - 注册同名执行器时，旧执行器的执行会话被关闭
// - Executor 方法正确返回已注册的执行器
package auth

import (
	"context"
	"net/http"
	"sync"
	"testing"

	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v7/sdk/cliproxy/executor"
)

// replaceAwareExecutor 是支持会话关闭跟踪的测试执行器。
// 记录所有被关闭的会话 ID，用于验证执行器替换时的清理行为。
type replaceAwareExecutor struct {
	id string

	mu               sync.Mutex
	closedSessionIDs []string
}

// Identifier 返回执行器的唯一标识符。
func (e *replaceAwareExecutor) Identifier() string {
	return e.id
}

// Execute 执行同步请求，测试中返回空结果。
func (e *replaceAwareExecutor) Execute(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

// ExecuteStream 执行流式请求，测试中返回空的已关闭通道。
func (e *replaceAwareExecutor) ExecuteStream(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (*cliproxyexecutor.StreamResult, error) {
	ch := make(chan cliproxyexecutor.StreamChunk)
	close(ch)
	return &cliproxyexecutor.StreamResult{Chunks: ch}, nil
}

// Refresh 刷新认证，测试中直接返回原认证。
func (e *replaceAwareExecutor) Refresh(_ context.Context, auth *Auth) (*Auth, error) {
	return auth, nil
}

// CountTokens 执行 Token 计数，测试中返回空结果。
func (e *replaceAwareExecutor) CountTokens(context.Context, *Auth, cliproxyexecutor.Request, cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	return cliproxyexecutor.Response{}, nil
}

// HttpRequest 执行 HTTP 请求，测试中返回 nil。
func (e *replaceAwareExecutor) HttpRequest(context.Context, *Auth, *http.Request) (*http.Response, error) {
	return nil, nil
}

// CloseExecutionSession 关闭执行会话，将会话 ID 记录到 closedSessionIDs 列表中。
func (e *replaceAwareExecutor) CloseExecutionSession(sessionID string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.closedSessionIDs = append(e.closedSessionIDs, sessionID)
}

// ClosedSessionIDs 返回所有被关闭的会话 ID 列表（线程安全副本）。
func (e *replaceAwareExecutor) ClosedSessionIDs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, len(e.closedSessionIDs))
	copy(out, e.closedSessionIDs)
	return out
}

// TestManagerRegisterExecutorClosesReplacedExecutionSessions 验证：
// 当注册同名执行器时，旧执行器会收到 CloseAllExecutionSessionsID 信号，
// 而新注册的执行器不受影响。
func TestManagerRegisterExecutorClosesReplacedExecutionSessions(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil, nil, nil)
	replaced := &replaceAwareExecutor{id: "codex"}
	current := &replaceAwareExecutor{id: "codex"}

	manager.RegisterExecutor(replaced)
	manager.RegisterExecutor(current)

	closed := replaced.ClosedSessionIDs()
	if len(closed) != 1 {
		t.Fatalf("expected replaced executor close calls = 1, got %d", len(closed))
	}
	if closed[0] != CloseAllExecutionSessionsID {
		t.Fatalf("expected close marker %q, got %q", CloseAllExecutionSessionsID, closed[0])
	}
	if len(current.ClosedSessionIDs()) != 0 {
		t.Fatalf("expected current executor to stay open")
	}
}

// TestManagerExecutorReturnsRegisteredExecutor 验证：
// Executor 方法能正确返回已注册的执行器（大小写不敏感），
// 未注册的 provider 返回 false。
func TestManagerExecutorReturnsRegisteredExecutor(t *testing.T) {
	t.Parallel()

	manager := NewManager(nil, nil, nil)
	current := &replaceAwareExecutor{id: "codex"}
	manager.RegisterExecutor(current)

	resolved, okResolved := manager.Executor("CODEX")
	if !okResolved {
		t.Fatal("expected registered executor to be found")
	}
	resolvedExecutor, okResolvedExecutor := resolved.(*replaceAwareExecutor)
	if !okResolvedExecutor {
		t.Fatalf("expected resolved executor type %T, got %T", current, resolved)
	}
	if resolvedExecutor != current {
		t.Fatal("expected resolved executor to match registered executor")
	}

	_, okMissing := manager.Executor("unknown")
	if okMissing {
		t.Fatal("expected unknown provider lookup to fail")
	}
}
