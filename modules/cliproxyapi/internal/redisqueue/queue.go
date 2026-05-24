// redisqueue - queue.go
// 本文件实现了内存中的使用记录队列，支持发布/订阅模式和基于时间窗口的消息过期。
// 队列采用环形缓冲 + 惰性清理策略，在高并发场景下保持低内存占用。
// 支持两种消费方式：订阅模式（实时推送）和轮询模式（PopOldest 批量拉取）。
package redisqueue

import (
	"sync"
	"sync/atomic"
	"time"
)

const (
	// defaultRetentionSeconds 是队列消息的默认保留时长（秒）。
	defaultRetentionSeconds int64 = 60
	// maxRetentionSeconds 是队列消息保留时长的最大上限（秒），防止配置过大导致内存溢出。
	maxRetentionSeconds int64 = 3600
	// usageSubscriberBuffer 是每个订阅者 channel 的缓冲区大小。
	usageSubscriberBuffer = 256
)

// queueItem 代表队列中的一条消息记录。
// 包含入队时间和序列化后的消息负载。
type queueItem struct {
	// enqueuedAt 记录消息的入队时间，用于过期清理。
	enqueuedAt time.Time
	// payload 是消息的序列化字节负载。
	payload    []byte
}

// queue 是内存使用记录队列的核心结构体。
// 采用切片+游标的方式实现 FIFO 队列，并维护一组订阅者 channel。
type queue struct {
	// mu 保护队列和订阅者集合的并发访问。
	mu               sync.Mutex
	// items 是存储队列消息的切片。
	items            []queueItem
	// head 是队列中第一个未消费消息的索引（逻辑起始位置）。
	head             int
	// subscribers 是当前活跃的订阅者 channel 集合，以唯一 ID 为键。
	subscribers      map[uint64]chan []byte
	// nextSubscriberID 是下一个分配给新订阅者的自增 ID。
	nextSubscriberID uint64
}

var (
	// enabled 控制队列功能的全局开关。
	enabled          atomic.Bool
	// retentionSeconds 存储当前配置的消息保留时长。
	retentionSeconds atomic.Int64
	// global 是全局唯一的队列实例。
	global           queue
)

// init 初始化默认的消息保留时长。
func init() {
	retentionSeconds.Store(defaultRetentionSeconds)
}

// SetEnabled 设置队列功能的全局开关。
// 关闭队列时会清空所有已缓存的消息和订阅者。
//
// 参数：
//   - value: true 启用队列，false 禁用队列
func SetEnabled(value bool) {
	enabled.Store(value)
	if !value {
		global.clear()
	}
}

// Enabled 返回队列功能是否已启用。
//
// 返回值：
//   - bool: true 表示队列已启用，false 表示已禁用
func Enabled() bool {
	return enabled.Load()
}

// SetRetentionSeconds 设置队列消息的保留时长（秒）。
// 值小于等于 0 时重置为默认值 60 秒，大于 3600 时截断为 3600 秒。
//
// 参数：
//   - value: 消息保留时长（秒）
func SetRetentionSeconds(value int) {
	normalized := int64(value)
	if normalized <= 0 {
		normalized = defaultRetentionSeconds
	} else if normalized > maxRetentionSeconds {
		normalized = maxRetentionSeconds
	}
	retentionSeconds.Store(normalized)
}

// Enqueue 将一条序列化后的使用记录推入队列。
// 如果存在活跃的订阅者，消息会直接推送到订阅者 channel 而不缓存；
// 否则消息会被缓存到队列中等待轮询消费。
//
// 参数：
//   - payload: 序列化后的消息字节数组
func Enqueue(payload []byte) {
	if !Enabled() {
		return
	}
	if len(payload) == 0 {
		return
	}
	if global.publishToSubscribers(payload) {
		return
	}
	global.enqueue(payload)
}

// PopOldest 从队列中取出指定数量的最旧消息（FIFO 顺序）。
// 取出的消息会从队列中移除。返回的消息数量可能少于请求数量（队列中消息不足时）。
//
// 参数：
//   - count: 期望取出的消息数量
//
// 返回值：
//   - [][]byte: 取出的消息字节数组切片，队列为空或已禁用时返回 nil
func PopOldest(count int) [][]byte {
	if !Enabled() {
		return nil
	}
	if count <= 0 {
		return nil
	}
	return global.popOldest(count)
}

// SubscribeUsage 创建一个新的使用记录订阅，返回一个接收 channel 和取消订阅函数。
// 订阅者通过返回的 channel 实时接收队列中的新消息。
// 调用返回的取消订阅函数可以安全地关闭订阅并释放资源。
//
// 返回值：
//   - <-chan []byte: 接收使用记录消息的只读 channel
//   - func(): 取消订阅的函数，可安全多次调用
func SubscribeUsage() (<-chan []byte, func()) {
	return global.subscribeUsage()
}

