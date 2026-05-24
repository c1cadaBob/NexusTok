// openai - openai_responses_websocket_toolcall_repair.go
// 提供 WebSocket 环境下 OpenAI Responses API 工具调用的修复和缓存机制。
// 解决 WebSocket 长连接中工具调用（function_call）与工具输出（function_call_output）
// 可能丢失或乱序的问题，通过缓存和引用计数确保请求的完整性。
package openai

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// WebSocket 工具输出缓存的配置常量
const (
	// websocketToolOutputCacheMaxPerSession 是每个会话缓存的最大工具输出数量
	websocketToolOutputCacheMaxPerSession = 256
	// websocketToolOutputCacheTTL 是缓存条目的生存时间（30 分钟）
	websocketToolOutputCacheTTL           = 30 * time.Minute
)

// 默认的全局缓存实例，用于 WebSocket 工具调用修复
var (
	// defaultWebsocketToolOutputCache 是工具输出的全局缓存
	defaultWebsocketToolOutputCache = newWebsocketToolOutputCache(0, websocketToolOutputCacheMaxPerSession)
	// defaultWebsocketToolCallCache 是工具调用的全局缓存
	defaultWebsocketToolCallCache = newWebsocketToolOutputCache(0, websocketToolOutputCacheMaxPerSession)
	// defaultWebsocketToolSessionRefs 是会话引用计数器
	defaultWebsocketToolSessionRefs = newWebsocketToolSessionRefCounter()
)

// websocketToolOutputCache 管理 WebSocket 工具调用的输出缓存。
// 按会话隔离存储，支持 TTL 过期和容量限制。
type websocketToolOutputCache struct {
	// mu 是并发访问的互斥锁
	mu            sync.Mutex
	// ttl 是缓存条目的生存时间
	ttl           time.Duration
	// maxPerSession 是每个会话的最大缓存条目数
	maxPerSession int
	// sessions 存储按会话键索引的缓存数据
	sessions      map[string]*websocketToolOutputSession
}

// websocketToolOutputSession 存储单个 WebSocket 会话的工具调用缓存数据。
type websocketToolOutputSession struct {
	// lastSeen 是该会话最后一次被访问的时间
	lastSeen time.Time
	// outputs 存储按 call_id 索引的工具输出 JSON
	outputs  map[string]json.RawMessage
	// order 记录 call_id 的插入顺序，用于 LRU 淘汰
	order    []string
}

// newWebsocketToolOutputCache 创建新的工具输出缓存实例。
// ttl 为缓存过期时间（<=0 使用默认值 30 分钟），
// maxPerSession 为每会话最大条目数（<=0 使用默认值 256）。
func newWebsocketToolOutputCache(ttl time.Duration, maxPerSession int) *websocketToolOutputCache {
	if ttl < 0 {
		ttl = websocketToolOutputCacheTTL
	}
	if maxPerSession <= 0 {
		maxPerSession = websocketToolOutputCacheMaxPerSession
	}
	return &websocketToolOutputCache{
		ttl:           ttl,
		maxPerSession: maxPerSession,
		sessions:      make(map[string]*websocketToolOutputSession),
	}
}

// record 将工具调用或输出记录到缓存中。
// sessionKey 为会话标识，call_id 为工具调用 ID，item 为 JSON 数据。
// 如果缓存已满，会淘汰最早的条目（LRU 策略）。
func (c *websocketToolOutputCache) record(sessionKey string, callID string, item json.RawMessage) {
	sessionKey = strings.TrimSpace(sessionKey)
	callID = strings.TrimSpace(callID)
	if sessionKey == "" || callID == "" || c == nil {
		return
	}

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cleanupLocked(now)

	session, ok := c.sessions[sessionKey]
	if !ok || session == nil {
		session = &websocketToolOutputSession{
			lastSeen: now,
			outputs:  make(map[string]json.RawMessage),
		}
		c.sessions[sessionKey] = session
	}
	session.lastSeen = now

	if _, exists := session.outputs[callID]; !exists {
		session.order = append(session.order, callID)
	}
	session.outputs[callID] = append(json.RawMessage(nil), item...)

	for len(session.order) > c.maxPerSession {
		evict := session.order[0]
		session.order = session.order[1:]
		delete(session.outputs, evict)
	}
}

