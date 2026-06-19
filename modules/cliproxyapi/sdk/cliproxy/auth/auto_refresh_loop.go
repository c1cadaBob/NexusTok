// auth - auto_refresh_loop.go
// 该文件实现了认证凭据的自动刷新循环，使用最小堆调度器按到期时间排序刷新任务。
// 支持并发刷新、脏标记跟踪、动态重调度等功能，确保 OAuth 令牌等凭据在过期前被及时刷新。
package auth

import (
	"container/heap"
	"context"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

// authAutoRefreshLoop 管理认证凭据的自动刷新调度，使用最小堆按刷新时间排序。
type authAutoRefreshLoop struct {
	manager     *Manager      // 认证管理器引用
	interval    time.Duration // 默认刷新检查间隔
	concurrency int           // 最大并发刷新数

	mu    sync.Mutex                  // 保护队列和脏标记的互斥锁
	queue refreshMinHeap              // 按刷新时间排序的最小堆
	index map[string]*refreshHeapItem // 认证 ID 到堆项的索引
	dirty map[string]struct{}         // 需要重新调度的脏标记集合

	wakeCh chan struct{} // 唤醒主循环的信号通道
	jobs   chan string   // 刷新任务分发通道
}

// newAuthAutoRefreshLoop 创建新的自动刷新循环实例。
func newAuthAutoRefreshLoop(manager *Manager, interval time.Duration, concurrency int) *authAutoRefreshLoop {
	if interval <= 0 {
		interval = refreshCheckInterval
	}
	if concurrency <= 0 {
		concurrency = refreshMaxConcurrency
	}
	jobBuffer := concurrency * 4
	if jobBuffer < 64 {
		jobBuffer = 64
	}
	return &authAutoRefreshLoop{
		manager:     manager,
		interval:    interval,
		concurrency: concurrency,
		index:       make(map[string]*refreshHeapItem),
		dirty:       make(map[string]struct{}),
		wakeCh:      make(chan struct{}, 1),
		jobs:        make(chan string, jobBuffer),
	}
}

// queueReschedule 将认证 ID 标记为脏并唤醒主循环重新调度。
func (l *authAutoRefreshLoop) queueReschedule(authID string) {
	if l == nil || authID == "" {
		return
	}
	l.mu.Lock()
	l.dirty[authID] = struct{}{}
	l.mu.Unlock()
	select {
	case l.wakeCh <- struct{}{}:
	default:
	}
}

// run 启动刷新循环，创建指定数量的工作协程并开始主调度循环。
func (l *authAutoRefreshLoop) run(ctx context.Context) {
	if l == nil || l.manager == nil {
		return
	}

	workers := l.concurrency
	if workers <= 0 {
		workers = refreshMaxConcurrency
	}
	for i := 0; i < workers; i++ {
		go l.worker(ctx)
	}

	l.loop(ctx)
}

// worker 是刷新工作协程的主循环，从 jobs 通道接收认证 ID 并执行刷新。
func (l *authAutoRefreshLoop) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case authID := <-l.jobs:
			if authID == "" {
				continue
			}
			l.manager.refreshAuth(ctx, authID)
			l.queueReschedule(authID)
		}
	}
}

// rebuild 重建刷新队列，遍历所有认证凭据计算下次刷新时间并重新入堆。
func (l *authAutoRefreshLoop) rebuild(now time.Time) {
	type entry struct {
		id   string
		next time.Time
	}

	entries := make([]entry, 0)

	l.manager.mu.RLock()
	for id, auth := range l.manager.auths {
		next, ok := nextRefreshCheckAt(now, auth, l.interval)
		if !ok {
			continue
		}
		entries = append(entries, entry{id: id, next: next})
	}
	l.manager.mu.RUnlock()

	l.mu.Lock()
	l.queue = l.queue[:0]
	l.index = make(map[string]*refreshHeapItem, len(entries))
	for _, e := range entries {
		item := &refreshHeapItem{id: e.id, next: e.next}
		heap.Push(&l.queue, item)
		l.index[e.id] = item
	}
	l.mu.Unlock()
}

// loop 是主调度循环，监听唤醒信号和定时器事件，处理到期的刷新任务。
func (l *authAutoRefreshLoop) loop(ctx context.Context) {
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	defer timer.Stop()

	var timerCh <-chan time.Time
	l.resetTimer(timer, &timerCh, time.Now())

	for {
		select {
		case <-ctx.Done():
			return
		case <-l.wakeCh:
			now := time.Now()
			l.applyDirty(now)
			l.resetTimer(timer, &timerCh, now)
		case <-timerCh:
			now := time.Now()
			l.handleDue(ctx, now)
			l.applyDirty(now)
			l.resetTimer(timer, &timerCh, now)
		}
	}
}

