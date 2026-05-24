// collector - collector.go
// 使用量采集器核心模块。
// 该模块实现了 Manager（采集管理器），负责从上游 CPA 服务拉取使用量事件数据并入库。
// 支持三种采集传输模式：
//   - subscribe: Redis Pub/Sub 订阅模式（CPA v7.0.7+），实时性最高
//   - http: HTTP 轮询模式，通过 REST 接口批量拉取使用量队列
//   - resp: RESP 协议（Redis 协议）直接连接，通过 LPOP/RPOP 操作队列
//
// 当模式设为 auto 时，按 subscribe -> http -> resp 的优先级自动探测并降级。
// 采集器具备指数退避重连机制和认证快照补充功能。
package collector

import (
	"context"
	"errors"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/seakee/cpa-manager/usage-service/internal/config"
	"github.com/seakee/cpa-manager/usage-service/internal/httpqueue"
	"github.com/seakee/cpa-manager/usage-service/internal/resp"
	"github.com/seakee/cpa-manager/usage-service/internal/store"
	"github.com/seakee/cpa-manager/usage-service/internal/usage"
)

// Status 表示采集器当前的运行状态信息。
// 通过 /status API 接口对外暴露，用于监控和调试。
type Status struct {
	Collector      string `json:"collector"`       // 采集器状态：stopped/starting/running/error
	Upstream       string `json:"upstream"`         // 当前连接的 CPA 上游地址
	Mode           string `json:"mode"`             // 配置的采集模式：auto/http/resp/subscribe
	Transport      string `json:"transport"`         // 实际使用的传输协议：http/resp/subscribe
	Queue          string `json:"queue"`             // 使用量队列名称
	LastConsumedAt int64  `json:"lastConsumedAt"`    // 最近一次消费消息的时间戳（毫秒）
	LastInsertedAt int64  `json:"lastInsertedAt"`    // 最近一次成功入库的时间戳（毫秒）
	TotalInserted  int64  `json:"totalInserted"`     // 累计成功入库的事件数
	TotalSkipped   int64  `json:"totalSkipped"`      // 累计跳过的重复事件数
	DeadLetters    int64  `json:"deadLetters"`       // 死信队列中的事件数（解析失败的消息）
	LastError      string `json:"lastError,omitempty"` // 最近一次错误信息
}

// RuntimeConfig 表示采集器的运行时配置。
// 在采集器启动时传入，优先级高于基础配置（config.Config）。
type RuntimeConfig struct {
	CPAUpstreamURL string        // CPA 上游服务的基础 URL
	ManagementKey  string        // CPA 管理接口的认证密钥
	CollectorMode  string        // 采集模式：auto/http/resp/subscribe
	Queue          string        // 使用量队列名称（Redis 队列名）
	PopSide        string        // 队列弹出方向：left（LPOP）或 right（RPOP）
	BatchSize      int           // 每次批量拉取的事件数量
	PollInterval   time.Duration // HTTP/RESP 模式下的轮询间隔
	TLSSkipVerify  bool          // 是否跳过 TLS 证书验证（用于 RESP 连接）
}

// Manager 是使用量采集管理器的核心结构。
// 管理采集器的生命周期（启动、停止、状态查询），
// 并协调多种传输模式的切换和认证快照的补充。
type Manager struct {
	base             config.Config           // 基础配置（作为回退默认值）
	store            *store.Store            // 数据存储层，负责事件入库
	snapshotResolver *authSnapshotResolver   // 认证快照解析器，用于补充账户信息
	mu               sync.Mutex              // 保护 status、cancel 等字段的并发访问
	cancel           context.CancelFunc      // 取消当前采集 goroutine 的函数
	status           Status                  // 当前采集器运行状态
	runtimeCfg       RuntimeConfig           // 当前运行时配置
}

// NewManager 创建一个新的采集管理器实例。
// 初始化基础配置引用、存储层引用和认证快照解析器。
// 初始状态为 stopped，需调用 Start 方法启动采集。
func NewManager(base config.Config, store *store.Store) *Manager {
	return &Manager{
		base:             base,
		store:            store,
		snapshotResolver: newAuthSnapshotResolver(),
		status: Status{
			Collector: "stopped",
			Mode:      collectorMode(base.CollectorMode),
			Queue:     base.Queue,
		},
	}
}