// get 从缓存中获取指定会话和 call_id 的工具调用或输出数据。
// 返回缓存的数据和是否存在，每次访问会更新 lastSeen 时间。
func (c *websocketToolOutputCache) get(sessionKey string, callID string) (json.RawMessage, bool) {
	sessionKey = strings.TrimSpace(sessionKey)
	callID = strings.TrimSpace(callID)
	if sessionKey == "" || callID == "" || c == nil {
		return nil, false
	}

	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cleanupLocked(now)

	session, ok := c.sessions[sessionKey]
	if !ok || session == nil {
		return nil, false
	}
	session.lastSeen = now
	item, ok := session.outputs[callID]
	if !ok || len(item) == 0 {
		return nil, false
	}
	return append(json.RawMessage(nil), item...), true
}

// cleanupLocked 清理已过期的会话缓存。
// 必须在持有锁的情况下调用。
func (c *websocketToolOutputCache) cleanupLocked(now time.Time) {
	if c == nil || c.ttl <= 0 {
		return
	}

	for key, session := range c.sessions {
		if session == nil {
			delete(c.sessions, key)
			continue
		}
		if now.Sub(session.lastSeen) > c.ttl {
			delete(c.sessions, key)
		}
	}
}

// deleteSession 删除指定会话的所有缓存数据。
func (c *websocketToolOutputCache) deleteSession(sessionKey string) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.sessions, sessionKey)
}

// websocketDownstreamSessionKey 从 HTTP 请求中提取 WebSocket 下游会话标识。
// 优先级：X-Client-Request-Id > X-Codex-Turn-Metadata.session_id > Session_id。
func websocketDownstreamSessionKey(req *http.Request) string {
	if req == nil {
		return ""
	}
	if requestID := strings.TrimSpace(req.Header.Get("X-Client-Request-Id")); requestID != "" {
		return requestID
	}
	if raw := strings.TrimSpace(req.Header.Get("X-Codex-Turn-Metadata")); raw != "" {
		if sessionID := strings.TrimSpace(gjson.Get(raw, "session_id").String()); sessionID != "" {
			return sessionID
		}
	}
	if sessionID := strings.TrimSpace(req.Header.Get("Session_id")); sessionID != "" {
		return sessionID
	}
	return ""
}

// websocketToolSessionRefCounter 跟踪 WebSocket 会话的引用计数。
// 用于确定何时可以安全清理会话的工具调用缓存。
type websocketToolSessionRefCounter struct {
	// mu 是并发访问的互斥锁
	mu     sync.Mutex
	// counts 存储每个会话键的引用计数
	counts map[string]int
}

// newWebsocketToolSessionRefCounter 创建新的会话引用计数器实例。
func newWebsocketToolSessionRefCounter() *websocketToolSessionRefCounter {
	return &websocketToolSessionRefCounter{counts: make(map[string]int)}
}

// acquire 增加指定会话的引用计数。
func (c *websocketToolSessionRefCounter) acquire(sessionKey string) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || c == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.counts[sessionKey]++
}

