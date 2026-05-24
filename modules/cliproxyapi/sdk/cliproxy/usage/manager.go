// usage - manager.go
// 实现使用量统计管理器，维护一个使用量记录队列并将其分发给已注册的插件。
// 提供全局默认管理器实例和便捷的发布/注册函数。
package usage

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// Record 包含单次提供商请求的使用量统计数据。
type Record struct {
	// Provider 是提供商名称
	Provider  string
	// Model 是模型名称
	Model     string
	// Alias 是客户端请求的模型别名
	Alias     string
	// APIKey 是使用的 API 密钥
	APIKey    string
	// AuthID 是认证条目 ID
	AuthID    string
	// AuthIndex 是认证索引
	AuthIndex string
	// AuthType 是认证类型
	AuthType  string
	// Source 是请求来源标识
	Source    string
	// ReasoningEffort 存储客户端请求的思维级别，用于请求事件日志
	ReasoningEffort string
	// RequestedAt 是请求发起时间
	RequestedAt     time.Time
	// Latency 是请求延迟
	Latency         time.Duration
	// Failed 指示请求是否失败
	Failed          bool
	// Fail 包含 HTTP 失败元数据
	Fail            Failure
	// Detail 包含 token 使用量明细
	Detail          Detail
	// ResponseHeaders 存储上游响应头的快照，供使用量接收器使用
	ResponseHeaders http.Header
}

// Failure 包含上游请求尝试的 HTTP 失败元数据。
type Failure struct {
	// StatusCode 是 HTTP 状态码
	StatusCode int
	// Body 是响应体内容
	Body       string
}

// Detail 包含 token 使用量明细分解。
type Detail struct {
	InputTokens         int64
	OutputTokens        int64
	ReasoningTokens     int64
	CachedTokens        int64
	CacheReadTokens     int64
	CacheCreationTokens int64
	TotalTokens         int64
}

// requestedModelAliasContextKey 是用于存储客户端请求模型名称的 context 键类型。
type requestedModelAliasContextKey struct{}
// reasoningEffortContextKey 是用于存储客户端请求推理努力级别的 context 键类型。
type reasoningEffortContextKey struct{}

// WithRequestedModelAlias stores the client-requested model name for usage sinks.
func WithRequestedModelAlias(ctx context.Context, alias string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return ctx
	}
	return context.WithValue(ctx, requestedModelAliasContextKey{}, alias)
}

// RequestedModelAliasFromContext returns the client-requested model name stored in ctx.
func RequestedModelAliasFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	raw := ctx.Value(requestedModelAliasContextKey{})
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []byte:
		return strings.TrimSpace(string(value))
	default:
		return ""
	}
}

// WithReasoningEffort stores the client-requested reasoning effort for usage sinks.
func WithReasoningEffort(ctx context.Context, effort string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return ctx
	}
	return context.WithValue(ctx, reasoningEffortContextKey{}, effort)
}

// ReasoningEffortFromContext returns the client-requested reasoning effort stored in ctx.
func ReasoningEffortFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	raw := ctx.Value(reasoningEffortContextKey{})
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []byte:
		return strings.TrimSpace(string(value))
	default:
		return ""
	}
}

// Plugin 接口消费代理运行时发出的使用量记录。
type Plugin interface {
	// HandleUsage 处理一条使用量记录。
	HandleUsage(ctx context.Context, record Record)
}

// queueItem 是队列中的使用量记录项，包含 context 和记录数据。
type queueItem struct {
	ctx    context.Context
	record Record
}

// Manager 维护使用量记录队列并将其分发给已注册的插件。
type Manager struct {
	// once 确保 Start 方法只执行一次
	once     sync.Once
	// stopOnce 确保 Stop 方法只执行一次
	stopOnce sync.Once
	// cancel 用于取消后台工作协程
	cancel   context.CancelFunc

	// mu 保护队列的并发访问
	mu     sync.Mutex
	// cond 用于工作协程等待新队列项
	cond   *sync.Cond
	// queue 是待处理的使用量记录队列
	queue  []queueItem
	// closed 标记管理器是否已关闭
	closed bool

	// pluginsMu 保护插件列表的并发访问
	pluginsMu sync.RWMutex
	// plugins 是已注册的使用量插件列表
	plugins   []Plugin
}