// resetTimer 根据堆顶元素的刷新时间重置定时器。
func (l *authAutoRefreshLoop) resetTimer(timer *time.Timer, timerCh *<-chan time.Time, now time.Time) {
	next, ok := l.peek()
	if !ok {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		*timerCh = nil
		return
	}

	wait := next.Sub(now)
	if wait < 0 {
		wait = 0
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(wait)
	*timerCh = timer.C
}

// peek 返回堆顶元素的刷新时间，不移除元素。
func (l *authAutoRefreshLoop) peek() (time.Time, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.queue) == 0 {
		return time.Time{}, false
	}
	return l.queue[0].next, true
}

// handleDue 处理所有到期的刷新任务。
func (l *authAutoRefreshLoop) handleDue(ctx context.Context, now time.Time) {
	due := l.popDue(now)
	if len(due) == 0 {
		return
	}
	if log.IsLevelEnabled(log.DebugLevel) {
		log.Debugf("auto-refresh scheduler due auths: %d", len(due))
	}
	for _, authID := range due {
		l.handleDueAuth(ctx, now, authID)
	}
}

// popDue 从堆中弹出所有到期的认证 ID。
func (l *authAutoRefreshLoop) popDue(now time.Time) []string {
	l.mu.Lock()
	defer l.mu.Unlock()

	var due []string
	for len(l.queue) > 0 {
		item := l.queue[0]
		if item == nil || item.next.After(now) {
			break
		}
		popped := heap.Pop(&l.queue).(*refreshHeapItem)
		if popped == nil {
			continue
		}
		delete(l.index, popped.id)
		due = append(due, popped.id)
	}
	return due
}

// handleDueAuth 处理单个到期认证凭据的刷新逻辑，判断是否需要刷新并提交到工作队列。
func (l *authAutoRefreshLoop) handleDueAuth(ctx context.Context, now time.Time, authID string) {
	if authID == "" {
		return
	}

	manager := l.manager

	manager.mu.RLock()
	auth := manager.auths[authID]
	if auth == nil {
		manager.mu.RUnlock()
		return
	}
	next, shouldSchedule := nextRefreshCheckAt(now, auth, l.interval)
	shouldRefresh := manager.shouldRefresh(auth, now)
	exec := manager.executors[auth.Provider]
	manager.mu.RUnlock()

	if !shouldSchedule {
		l.remove(authID)
		return
	}

	if !shouldRefresh {
		l.upsert(authID, next)
		return
	}

	if exec == nil {
		l.upsert(authID, now.Add(l.interval))
		return
	}

	if !manager.markRefreshPending(authID, now) {
		manager.mu.RLock()
		auth = manager.auths[authID]
		next, shouldSchedule = nextRefreshCheckAt(now, auth, l.interval)
		manager.mu.RUnlock()
		if shouldSchedule {
			l.upsert(authID, next)
		} else {
			l.remove(authID)
		}
		return
	}

	select {
	case <-ctx.Done():
		return
	case l.jobs <- authID:
	}
}

// applyDirty 处理所有脏标记的认证凭据，重新计算刷新时间并更新堆。
func (l *authAutoRefreshLoop) applyDirty(now time.Time) {
	dirty := l.drainDirty()
	if len(dirty) == 0 {
		return
	}

	for _, authID := range dirty {
		l.manager.mu.RLock()
		auth := l.manager.auths[authID]
		next, ok := nextRefreshCheckAt(now, auth, l.interval)
		l.manager.mu.RUnlock()

		if !ok {
			l.remove(authID)
			continue
		}
		l.upsert(authID, next)
	}
}

// drainDirty 取出并清空所有脏标记的认证 ID 列表。
func (l *authAutoRefreshLoop) drainDirty() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.dirty) == 0 {
		return nil
	}
	out := make([]string, 0, len(l.dirty))
	for authID := range l.dirty {
		out = append(out, authID)
		delete(l.dirty, authID)
	}
	return out
}