// Start 启动采集器。
// 如果已有采集 goroutine 在运行，会先取消旧的再启动新的。
// 采集 goroutine 在独立的上下文中运行，受 ctx 取消控制。
//
// 参数：
//   - ctx: 父上下文，取消时将级联取消采集 goroutine
//   - cfg: 运行时配置，覆盖基础配置中的连接和采集参数
func (m *Manager) Start(ctx context.Context, cfg RuntimeConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	// 如果已有运行中的采集器，先取消
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.runtimeCfg = cfg
	// 更新状态为启动中
	m.status.Collector = "starting"
	m.status.Upstream = cfg.CPAUpstreamURL
	m.status.Mode = collectorMode(valueOr(cfg.CollectorMode, m.base.CollectorMode))
	m.status.Transport = ""
	m.status.Queue = valueOr(cfg.Queue, m.base.Queue)
	m.status.LastError = ""

	// 创建可取消的子上下文并在独立 goroutine 中运行采集循环
	runCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	go m.run(runCtx, cfg)
}

// Stop 停止采集器。
// 通过取消上下文来通知采集 goroutine 退出，状态更新为 stopped。
func (m *Manager) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.status.Collector = "stopped"
}

// Status 返回采集器当前的运行状态快照。
// 返回的是 Status 结构体的副本，调用方可安全读取无需加锁。
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

// setStatus 通过传入的更新函数安全地修改采集器状态。
// 在锁保护下执行更新操作，确保状态变更的原子性。
func (m *Manager) setStatus(update func(*Status)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	update(&m.status)
}

// run 是采集器的主运行循环入口。
// 根据配置的采集模式选择对应的传输方式：
//   - subscribe 模式：直接使用 Redis Pub/Sub 订阅
//   - auto 模式：按 subscribe -> http -> resp 的优先级依次尝试
//   - http 模式：直接使用 HTTP 轮询
//   - 其他情况：回退到 RESP 协议
func (m *Manager) run(ctx context.Context, cfg RuntimeConfig) {
	mode := collectorMode(valueOr(cfg.CollectorMode, m.base.CollectorMode))

	// subscribe 模式：仅使用 Redis Pub/Sub
	if mode == "subscribe" {
		m.runSubscribe(ctx, cfg, mode)
		return
	}
	// auto 模式：先尝试 subscribe，成功则不降级
	if mode == "auto" && m.runSubscribe(ctx, cfg, mode) {
		return
	}
	// http 模式或 auto 模式的降级路径
	if mode == "http" {
		m.runHTTP(ctx, cfg, mode)
		return
	}
	// auto 模式：subscribe 不支持时尝试 http
	if mode == "auto" && m.runHTTP(ctx, cfg, mode) {
		return
	}
	// 回退到 RESP 协议模式
	m.runRESP(ctx, cfg)
}