// release 减少指定会话的引用计数。
// 如果引用计数降为 0，则删除该会话的计数记录并返回 true，
// 表示可以安全清理该会话的缓存数据。
func (c *websocketToolSessionRefCounter) release(sessionKey string) bool {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || c == nil {
		return false
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	count := c.counts[sessionKey]
	if count <= 1 {
		delete(c.counts, sessionKey)
		return true
	}
	c.counts[sessionKey] = count - 1
	return false
}

// retainResponsesWebsocketToolCaches 增加指定会话的 WebSocket 工具缓存引用计数。
// 在开始新的 WebSocket 连接时调用，防止缓存被过早清理。
func retainResponsesWebsocketToolCaches(sessionKey string) {
	if defaultWebsocketToolSessionRefs == nil {
		return
	}
	defaultWebsocketToolSessionRefs.acquire(sessionKey)
}

// releaseResponsesWebsocketToolCaches 减少指定会话的 WebSocket 工具缓存引用计数。
// 当引用计数降为 0 时，清理该会话的工具输出缓存和工具调用缓存。
// 在 WebSocket 连接关闭时调用。
func releaseResponsesWebsocketToolCaches(sessionKey string) {
	if defaultWebsocketToolSessionRefs == nil {
		return
	}
	if !defaultWebsocketToolSessionRefs.release(sessionKey) {
		return
	}

	if defaultWebsocketToolOutputCache != nil {
		defaultWebsocketToolOutputCache.deleteSession(sessionKey)
	}
	if defaultWebsocketToolCallCache != nil {
		defaultWebsocketToolCallCache.deleteSession(sessionKey)
	}
}

// repairResponsesWebsocketToolCalls 修复 WebSocket 环境下 Responses API 的工具调用。
// 使用默认的全局缓存实例，确保工具调用和输出的配对完整性。
func repairResponsesWebsocketToolCalls(sessionKey string, payload []byte) []byte {
	return repairResponsesWebsocketToolCallsWithCaches(defaultWebsocketToolOutputCache, defaultWebsocketToolCallCache, sessionKey, payload)
}

// repairResponsesWebsocketToolCallsWithCache 使用指定的输出缓存修复工具调用。
// callCache 设为 nil 表示不使用工具调用缓存。
func repairResponsesWebsocketToolCallsWithCache(cache *websocketToolOutputCache, sessionKey string, payload []byte) []byte {
	return repairResponsesWebsocketToolCallsWithCaches(cache, nil, sessionKey, payload)
}

// repairResponsesWebsocketToolCallsWithCaches 修复工具调用的核心实现。
// 处理 Responses API 请求中的 input 数组，确保每个 function_call_output
// 都有对应的 function_call，反之亦然。
// 当 allowOrphanOutputs 为 true 时（如存在 previous_response_id），
// 允许孤立的 function_call_output 存在。
func repairResponsesWebsocketToolCallsWithCaches(outputCache, callCache *websocketToolOutputCache, sessionKey string, payload []byte) []byte {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || outputCache == nil || len(payload) == 0 {
		return payload
	}

	input := gjson.GetBytes(payload, "input")
	if !input.Exists() || !input.IsArray() {
		return payload
	}

	allowOrphanOutputs := strings.TrimSpace(gjson.GetBytes(payload, "previous_response_id").String()) != ""
	updatedRaw, errRepair := repairResponsesToolCallsArray(outputCache, callCache, sessionKey, input.Raw, allowOrphanOutputs)
	if errRepair != nil || updatedRaw == "" || updatedRaw == input.Raw {
		return payload
	}

	updated, errSet := sjson.SetRawBytes(payload, "input", []byte(updatedRaw))
	if errSet != nil {
		return payload
	}
	return updated
}

// repairResponsesToolCallsArray 修复工具调用数组的核心逻辑。
// 两遍扫描策略：
// 1. 第一遍：记录所有工具输出和工具调用，建立 call_id 映射
// 2. 第二遍：过滤孤立的工具输出和调用，从缓存中恢复缺失的配对项
func repairResponsesToolCallsArray(outputCache, callCache *websocketToolOutputCache, sessionKey string, rawArray string, allowOrphanOutputs bool) (string, error) {
	rawArray = strings.TrimSpace(rawArray)
	if rawArray == "" {
		return "[]", nil
	}

	var items []json.RawMessage
	if errUnmarshal := json.Unmarshal([]byte(rawArray), &items); errUnmarshal != nil {
		return "", errUnmarshal
	}

	// First pass: record tool outputs and remember which call_ids have outputs in this payload.
	outputPresent := make(map[string]struct{}, len(items))
	callPresent := make(map[string]struct{}, len(items))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		itemType := strings.TrimSpace(gjson.GetBytes(item, "type").String())
		switch {
		case isResponsesToolCallOutputType(itemType):
			callID := strings.TrimSpace(gjson.GetBytes(item, "call_id").String())
			if callID == "" {
				continue
			}
			outputPresent[callID] = struct{}{}
			outputCache.record(sessionKey, callID, item)
		case isResponsesToolCallType(itemType):
			callID := strings.TrimSpace(gjson.GetBytes(item, "call_id").String())
			if callID == "" {
				continue
			}
			callPresent[callID] = struct{}{}
			if callCache != nil {
				callCache.record(sessionKey, callID, item)
			}
		}
	}

	filtered := make([]json.RawMessage, 0, len(items))
	insertedCalls := make(map[string]struct{}, len(items))
	for _, item := range items {
		if len(item) == 0 {
			continue
		}
		itemType := strings.TrimSpace(gjson.GetBytes(item, "type").String())
		if isResponsesToolCallOutputType(itemType) {
			callID := strings.TrimSpace(gjson.GetBytes(item, "call_id").String())
			if callID == "" {
				// Upstream rejects tool outputs without a call_id; drop it.
				continue
			}

			if _, ok := callPresent[callID]; ok {
				filtered = append(filtered, item)
				continue
			}

			if callCache != nil {
				if cached, ok := callCache.get(sessionKey, callID); ok {
					if _, already := insertedCalls[callID]; !already {
						filtered = append(filtered, cached)
						insertedCalls[callID] = struct{}{}
						callPresent[callID] = struct{}{}
					}
					filtered = append(filtered, item)
					continue
				}
			}

			if allowOrphanOutputs {
				filtered = append(filtered, item)
				continue
			}

			// Drop orphaned function_call_output items; upstream rejects transcripts with missing calls.
			continue
		}
		if !isResponsesToolCallType(itemType) {
			filtered = append(filtered, item)
			continue
		}

		callID := strings.TrimSpace(gjson.GetBytes(item, "call_id").String())
		if callID == "" {
			// Upstream rejects tool calls without a call_id; drop it.
			continue
		}

		if _, ok := outputPresent[callID]; ok {
			filtered = append(filtered, item)
			continue
		}

		if cached, ok := outputCache.get(sessionKey, callID); ok {
			filtered = append(filtered, item)
			filtered = append(filtered, cached)
			outputPresent[callID] = struct{}{}
			continue
		}

		// Drop orphaned function_call items; upstream rejects transcripts with missing outputs.
	}

	out, errMarshal := json.Marshal(filtered)
	if errMarshal != nil {
		return "", errMarshal
	}
	return string(out), nil
}

