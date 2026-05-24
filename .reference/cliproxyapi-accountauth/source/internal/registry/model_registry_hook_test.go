// registry - model_registry_hook_test.go
// 模型注册表 Hook 回调机制测试
// 验证 ModelRegistry 的 Hook 机制能够正确触发回调：
// - 注册客户端时触发 OnModelsRegistered
// - 注销客户端时触发 OnModelsUnregistered
// - Hook 回调不应阻塞注册/注销操作（异步执行）
// - Hook 回调中的 panic 不应影响注册表的正常工作
package registry

import (
	"context"
	"sync"
	"testing"
	"time"
)

// newTestModelRegistry 创建用于测试的空 ModelRegistry 实例。
func newTestModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:           make(map[string]*ModelRegistration),
		clientModels:     make(map[string][]string),
		clientModelInfos: make(map[string]map[string]*ModelInfo),
		clientProviders:  make(map[string]string),
		mutex:            &sync.RWMutex{},
	}
}

// registeredCall 记录 OnModelsRegistered 回调的调用参数。
type registeredCall struct {
	provider string
	clientID string
	models   []*ModelInfo
}

// unregisteredCall 记录 OnModelsUnregistered 回调的调用参数。
type unregisteredCall struct {
	provider string
	clientID string
}

// capturingHook 是用于测试的 Hook 实现，
// 通过 channel 捕获回调调用，便于测试异步触发。
type capturingHook struct {
	registeredCh   chan registeredCall
	unregisteredCh chan unregisteredCall
}

func (h *capturingHook) OnModelsRegistered(ctx context.Context, provider, clientID string, models []*ModelInfo) {
	h.registeredCh <- registeredCall{provider: provider, clientID: clientID, models: models}
}

func (h *capturingHook) OnModelsUnregistered(ctx context.Context, provider, clientID string) {
	h.unregisteredCh <- unregisteredCall{provider: provider, clientID: clientID}
}

// TestModelRegistryHook_OnModelsRegisteredCalled 验证注册客户端时
// OnModelsRegistered Hook 被正确调用，参数（provider、clientID、models）正确传递。
func TestModelRegistryHook_OnModelsRegisteredCalled(t *testing.T) {
	r := newTestModelRegistry()
	hook := &capturingHook{
		registeredCh:   make(chan registeredCall, 1),
		unregisteredCh: make(chan unregisteredCall, 1),
	}
	r.SetHook(hook)

	inputModels := []*ModelInfo{
		{ID: "m1", DisplayName: "Model One"},
		{ID: "m2", DisplayName: "Model Two"},
	}
	r.RegisterClient("client-1", "OpenAI", inputModels)

	select {
	case call := <-hook.registeredCh:
		if call.provider != "openai" {
			t.Fatalf("provider mismatch: got %q, want %q", call.provider, "openai")
		}
		if call.clientID != "client-1" {
			t.Fatalf("clientID mismatch: got %q, want %q", call.clientID, "client-1")
		}
		if len(call.models) != 2 {
			t.Fatalf("models length mismatch: got %d, want %d", len(call.models), 2)
		}
		if call.models[0] == nil || call.models[0].ID != "m1" {
			t.Fatalf("models[0] mismatch: got %#v, want ID=%q", call.models[0], "m1")
		}
		if call.models[1] == nil || call.models[1].ID != "m2" {
			t.Fatalf("models[1] mismatch: got %#v, want ID=%q", call.models[1], "m2")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnModelsRegistered hook call")
	}
}

// TestModelRegistryHook_OnModelsUnregisteredCalled 验证注销客户端时
// OnModelsUnregistered Hook 被正确调用。
func TestModelRegistryHook_OnModelsUnregisteredCalled(t *testing.T) {
	r := newTestModelRegistry()
	hook := &capturingHook{
		registeredCh:   make(chan registeredCall, 1),
		unregisteredCh: make(chan unregisteredCall, 1),
	}
	r.SetHook(hook)

	r.RegisterClient("client-1", "OpenAI", []*ModelInfo{{ID: "m1"}})
	select {
	case <-hook.registeredCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnModelsRegistered hook call")
	}

	r.UnregisterClient("client-1")

	select {
	case call := <-hook.unregisteredCh:
		if call.provider != "openai" {
			t.Fatalf("provider mismatch: got %q, want %q", call.provider, "openai")
		}
		if call.clientID != "client-1" {
			t.Fatalf("clientID mismatch: got %q, want %q", call.clientID, "client-1")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnModelsUnregistered hook call")
	}
}

// blockingHook 是用于测试的阻塞 Hook 实现，
// 模拟 Hook 回调长时间阻塞的场景。
type blockingHook struct {
	started chan struct{}
	unblock chan struct{}
}

func (h *blockingHook) OnModelsRegistered(ctx context.Context, provider, clientID string, models []*ModelInfo) {
	select {
	case <-h.started:
	default:
		close(h.started)
	}
	<-h.unblock
}

func (h *blockingHook) OnModelsUnregistered(ctx context.Context, provider, clientID string) {}

// TestModelRegistryHook_DoesNotBlockRegisterClient 验证 Hook 回调
// 不会阻塞 RegisterClient 操作，注册操作应在 Hook 回调完成前返回。
func TestModelRegistryHook_DoesNotBlockRegisterClient(t *testing.T) {
	r := newTestModelRegistry()
	hook := &blockingHook{
		started: make(chan struct{}),
		unblock: make(chan struct{}),
	}
	r.SetHook(hook)
	defer close(hook.unblock)

	done := make(chan struct{})
	go func() {
		r.RegisterClient("client-1", "OpenAI", []*ModelInfo{{ID: "m1"}})
		close(done)
	}()

	select {
	case <-hook.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for hook to start")
	}

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("RegisterClient appears to be blocked by hook")
	}

	if !r.ClientSupportsModel("client-1", "m1") {
		t.Fatal("model registration failed; expected client to support model")
	}
}

// panicHook 是用于测试的 panic Hook 实现，
// 模拟 Hook 回调中发生 panic 的场景。
type panicHook struct {
	registeredCalled   chan struct{}
	unregisteredCalled chan struct{}
}

func (h *panicHook) OnModelsRegistered(ctx context.Context, provider, clientID string, models []*ModelInfo) {
	if h.registeredCalled != nil {
		h.registeredCalled <- struct{}{}
	}
	panic("boom")
}

func (h *panicHook) OnModelsUnregistered(ctx context.Context, provider, clientID string) {
	if h.unregisteredCalled != nil {
		h.unregisteredCalled <- struct{}{}
	}
	panic("boom")
}

// TestModelRegistryHook_PanicDoesNotAffectRegistry 验证 Hook 回调中
// 发生 panic 时，注册表的正常操作不受影响，模型注册/注销仍然成功。
func TestModelRegistryHook_PanicDoesNotAffectRegistry(t *testing.T) {
	r := newTestModelRegistry()
	hook := &panicHook{
		registeredCalled:   make(chan struct{}, 1),
		unregisteredCalled: make(chan struct{}, 1),
	}
	r.SetHook(hook)

	r.RegisterClient("client-1", "OpenAI", []*ModelInfo{{ID: "m1"}})

	select {
	case <-hook.registeredCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnModelsRegistered hook call")
	}

	if !r.ClientSupportsModel("client-1", "m1") {
		t.Fatal("model registration failed; expected client to support model")
	}

	r.UnregisterClient("client-1")

	select {
	case <-hook.unregisteredCalled:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for OnModelsUnregistered hook call")
	}
}