// runSubscribe 走 Redis Pub/Sub 订阅模式（CPA v7.0.7+）。返回值含义：
//   - true：ctx 已取消或永久退出，调用方无需降级
//   - false：协议不支持，且 mode=auto 时应降级到 HTTP/RESP
//
// mode=auto 时仅"首次探测"失败才降级；一旦订阅成功后再次断连则坚持订阅模式重连。
func (m *Manager) runSubscribe(ctx context.Context, cfg RuntimeConfig, mode string) bool {
	channel := valueOr(cfg.Queue, m.base.Queue)
	backoff := time.Second
	subscribed := false

	fallback := func() bool {
		m.setStatus(func(status *Status) {
			status.Collector = "starting"
			status.Transport = "http"
			status.LastError = ""
		})
		return false
	}

	for {
		if ctx.Err() != nil {
			return true
		}
		client, err := resp.Dial(cfg.CPAUpstreamURL, cfg.TLSSkipVerify)
		if err != nil {
			if mode == "auto" && !subscribed {
				return fallback()
			}
			m.markError("connect", err)
			sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		if err := client.Auth(cfg.ManagementKey); err != nil {
			_ = client.Close()
			if mode == "auto" && !subscribed {
				return fallback()
			}
			m.markError("auth", err)
			sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		if err := client.Subscribe(channel); err != nil {
			_ = client.Close()
			if mode == "auto" && !subscribed {
				return fallback()
			}
			m.markError("subscribe", err)
			sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		subscribed = true
		backoff = time.Second
		m.setStatus(func(status *Status) {
			status.Collector = "running"
			status.Transport = "subscribe"
			status.LastError = ""
		})

		err = m.consumeSubscribe(ctx, cfg, client)
		_ = client.Close()
		if ctx.Err() != nil {
			return true
		}
		if err != nil {
			m.markError("subscribe", err)
			sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
		}
	}
}

// consumeSubscribe 在已建立的 Redis 订阅连接上持续读取消息。
// 实现了心跳保活机制：当 readWindow 内未收到任何消息时发送 PING，
// 如果 ctx 被取消则通过设置读超时使阻塞中的 ReadMessage 立即返回。
//
// 参数：
//   - ctx: 上下文，用于控制退出
//   - cfg: 运行时配置
//   - client: 已认证并订阅的 RESP 客户端
func (m *Manager) consumeSubscribe(ctx context.Context, cfg RuntimeConfig, client *resp.Client) error {
	const pingInterval = 30 * time.Second // PING 心跳发送间隔
	const readWindow = pingInterval + 10*time.Second // 读超时窗口（心跳间隔 + 10秒缓冲）

	// 通过后台 goroutine 监听 ctx，触发读超时让阻塞中的 ReadMessage 立即返回。
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			// ctx 取消时设置读超时为当前时间，使 ReadMessage 立即返回
			_ = client.SetReadDeadline(time.Now())
		case <-done:
			// consumeSubscribe 正常退出时清理 goroutine
		}
	}()

	lastPing := time.Now()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 设置读超时窗口，超时后将触发 PING 心跳检查
		if err := client.SetReadDeadline(time.Now().Add(readWindow)); err != nil {
			return err
		}
		// 阻塞读取一条 PUBLISH 推送消息（自动跳过控制帧）
		_, payload, err := client.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			// 读超时时检查是否需要发送 PING 心跳
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if time.Since(lastPing) >= pingInterval {
					if perr := client.SendSubscribePing(); perr != nil {
						return perr
					}
					lastPing = time.Now()
				}
				continue
			}
			return err
		}
		// 跳过空白消息
		if strings.TrimSpace(payload) == "" {
			continue
		}
		// 处理收到的使用量事件
		if err := m.processItems(ctx, cfg, []string{payload}); err != nil {
			return err
		}
	}
}

// runHTTP 运行 HTTP 轮询采集循环。
// 通过 HTTP 队列客户端（httpqueue.Client）周期性地从上游拉取使用量事件。
// 当返回 ErrUnsupported 且模式为 auto 时，返回 false 以触发降级到 RESP 模式。
//
// 返回值：
//   - true: ctx 已取消或永久退出，调用方无需降级
//   - false: 协议不支持，且 mode=auto 时应降级到 RESP
func (m *Manager) runHTTP(ctx context.Context, cfg RuntimeConfig, mode string) bool {
	client := httpqueue.New(cfg.CPAUpstreamURL, cfg.ManagementKey)
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return true
		}
		err := m.consumeHTTP(ctx, cfg, client)
		if ctx.Err() != nil {
			return true
		}
		// 上游不支持 HTTP 队列接口且为 auto 模式时，降级到 RESP
		if errors.Is(err, httpqueue.ErrUnsupported) && mode == "auto" {
			m.setStatus(func(status *Status) {
				status.Collector = "starting"
				status.Transport = "resp"
				status.LastError = ""
			})
			return false
		}
		if err != nil {
			m.markError("http", err)
			sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
		}
	}
}