// NewManager 构造一个带有缓冲队列的管理器。
func NewManager(buffer int) *Manager {
	m := &Manager{}
	m.cond = sync.NewCond(&m.mu)
	return m
}

// Start 启动后台分发器。多次调用 Start 是安全的。
func (m *Manager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	m.once.Do(func() {
		if ctx == nil {
			ctx = context.Background()
		}
		var workerCtx context.Context
		workerCtx, m.cancel = context.WithCancel(ctx)
		go m.run(workerCtx)
	})
}

// Stop 停止分发器并排空队列。
func (m *Manager) Stop() {
	if m == nil {
		return
	}
	m.stopOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		m.mu.Lock()
		m.closed = true
		m.mu.Unlock()
		m.cond.Broadcast()
	})
}

// Register 将一个插件追加到分发列表中。
func (m *Manager) Register(plugin Plugin) {
	if m == nil || plugin == nil {
		return
	}
	m.pluginsMu.Lock()
	m.plugins = append(m.plugins, plugin)
	m.pluginsMu.Unlock()
}

// Publish 将使用量记录入队等待处理。如果未注册任何插件，记录将在下游被丢弃。
func (m *Manager) Publish(ctx context.Context, record Record) {
	if m == nil {
		return
	}
	// ensure worker is running even if Start was not called explicitly
	m.Start(context.Background())
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	m.queue = append(m.queue, queueItem{ctx: ctx, record: record})
	m.mu.Unlock()
	m.cond.Signal()
}

// run 是后台工作协程的主循环，从队列中取出记录并分发给插件。
func (m *Manager) run(ctx context.Context) {
	for {
		m.mu.Lock()
		for !m.closed && len(m.queue) == 0 {
			m.cond.Wait()
		}
		if len(m.queue) == 0 && m.closed {
			m.mu.Unlock()
			return
		}
		item := m.queue[0]
		m.queue = m.queue[1:]
		m.mu.Unlock()
		m.dispatch(item)
	}
}

// dispatch 将队列项分发给所有已注册的插件。
func (m *Manager) dispatch(item queueItem) {
	m.pluginsMu.RLock()
	plugins := make([]Plugin, len(m.plugins))
	copy(plugins, m.plugins)
	m.pluginsMu.RUnlock()
	if len(plugins) == 0 {
		return
	}
	for _, plugin := range plugins {
		if plugin == nil {
			continue
		}
		safeInvoke(plugin, item.ctx, item.record)
	}
}

// safeInvoke 安全地调用插件的 HandleUsage 方法，捕获 panic 以防止单个插件崩溃影响整个分发器。
func safeInvoke(plugin Plugin, ctx context.Context, record Record) {
	defer func() {
		if r := recover(); r != nil {
			log.Errorf("usage: plugin panic recovered: %v", r)
		}
	}()
	plugin.HandleUsage(ctx, record)
}

// defaultManager 是全局默认的使用量管理器实例，缓冲区大小为 512。
var defaultManager = NewManager(512)

// DefaultManager 返回全局默认的使用量管理器实例。
func DefaultManager() *Manager { return defaultManager }

// RegisterPlugin 在默认管理器上注册一个插件。
func RegisterPlugin(plugin Plugin) { DefaultManager().Register(plugin) }

// PublishRecord 使用默认管理器发布一条使用量记录。
func PublishRecord(ctx context.Context, record Record) { DefaultManager().Publish(ctx, record) }

// StartDefault 启动默认管理器的分发器。
func StartDefault(ctx context.Context) { DefaultManager().Start(ctx) }

// StopDefault 停止默认管理器的分发器。
func StopDefault() { DefaultManager().Stop() }
