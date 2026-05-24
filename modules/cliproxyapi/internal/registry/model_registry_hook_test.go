// registry - model_registry_hook_test.go
// 该文件测试模型注册表的钩子（Hook）机制。
// 测试覆盖了注册/注销钩子的调用、钩子不阻塞注册操作、钩子 panic 不影响注册表等场景。

package registry

import (
	"context"
	"sync"
	"testing"
	"time"
)

// newTestModelRegistry 创建用于测试的空模型注册表实例。
func newTestModelRegistry() *ModelRegistry {
	return &ModelRegistry{
		models:           make(map[string]*ModelRegistration),
		clientModels:     make(map[string][]string),
		clientModelInfos: make(map[string]map[string]*ModelInfo),
		clientProviders:  make(map[string]string),
		mutex:            &sync.RWMutex{},
	}
}

// registeredCall 记录 OnModelsRegistered 钩子的调用参数。
type registeredCall struct {
	provider string
	clientID string
	models   []*ModelInfo
}

// unregisteredCall 记录 OnModelsUnregistered 钩子的调用参数。
type unregisteredCall struct {
	provider string
	clientID string
}

// capturingHook 是一个捕获钩子调用参数的测试替身。
type capturingHook struct {
	registeredCh   chan registeredCall
	unregisteredCh chan unregisteredCall
}

// OnModelsRegistered 将注册事件推送到 registeredCh 通道。
func (h *capturingHook) OnModelsRegistered(ctx context.Context, provider, clientID string, models []*ModelInfo) {
	h.registeredCh <- registeredCall{provider: provider, clientID: clientID, models: models}
}

// OnModelsUnregistered 将注销事件推送到 unregisteredCh 通道。
func (h *capturingHook) OnModelsUnregistered(ctx context.Context, provider, clientID string) {
	h.unregisteredCh <- unregisteredCall{provider: provider, clientID: clientID}
}

// TestModelRegistryHook_OnModelsRegisteredCalled 测试注册模型时 OnModelsRegistered 钩子被正确调用。
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

// TestModelRegistryHook_OnModelsUnregisteredCalled 测试注销模型时 OnModelsUnregistered 钩子被正确调用。
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

// blockingHook 是一个会阻塞的测试钩子，用于验证钩子不阻塞注册操作。
type blockingHook struct {
	started chan struct{}
	unblock chan struct{}
}

// OnModelsRegistered 阻塞直到 unblock 通道被关闭，模拟慢钩子。
func (h *blockingHook) OnModelsRegistered(ctx context.Context, provider, clientID string, models []*ModelInfo) {
	select {
	case <-h.started:
	default:
		close(h.started)
	}
	<-h.unblock
}

// OnModelsUnregistered 空实现，满足接口要求。
func (h *blockingHook) OnModelsUnregistered(ctx context.Context, provider, clientID string) {}

// TestModelRegistryHook_DoesNotBlockRegisterClient 测试钩子执行不会阻塞 RegisterClient 操作。
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

// panicHook 是一个会 panic 的测试钩子，用于验证钩子 panic 不影响注册表。
type panicHook struct {
	registeredCalled   chan struct{}
	unregisteredCalled chan struct{}
}

// OnModelsRegistered 发送通知后触发 panic。
func (h *panicHook) OnModelsRegistered(ctx context.Context, provider, clientID string, models []*ModelInfo) {
	if h.registeredCalled != nil {
		h.registeredCalled <- struct{}{}
	}
	panic("boom")
}

// OnModelsUnregistered 发送通知后触发 panic。
func (h *panicHook) OnModelsUnregistered(ctx context.Context, provider, clientID string) {
	if h.unregisteredCalled != nil {
		h.unregisteredCalled <- struct{}{}
	}
	panic("boom")
}

// TestModelRegistryHook_PanicDoesNotAffectRegistry 测试钩子中的 panic 不会影响注册表的正常工作。
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