// recordResponsesWebsocketToolCallsFromPayload 从 Responses API 的事件负载中记录工具调用。
// 使用默认的全局工具调用缓存。
func recordResponsesWebsocketToolCallsFromPayload(sessionKey string, payload []byte) {
	recordResponsesWebsocketToolCallsFromPayloadWithCache(defaultWebsocketToolCallCache, sessionKey, payload)
}

// recordResponsesWebsocketToolCallsFromPayloadWithCache 从 Responses API 的事件负载中记录工具调用到指定缓存。
// 处理的事件类型：
// - response.completed: 从 response.output 数组中提取工具调用
// - response.output_item.added/done: 从 item 中提取工具调用
func recordResponsesWebsocketToolCallsFromPayloadWithCache(cache *websocketToolOutputCache, sessionKey string, payload []byte) {
	sessionKey = strings.TrimSpace(sessionKey)
	if sessionKey == "" || cache == nil || len(payload) == 0 {
		return
	}

	eventType := strings.TrimSpace(gjson.GetBytes(payload, "type").String())
	switch eventType {
	case "response.completed":
		output := gjson.GetBytes(payload, "response.output")
		if !output.Exists() || !output.IsArray() {
			return
		}
		for _, item := range output.Array() {
			if !isResponsesToolCallType(item.Get("type").String()) {
				continue
			}
			callID := strings.TrimSpace(item.Get("call_id").String())
			if callID == "" {
				continue
			}
			cache.record(sessionKey, callID, json.RawMessage(item.Raw))
		}
	case "response.output_item.added", "response.output_item.done":
		item := gjson.GetBytes(payload, "item")
		if !item.Exists() || !item.IsObject() {
			return
		}
		if !isResponsesToolCallType(item.Get("type").String()) {
			return
		}
		callID := strings.TrimSpace(item.Get("call_id").String())
		if callID == "" {
			return
		}
		cache.record(sessionKey, callID, json.RawMessage(item.Raw))
	}
}

// isResponsesToolCallType 判断给定类型是否为 Responses API 的工具调用类型。
// 支持 "function_call" 和 "custom_tool_call" 两种类型。
func isResponsesToolCallType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "function_call", "custom_tool_call":
		return true
	default:
		return false
	}
}

// isResponsesToolCallOutputType 判断给定类型是否为 Responses API 的工具输出类型。
// 支持 "function_call_output" 和 "custom_tool_call_output" 两种类型。
func isResponsesToolCallOutputType(itemType string) bool {
	switch strings.TrimSpace(itemType) {
	case "function_call_output", "custom_tool_call_output":
		return true
	default:
		return false
	}
}