// runRESP 运行 RESP 协议采集循环。
// 通过直接连接上游的 Redis 兼容接口，使用 LPOP/RPOP 操作消费使用量队列。
// 连接断开时自动重连，采用指数退避策略避免频繁重试。
func (m *Manager) runRESP(ctx context.Context, cfg RuntimeConfig) {
	queue := valueOr(cfg.Queue, m.base.Queue)
	popSide := valueOr(cfg.PopSide, m.base.PopSide)
	backoff := time.Second

	for {
		if ctx.Err() != nil {
			return
		}
		// 建立 RESP 连接
		client, err := resp.Dial(cfg.CPAUpstreamURL, cfg.TLSSkipVerify)
		if err != nil {
			m.markError("connect", err)
			sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		// 认证
		if err := client.Auth(cfg.ManagementKey); err != nil {
			_ = client.Close()
			m.markError("auth", err)
			sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
			continue
		}
		// 连接成功，重置退避并更新状态
		backoff = time.Second
		m.setStatus(func(status *Status) {
			status.Collector = "running"
			status.Transport = "resp"
			status.LastError = ""
		})

		// 进入消息消费循环
		err = m.consumeRESP(ctx, cfg, client, queue, popSide)
		_ = client.Close()
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			m.markError("consume", err)
			sleep(ctx, backoff)
			backoff = nextBackoff(backoff)
		}
	}
}

// consumeHTTP 通过 HTTP 接口周期性地从上游拉取使用量事件并处理。
// 当队列为空时按 pollInterval 等待后重试。
// 返回的错误会传播给调用方，由 runHTTP 决定是否重连或降级。
func (m *Manager) consumeHTTP(ctx context.Context, cfg RuntimeConfig, client *httpqueue.Client) error {
	ticker := time.NewTicker(m.pollInterval(cfg))
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		m.setStatus(func(status *Status) {
			status.Collector = "running"
			status.Transport = "http"
			status.LastError = ""
		})
		// 批量弹出使用量事件
		items, err := client.Pop(ctx, m.batchSize(cfg))
		if err != nil {
			return err
		}
		if len(items) == 0 {
			// 队列为空，等待下一个轮询周期
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				continue
			}
		}
		// 处理拉取到的事件
		if err := m.processItems(ctx, cfg, items); err != nil {
			return err
		}
	}
}

// consumeRESP 通过 RESP 协议周期性地从 Redis 队列弹出使用量事件并处理。
// 使用 LPOP 或 RPOP 操作，当队列为空时按 pollInterval 等待后重试。
func (m *Manager) consumeRESP(ctx context.Context, cfg RuntimeConfig, client *resp.Client, queue string, popSide string) error {
	ticker := time.NewTicker(m.pollInterval(cfg))
	defer ticker.Stop()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// 从队列中批量弹出事件
		items, err := client.Pop(queue, popSide, m.batchSize(cfg))
		if err != nil {
			return err
		}
		if len(items) == 0 {
			// 队列为空，等待下一个轮询周期
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
				continue
			}
		}
		// 处理拉取到的事件
		if err := m.processItems(ctx, cfg, items); err != nil {
			return err
		}
	}
}

// processItems 处理一批从队列拉取到的原始使用量消息。
// 处理流程：
// 1. 更新最近消费时间戳
// 2. 逐条解析原始 JSON 消息为 usage.Event 结构
// 3. 解析失败的消息写入死信队列（dead_letter_events 表）
// 4. 通过认证快照解析器补充事件的账户信息
// 5. 批量入库（INSERT OR IGNORE，跳过重复事件）
// 6. 更新成功入库数和跳过数
func (m *Manager) processItems(ctx context.Context, cfg RuntimeConfig, items []string) error {
	if len(items) == 0 {
		return nil
	}
	m.setStatus(func(status *Status) {
		status.LastConsumedAt = time.Now().UnixMilli()
	})
	// 解析原始消息为事件对象
	events := make([]usage.Event, 0, len(items))
	for _, item := range items {
		event, err := usage.NormalizeRaw([]byte(item))
		if err != nil {
			// 解析失败，写入死信队列
			_ = m.store.AddDeadLetter(ctx, item, err)
			m.setStatus(func(status *Status) {
				status.DeadLetters++
			})
			continue
		}
		events = append(events, event)
	}
	// 补充认证快照信息（account_snapshot、auth_label_snapshot 等）
	m.enrichAccountSnapshots(ctx, cfg, events)
	// 批量入库，重复事件（event_hash 冲突）将被跳过
	result, err := m.store.InsertEvents(ctx, events)
	if err != nil {
		return err
	}
	if result.Inserted > 0 || result.Skipped > 0 {
		m.setStatus(func(status *Status) {
			status.LastInsertedAt = time.Now().UnixMilli()
			status.TotalInserted += int64(result.Inserted)
			status.TotalSkipped += int64(result.Skipped)
		})
	}
	return nil
}