// clear 清空队列中的所有消息和订阅者。
// 该方法会关闭所有活跃的订阅者 channel。
// 调用方需确保在持有锁的情况下调用此方法。
func (q *queue) clear() {
	q.mu.Lock()

	subscribers := make([]chan []byte, 0, len(q.subscribers))
	for _, subscriber := range q.subscribers {
		subscribers = append(subscribers, subscriber)
	}
	q.items = nil
	q.head = 0
	q.subscribers = nil
	q.mu.Unlock()

	for _, subscriber := range subscribers {
		close(subscriber)
	}
}

// enqueue 将消息添加到队列末尾。
// 在入队前会先清理过期消息，入队后根据需要触发内存压缩。
// 消息负载会被深拷贝以防调用方后续修改。
//
// 参数：
//   - payload: 待入队的消息字节数组
func (q *queue) enqueue(payload []byte) {
	now := time.Now()

	q.mu.Lock()
	defer q.mu.Unlock()

	q.pruneLocked(now)
	q.items = append(q.items, queueItem{
		enqueuedAt: now,
		payload:    append([]byte(nil), payload...),
	})
	q.maybeCompactLocked()
}

// publishToSubscribers 尝试将消息推送给所有活跃的订阅者。
// 如果没有订阅者则返回 false，调用方应将消息缓存到队列中。
// 对于缓冲区已满的订阅者，会自动将其从订阅列表中移除并关闭 channel。
//
// 参数：
//   - payload: 待推送的消息字节数组
//
// 返回值：
//   - bool: true 表示至少有一个订阅者接收了消息，false 表示无活跃订阅者
func (q *queue) publishToSubscribers(payload []byte) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.subscribers) == 0 {
		return false
	}

	for id, subscriber := range q.subscribers {
		cloned := append([]byte(nil), payload...)
		select {
		case subscriber <- cloned:
		default:
			delete(q.subscribers, id)
			close(subscriber)
		}
	}

	return true
}

// subscribeUsage 创建并注册一个新的订阅者。
// 返回的取消订阅函数使用 sync.Once 保证多次调用的安全性。
//
// 返回值：
//   - <-chan []byte: 接收消息的只读 channel
//   - func(): 取消订阅函数
func (q *queue) subscribeUsage() (<-chan []byte, func()) {
	subscriber := make(chan []byte, usageSubscriberBuffer)

	q.mu.Lock()
	if q.subscribers == nil {
		q.subscribers = make(map[uint64]chan []byte)
	}
	q.nextSubscriberID++
	id := q.nextSubscriberID
	q.subscribers[id] = subscriber
	q.mu.Unlock()

	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			q.unsubscribeUsage(id)
		})
	}
	return subscriber, unsubscribe
}

// unsubscribeUsage 移除并关闭指定 ID 的订阅者。
//
// 参数：
//   - id: 订阅者的唯一标识符
func (q *queue) unsubscribeUsage(id uint64) {
	q.mu.Lock()
	subscriber, ok := q.subscribers[id]
	if ok {
		delete(q.subscribers, id)
	}
	q.mu.Unlock()

	if ok {
		close(subscriber)
	}
}

// popOldest 从队列中取出指定数量的最旧消息。
// 在取出前先清理过期消息，取出后根据需要触发内存压缩。
//
// 参数：
//   - count: 期望取出的消息数量
//
// 返回值：
//   - [][]byte: 取出的消息字节数组切片
func (q *queue) popOldest(count int) [][]byte {
	now := time.Now()

	q.mu.Lock()
	defer q.mu.Unlock()

	q.pruneLocked(now)
	available := len(q.items) - q.head
	if available <= 0 {
		q.items = nil
		q.head = 0
		return nil
	}
	if count > available {
		count = available
	}

	out := make([][]byte, 0, count)
	for i := 0; i < count; i++ {
		item := q.items[q.head+i]
		out = append(out, item.payload)
	}
	q.head += count
	q.maybeCompactLocked()
	return out
}

// pruneLocked 清理队列中已过期的消息。
// 通过移动 head 游标来跳过过期消息，而不是立即回收内存。
// 内存回收由 maybeCompactLocked 在合适的时机执行。
//
// 参数：
//   - now: 当前时间，用于计算消息是否过期
func (q *queue) pruneLocked(now time.Time) {
	if q.head >= len(q.items) {
		q.items = nil
		q.head = 0
		return
	}

	windowSeconds := retentionSeconds.Load()
	if windowSeconds <= 0 {
		windowSeconds = defaultRetentionSeconds
	}
	cutoff := now.Add(-time.Duration(windowSeconds) * time.Second)
	for q.head < len(q.items) && q.items[q.head].enqueuedAt.Before(cutoff) {
		q.head++
	}
}

// maybeCompactLocked 在满足条件时压缩队列内存。
// 当 head 游标积累了足够多的已消费消息（超过 1024 条或超过总消息数的一半）时，
// 将剩余未消费消息拷贝到新切片中，释放已消费消息占用的内存。
func (q *queue) maybeCompactLocked() {
	if q.head == 0 {
		return
	}
	if q.head >= len(q.items) {
		q.items = nil
		q.head = 0
		return
	}
	if q.head < 1024 && q.head*2 < len(q.items) {
		return
	}
	q.items = append([]queueItem(nil), q.items[q.head:]...)
	q.head = 0
}