// upsert 插入或更新堆中指定认证 ID 的刷新时间。
func (l *authAutoRefreshLoop) upsert(authID string, next time.Time) {
	if authID == "" || next.IsZero() {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if item, ok := l.index[authID]; ok && item != nil {
		item.next = next
		heap.Fix(&l.queue, item.index)
		return
	}
	item := &refreshHeapItem{id: authID, next: next}
	heap.Push(&l.queue, item)
	l.index[authID] = item
}

// remove 从堆中移除指定认证 ID 的刷新项。
func (l *authAutoRefreshLoop) remove(authID string) {
	if authID == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	item, ok := l.index[authID]
	if !ok || item == nil {
		return
	}
	heap.Remove(&l.queue, item.index)
	delete(l.index, authID)
}

// nextRefreshCheckAt 计算指定认证凭据的下次刷新检查时间，
// 综合考虑认证失败状态、账号类型、首选刷新间隔和过期时间等因素。
func nextRefreshCheckAt(now time.Time, auth *Auth, interval time.Duration) (time.Time, bool) {
	if auth == nil {
		return time.Time{}, false
	}
	if hasUnauthorizedAuthFailure(auth) {
		return time.Time{}, false
	}
	if authAccessTokenOnly(auth) {
		return time.Time{}, false
	}

	accountType, _ := auth.AccountInfo()
	if accountType == "api_key" {
		return time.Time{}, false
	}

	if !auth.NextRefreshAfter.IsZero() && now.Before(auth.NextRefreshAfter) {
		return auth.NextRefreshAfter, true
	}

	if evaluator, ok := auth.Runtime.(RefreshEvaluator); ok && evaluator != nil {
		if interval <= 0 {
			interval = refreshCheckInterval
		}
		return now.Add(interval), true
	}

	lastRefresh := auth.LastRefreshedAt
	if lastRefresh.IsZero() {
		if ts, ok := authLastRefreshTimestamp(auth); ok {
			lastRefresh = ts
		}
	}

	expiry, hasExpiry := auth.ExpirationTime()

	if pref := authPreferredInterval(auth); pref > 0 {
		candidates := make([]time.Time, 0, 2)
		if hasExpiry && !expiry.IsZero() {
			if !expiry.After(now) || expiry.Sub(now) <= pref {
				return now, true
			}
			candidates = append(candidates, expiry.Add(-pref))
		}
		if lastRefresh.IsZero() {
			return now, true
		}
		candidates = append(candidates, lastRefresh.Add(pref))
		next := candidates[0]
		for _, candidate := range candidates[1:] {
			if candidate.Before(next) {
				next = candidate
			}
		}
		if !next.After(now) {
			return now, true
		}
		return next, true
	}

	provider := strings.ToLower(auth.Provider)
	lead := ProviderRefreshLead(provider, auth.Runtime)
	if lead == nil {
		return time.Time{}, false
	}
	if hasExpiry && !expiry.IsZero() {
		dueAt := expiry.Add(-*lead)
		if !dueAt.After(now) {
			return now, true
		}
		return dueAt, true
	}
	if !lastRefresh.IsZero() {
		dueAt := lastRefresh.Add(*lead)
		if !dueAt.After(now) {
			return now, true
		}
		return dueAt, true
	}
	return now, true
}

// authAccessTokenOnly 判断认证是否只能使用短期 access_token，不能自动刷新。
//
// 该标记主要用于 Codex AT-only 凭据：它们可以被调度执行真实请求，但没有
// refresh_token，后台自动刷新没有可执行动作。这里同时读取 attributes 和 metadata，
// 以兼容管理端上传、文件 watcher 合成以及未来持久化恢复的不同入口。
func authAccessTokenOnly(auth *Auth) bool {
	if auth == nil {
		return false
	}
	if stringFlag(auth.Attributes, "access_token_only") || stringFlag(auth.Attributes, "at_only") {
		return true
	}
	if mode := strings.TrimSpace(auth.Attributes["credential_mode"]); strings.EqualFold(mode, "access_token_only") {
		return true
	}
	if metadataFlag(auth.Metadata, "access_token_only", "at_only") {
		return true
	}
	if mode := metadataString(auth.Metadata, "credential_mode"); strings.EqualFold(mode, "access_token_only") {
		return true
	}
	return false
}

func stringFlag(values map[string]string, keys ...string) bool {
	if len(values) == 0 {
		return false
	}
	for _, key := range keys {
		switch strings.ToLower(strings.TrimSpace(values[key])) {
		case "1", "true", "yes", "on":
			return true
		}
	}
	return false
}

func metadataString(values map[string]any, key string) string {
	if len(values) == 0 || key == "" {
		return ""
	}
	if value, ok := values[key]; ok {
		switch typed := value.(type) {
		case string:
			return strings.TrimSpace(typed)
		}
	}
	return ""
}

func metadataFlag(values map[string]any, keys ...string) bool {
	if len(values) == 0 {
		return false
	}
	for _, key := range keys {
		value, ok := values[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case bool:
			if typed {
				return true
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(typed)) {
			case "1", "true", "yes", "on":
				return true
			}
		}
	}
	return false
}

// refreshHeapItem 表示刷新调度堆中的单个条目，包含认证 ID、下次刷新时间和堆索引。
type refreshHeapItem struct {
	id    string    // 认证凭据 ID
	next  time.Time // 下次刷新时间
	index int       // 在堆中的索引
}

// refreshMinHeap 是按刷新时间排序的最小堆实现。
type refreshMinHeap []*refreshHeapItem

// Len 返回堆中元素数量。
func (h refreshMinHeap) Len() int { return len(h) }

// Less 比较两个元素的刷新时间，用于堆排序。
func (h refreshMinHeap) Less(i, j int) bool {
	return h[i].next.Before(h[j].next)
}

func (h refreshMinHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *refreshMinHeap) Push(x any) {
	item, ok := x.(*refreshHeapItem)
	if !ok || item == nil {
		return
	}
	item.index = len(*h)
	*h = append(*h, item)
}

func (h *refreshMinHeap) Pop() any {
	old := *h
	n := len(old)
	if n == 0 {
		return (*refreshHeapItem)(nil)
	}
	item := old[n-1]
	item.index = -1
	*h = old[:n-1]
	return item
}