// enrichAccountSnapshots 为使用量事件补充认证快照信息。
// 仅补充尚无 AccountSnapshot 且具有 AuthIndex 的事件。
// 通过 snapshotResolver 从上游 CPA 批量获取认证文件信息后填充以下字段：
//   - AccountSnapshot: 账号标识
//   - AuthLabelSnapshot: 认证标签
//   - AuthFileSnapshot: 认证文件名
//   - AuthProviderSnapshot: 提供商
//   - AuthProjectIDSnapshot: 项目 ID（仅在事件本身为空时填充）
//   - AuthSnapshotAtMS: 快照采集时间
func (m *Manager) enrichAccountSnapshots(ctx context.Context, cfg RuntimeConfig, events []usage.Event) {
	if len(events) == 0 || m.snapshotResolver == nil {
		return
	}
	// 收集需要补充快照的 auth_index 集合
	authIndices := make(map[string]struct{})
	for i := range events {
		if events[i].AccountSnapshot != "" || events[i].AuthIndex == "" {
			continue
		}
		authIndices[events[i].AuthIndex] = struct{}{}
	}
	if len(authIndices) == 0 {
		return
	}
	// 批量查询认证快照
	snapshots := m.snapshotResolver.lookup(ctx, cfg, authIndices)
	if len(snapshots) == 0 {
		return
	}
	// 将快照信息填充到对应的事件中
	for i := range events {
		if events[i].AuthIndex == "" || events[i].AccountSnapshot != "" {
			continue
		}
		snapshot, ok := snapshots[events[i].AuthIndex]
		if !ok {
			continue
		}
		events[i].AccountSnapshot = snapshot.Account
		events[i].AuthLabelSnapshot = snapshot.Label
		events[i].AuthFileSnapshot = snapshot.FileName
		events[i].AuthProviderSnapshot = snapshot.Provider
		// 仅在事件本身没有项目 ID 时才补充
		if events[i].AuthProjectIDSnapshot == "" {
			events[i].AuthProjectIDSnapshot = snapshot.ProjectID
		}
		events[i].AuthSnapshotAtMS = snapshot.CapturedAtMS
	}
}

// markError 更新采集器状态为错误状态，记录出错阶段和错误信息。
// 用于在连接、认证、订阅、消费等阶段发生错误时记录状态。
func (m *Manager) markError(stage string, err error) {
	m.setStatus(func(status *Status) {
		status.Collector = "error"
		status.LastError = stage + ": " + err.Error()
	})
}

// sleep 在指定的持续时间内阻塞等待，或在 ctx 取消时提前返回。
// 用于在重试之间添加延迟，支持优雅退出。
func sleep(ctx context.Context, duration time.Duration) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}

// nextBackoff 计算指数退避的下一次等待时间。
// 将当前等待时间翻倍，上限为 30 秒，避免等待时间过长。
func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > 30*time.Second {
		return 30 * time.Second
	}
	return next
}

// valueOr 返回 value；若 value 为空字符串则返回 fallback。
// 用于运行时配置和基础配置之间的级联回退。
func valueOr(value string, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// collectorMode 规范化采集模式字符串。
// 将输入转为小写并去除空白，有效值为 http/resp/subscribe/auto。
// 无效值统一返回 "auto"（自动探测模式）。
func collectorMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "http", "resp", "subscribe":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "auto"
	}
}

// batchSize 获取每批次的事件数量。
// 优先使用运行时配置，其次使用基础配置，最后回退到默认值 100。
func (m *Manager) batchSize(cfg RuntimeConfig) int {
	if cfg.BatchSize > 0 {
		return cfg.BatchSize
	}
	if m.base.BatchSize <= 0 {
		return 100
	}
	return m.base.BatchSize
}

// pollInterval 获取轮询间隔时间。
// 优先使用运行时配置，其次使用基础配置，最后回退到默认值 500ms。
func (m *Manager) pollInterval(cfg RuntimeConfig) time.Duration {
	if cfg.PollInterval > 0 {
		return cfg.PollInterval
	}
	if m.base.PollInterval <= 0 {
		return 500 * time.Millisecond
	}
	return m.base.PollInterval
}
