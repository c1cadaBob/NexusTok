// usage - event.go
// 使用量事件的核心数据结构和标准化解析逻辑。
// 主要功能：
//   - Event 结构体定义：统一的使用量事件数据模型
//   - NormalizeRaw：将上游推送的原始 JSON 消息标准化为 Event 对象
//   - BuildPayload：将事件列表聚合为按 API 端点和模型分组的 Payload
//   - 源信息脱敏：邮箱和密钥的自动遮蔽（maskSource）
//   - 事件哈希：基于关键字段生成稳定的事件唯一标识（buildEventHash）
//   - 敏感字段脱敏：递归脱敏 JSON 中的 api_key、authorization 等字段（redactValue）
package usage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Event 表示一个标准化的使用量事件。
// 所有上游推送的使用量数据都通过 NormalizeRaw 转换为此结构后入库。
// 事件通过 EventHash 实现去重（INSERT OR IGNORE）。
type Event struct {
	RequestID             string `json:"request_id,omitempty"`       // 请求唯一标识
	EventHash             string `json:"event_hash"`                 // 事件哈希（用于去重）
	TimestampMS           int64  `json:"timestamp_ms"`               // 事件时间戳（毫秒）
	Timestamp             string `json:"timestamp"`                  // 事件时间（RFC3339 格式）
	Provider              string `json:"provider,omitempty"`         // AI 提供商
	Model                 string `json:"model"`                      // 聚合用模型名（优先使用请求模型）
	RequestedModel        string `json:"requested_model,omitempty"`  // 请求的模型名（别名）
	ResolvedModel         string `json:"resolved_model,omitempty"`   // 实际解析的模型名
	Endpoint              string `json:"endpoint,omitempty"`         // API 端点（如 "POST /v1/chat/completions"）
	Method                string `json:"method,omitempty"`           // HTTP 方法
	Path                  string `json:"path,omitempty"`             // 请求路径
	AuthType              string `json:"auth_type,omitempty"`        // 认证类型
	AuthIndex             string `json:"auth_index,omitempty"`       // 认证文件索引
	Source                string `json:"source,omitempty"`           // 请求来源（脱敏后）
	SourceHash            string `json:"source_hash,omitempty"`      // 来源的 SHA-256 哈希
	APIKeyHash            string `json:"api_key_hash,omitempty"`     // API Key 的 SHA-256 哈希
	AccountSnapshot       string `json:"account_snapshot,omitempty"`       // 账号快照
	AuthLabelSnapshot     string `json:"auth_label_snapshot,omitempty"`    // 认证标签快照
	AuthFileSnapshot      string `json:"auth_file_snapshot,omitempty"`     // 认证文件名快照
	AuthProviderSnapshot  string `json:"auth_provider_snapshot,omitempty"` // 提供商快照
	AuthProjectIDSnapshot string `json:"auth_project_id_snapshot,omitempty"` // 项目 ID 快照
	AuthSnapshotAtMS      int64  `json:"auth_snapshot_at_ms,omitempty"`   // 快照采集时间
	InputTokens           int64  `json:"input_tokens"`               // 输入 token 数
	OutputTokens          int64  `json:"output_tokens"`              // 输出 token 数
	ReasoningTokens       int64  `json:"reasoning_tokens"`           // 推理 token 数
	CachedTokens          int64  `json:"cached_tokens"`              // 缓存命中 token 数
	CacheTokens           int64  `json:"cache_tokens"`               // 缓存写入 token 数
	TotalTokens           int64  `json:"total_tokens"`               // 总 token 数
	LatencyMS             *int64 `json:"latency_ms,omitempty"`       // 响应延迟（毫秒）
	Failed                bool   `json:"failed"`                     // 是否失败
	RawJSON               string `json:"raw_json,omitempty"`         // 原始 JSON（脱敏后）
	CreatedAtMS           int64  `json:"created_at_ms"`              // 入库时间戳
}

// Tokens 表示 token 使用量的分项统计。
type Tokens struct {
	InputTokens     int64 `json:"input_tokens"`     // 输入 token 数
	OutputTokens    int64 `json:"output_tokens"`    // 输出 token 数
	ReasoningTokens int64 `json:"reasoning_tokens"` // 推理 token 数
	CachedTokens    int64 `json:"cached_tokens"`    // 缓存命中 token 数
	CacheTokens     int64 `json:"cache_tokens"`     // 缓存写入 token 数
	TotalTokens     int64 `json:"total_tokens"`     // 总 token 数
}

// Detail 表示 Payload 中单条请求的详细信息。
type Detail struct {
	Timestamp             string `json:"timestamp"`                       // 请求时间
	Source                string `json:"source"`                          // 请求来源（脱敏后）
	AuthIndex             string `json:"auth_index,omitempty"`            // 认证索引
	APIKeyHash            string `json:"api_key_hash,omitempty"`          // API Key 哈希
	AccountSnapshot       string `json:"account_snapshot,omitempty"`      // 账号快照
	AuthLabelSnapshot     string `json:"auth_label_snapshot,omitempty"`   // 认证标签快照
	AuthFileSnapshot      string `json:"auth_file_snapshot,omitempty"`    // 认证文件名快照
	AuthProviderSnapshot  string `json:"auth_provider_snapshot,omitempty"` // 提供商快照
	AuthProjectIDSnapshot string `json:"auth_project_id_snapshot,omitempty"` // 项目 ID 快照
	AuthSnapshotAtMS      int64  `json:"auth_snapshot_at_ms,omitempty"`   // 快照采集时间
	LatencyMS             *int64 `json:"latency_ms,omitempty"`            // 响应延迟
	ResolvedModel         string `json:"resolved_model,omitempty"`        // 解析的模型名
	Tokens                Tokens `json:"tokens"`                          // token 使用量
	Failed                bool   `json:"failed"`                          // 是否失败
}

// ModelAggregate 表示某个模型下的请求聚合。
type ModelAggregate struct {
	Details []Detail `json:"details"` // 请求详情列表
}

// APIAggregate 表示某个 API 端点下的模型聚合。
type APIAggregate struct {
	Models map[string]*ModelAggregate `json:"models"` // 模型名到聚合的映射
}

// Payload 表示构建后的使用量负载数据，按 API 端点和模型分组聚合。
type Payload struct {
	TotalRequests int64                    `json:"total_requests"` // 总请求数
	SuccessCount  int64                    `json:"success_count"`  // 成功请求数
	FailureCount  int64                    `json:"failure_count"`  // 失败请求数
	TotalTokens   int64                    `json:"total_tokens"`   // 总 token 数
	APIs          map[string]*APIAggregate `json:"apis"`           // 按端点分组的聚合数据
}

// endpointPattern 匹配 "METHOD /path" 格式的端点字符串。
// 用于从 endpoint 字段中提取 HTTP 方法和路径。
var endpointPattern = regexp.MustCompile(`^(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD)\s+(\S+)`)

// NormalizeRaw 将上游推送的原始 JSON 消息标准化为 Event 结构。
// 解析逻辑：
// 1. 反序列化为 map，提取时间戳（支持多种格式：Unix 毫秒/秒、RFC3339 等）
// 2. 提取 HTTP 方法和路径，组合为 endpoint
// 3. 提取 token 使用量（支持嵌套和扁平两种 JSON 结构）
// 4. 提取请求来源并进行脱敏处理
// 5. 区分请求模型（alias）和解析模型（model）
// 6. 递归脱敏原始 JSON 中的敏感字段
// 7. 生成稳定的事件哈希用于去重
func NormalizeRaw(raw []byte) (Event, error) {
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return Event{}, err
	}
	record, ok := payload.(map[string]any)
	if !ok {
		return Event{}, fmt.Errorf("usage payload is not a JSON object")
	}

	redacted := redactValue(payload)
	redactedJSON, _ := json.Marshal(redacted)

	timestampMS, timestamp := readTimestamp(record)
	method := strings.ToUpper(readString(record, "method", "http_method", "httpMethod"))
	path := readString(record, "path", "url_path", "urlPath", "route")
	endpoint := readString(record, "endpoint", "api", "request", "operation")
	if endpoint == "" && method != "" && path != "" {
		endpoint = method + " " + path
	}
	if endpoint != "" {
		if match := endpointPattern.FindStringSubmatch(endpoint); len(match) == 3 {
			if method == "" {
				method = strings.ToUpper(match[1])
			}
			if path == "" {
				path = match[2]
			}
		}
	}
	if endpoint == "" {
		endpoint = "-"
	}

	inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheTokens, totalTokens := readTokenFields(record)
	if totalTokens <= 0 {
		totalTokens = inputTokens + outputTokens + reasoningTokens + maxInt64(cachedTokens, cacheTokens)
	}

	latencyMS := readOptionalInt(record, "latency_ms", "latencyMs", "duration_ms", "durationMs", "elapsed_ms", "elapsedMs")
	failed := readFailed(record)
	sourceRaw := readString(record, "source", "api_key", "apiKey", "key", "account", "email")
	source := maskSource(sourceRaw)
	apiKey := readString(record, "api_key", "apiKey", "key")
	authIndex := readString(record, "auth_index", "authIndex", "AuthIndex")

	requestedModel := readString(record, "alias", "requested_model", "requestedModel")
	resolvedModel := readString(record, "model", "model_name", "modelName", "resolved_model", "resolvedModel")
	model := requestedModel
	if model == "" {
		model = resolvedModel
	}

	event := Event{
		RequestID:             readString(record, "request_id", "requestId", "id"),
		TimestampMS:           timestampMS,
		Timestamp:             timestamp,
		Provider:              readString(record, "provider", "type", "auth_type", "authType"),
		Model:                 model,
		RequestedModel:        requestedModel,
		ResolvedModel:         resolvedModel,
		Endpoint:              endpoint,
		Method:                method,
		Path:                  path,
		AuthType:              readString(record, "auth_type", "authType"),
		AuthIndex:             authIndex,
		Source:                source,
		SourceHash:            hashString(sourceRaw),
		APIKeyHash:            hashString(apiKey),
		AccountSnapshot:       readString(record, "account_snapshot", "accountSnapshot"),
		AuthLabelSnapshot:     readString(record, "auth_label_snapshot", "authLabelSnapshot"),
		AuthFileSnapshot:      readString(record, "auth_file_snapshot", "authFileSnapshot"),
		AuthProviderSnapshot:  readString(record, "auth_provider_snapshot", "authProviderSnapshot"),
		AuthProjectIDSnapshot: readString(record, "auth_project_id_snapshot", "authProjectIdSnapshot", "project_id", "projectId"),
		AuthSnapshotAtMS:      readInt(record, "auth_snapshot_at_ms", "authSnapshotAtMs"),
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		ReasoningTokens:       reasoningTokens,
		CachedTokens:          cachedTokens,
		CacheTokens:           cacheTokens,
		TotalTokens:           totalTokens,
		LatencyMS:             latencyMS,
		Failed:                failed,
		RawJSON:               string(redactedJSON),
		CreatedAtMS:           time.Now().UnixMilli(),
	}
	if event.Model == "" {
		event.Model = "-"
	}
	event.EventHash = buildEventHash(event)
	return event, nil
}

// BuildPayload 将事件列表聚合为按 API 端点和模型分组的 Payload。
// 用于 /v0/management/usage API 的响应格式。
// 聚合维度：endpoint -> model -> details[]。
func BuildPayload(events []Event) Payload {
	payload := Payload{APIs: map[string]*APIAggregate{}}
	for _, event := range events {
		payload.TotalRequests++
		if event.Failed {
			payload.FailureCount++
		} else {
			payload.SuccessCount++
		}
		payload.TotalTokens += event.TotalTokens

		endpoint := event.Endpoint
		if endpoint == "" {
			endpoint = "-"
		}
		apiEntry := payload.APIs[endpoint]
		if apiEntry == nil {
			apiEntry = &APIAggregate{Models: map[string]*ModelAggregate{}}
			payload.APIs[endpoint] = apiEntry
		}
		model := event.Model
		if model == "" {
			model = "-"
		}
		modelEntry := apiEntry.Models[model]
		if modelEntry == nil {
			modelEntry = &ModelAggregate{}
			apiEntry.Models[model] = modelEntry
		}
		modelEntry.Details = append(modelEntry.Details, Detail{
			Timestamp:             event.Timestamp,
			Source:                event.Source,
			AuthIndex:             event.AuthIndex,
			APIKeyHash:            event.APIKeyHash,
			AccountSnapshot:       event.AccountSnapshot,
			AuthLabelSnapshot:     event.AuthLabelSnapshot,
			AuthFileSnapshot:      event.AuthFileSnapshot,
			AuthProviderSnapshot:  event.AuthProviderSnapshot,
			AuthProjectIDSnapshot: event.AuthProjectIDSnapshot,
			AuthSnapshotAtMS:      event.AuthSnapshotAtMS,
			LatencyMS:             event.LatencyMS,
			ResolvedModel:         event.ResolvedModel,
			Failed:                event.Failed,
			Tokens: Tokens{
				InputTokens:     event.InputTokens,
				OutputTokens:    event.OutputTokens,
				ReasoningTokens: event.ReasoningTokens,
				CachedTokens:    event.CachedTokens,
				CacheTokens:     event.CacheTokens,
				TotalTokens:     event.TotalTokens,
			},
		})
	}
	return payload
}

// readTimestamp 从记录中读取并解析时间戳。
// 支持的格式：
//   - float64: Unix 毫秒或秒（< 10^10 视为秒，自动乘以 1000）
//   - string: 纯数字（Unix 毫秒/秒）或日期字符串（RFC3339、"2006-01-02 15:04:05" 等）
//
// 返回毫秒时间戳和 RFC3339 格式字符串。无法解析时使用当前时间。
func readTimestamp(record map[string]any) (int64, string) {
	raw := first(record, "timestamp", "time", "created_at", "createdAt", "created", "request_time", "requestTime")
	now := time.Now()
	if raw == nil {
		return now.UnixMilli(), now.UTC().Format(time.RFC3339Nano)
	}
	switch value := raw.(type) {
	case float64:
		ms := int64(value)
		if ms < 10_000_000_000 {
			ms *= 1000
		}
		return ms, time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
	case string:
		trimmed := strings.TrimSpace(value)
		if number, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			if number < 10_000_000_000 {
				number *= 1000
			}
			return number, time.UnixMilli(number).UTC().Format(time.RFC3339Nano)
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05", "2006-01-02T15:04:05"} {
			if parsed, err := time.Parse(layout, trimmed); err == nil {
				return parsed.UnixMilli(), parsed.UTC().Format(time.RFC3339Nano)
			}
		}
	}
	return now.UnixMilli(), now.UTC().Format(time.RFC3339Nano)
}

// readTokenFields 从记录中提取 token 使用量的各个分项。
// 支持嵌套结构（tokens/usage 子对象）和扁平结构（直接在根对象中）。
// 先尝试从嵌套对象读取，为零时回退到根对象。
// 返回顺序：input, output, reasoning, cached, cache, total。
func readTokenFields(record map[string]any) (int64, int64, int64, int64, int64, int64) {
	tokens := map[string]any{}
	if nested, ok := first(record, "tokens", "usage").(map[string]any); ok {
		tokens = nested
	}
	input := readIntFrom(tokens, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens")
	if input == 0 {
		input = readInt(record, "input_tokens", "inputTokens", "prompt_tokens", "promptTokens")
	}
	output := readIntFrom(tokens, "output_tokens", "outputTokens", "completion_tokens", "completionTokens")
	if output == 0 {
		output = readInt(record, "output_tokens", "outputTokens", "completion_tokens", "completionTokens")
	}
	reasoning := readIntFrom(tokens, "reasoning_tokens", "reasoningTokens")
	if reasoning == 0 {
		reasoning = readInt(record, "reasoning_tokens", "reasoningTokens")
	}
	cached := readIntFrom(tokens, "cached_tokens", "cachedTokens")
	if cached == 0 {
		cached = readInt(record, "cached_tokens", "cachedTokens")
	}
	cache := readIntFrom(tokens, "cache_tokens", "cacheTokens")
	if cache == 0 {
		cache = readInt(record, "cache_tokens", "cacheTokens")
	}
	total := readIntFrom(tokens, "total_tokens", "totalTokens", "total")
	if total == 0 {
		total = readInt(record, "total_tokens", "totalTokens", "total")
	}
	return input, output, reasoning, cached, cache, total
}

// readFailed 判断请求是否失败。
// 优先读取 failed/is_failed 字段；其次读取 success/ok 字段（取反）；
// 最后检查 status/status_code（>= 400 视为失败）或 error/error_message 字段是否存在。
func readFailed(record map[string]any) bool {
	if value, ok := first(record, "failed", "is_failed", "isFailed").(bool); ok {
		return value
	}
	if value, ok := first(record, "success", "ok").(bool); ok {
		return !value
	}
	status := readInt(record, "status", "status_code", "statusCode", "http_status", "httpStatus")
	if status >= 400 {
		return true
	}
	return first(record, "error", "error_message", "errorMessage") != nil
}

// readOptionalInt 读取可选的整数值。
// 当值存在且为 0 时仍返回非 nil 指针；当字段完全不存在时返回 nil。
func readOptionalInt(record map[string]any, keys ...string) *int64 {
	value := readInt(record, keys...)
	if value == 0 && first(record, keys...) == nil {
		return nil
	}
	return &value
}

// readString 从记录中读取字符串值。
// 支持 string、json.Number、float64 等类型，统一 TrimSpace 处理。
func readString(record map[string]any, keys ...string) string {
	raw := first(record, keys...)
	if raw == nil {
		return ""
	}
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case json.Number:
		return value.String()
	case float64:
		if value == float64(int64(value)) {
			return strconv.FormatInt(int64(value), 10)
		}
		return strconv.FormatFloat(value, 'f', -1, 64)
	default:
		return strings.TrimSpace(fmt.Sprint(value))
	}
}

// readInt 从记录中读取整数值。
// 委托给 readIntFrom，支持 float64、int64、json.Number、string 等类型。
func readInt(record map[string]any, keys ...string) int64 {
	return readIntFrom(record, keys...)
}

// readIntFrom 从记录中读取整数值的底层实现。
// 支持 float64、int64、int、json.Number 和 string 类型的自动转换。
func readIntFrom(record map[string]any, keys ...string) int64 {
	raw := first(record, keys...)
	switch value := raw.(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case int:
		return int64(value)
	case json.Number:
		number, _ := value.Int64()
		return number
	case string:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
		return parsed
	default:
		return 0
	}
}

// first 从记录中按 keys 顺序查找第一个存在的值。
// 用于支持同一字段的多种命名风格。
func first(record map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := record[key]; ok {
			return value
		}
	}
	return nil
}

// maxInt64 返回两个 int64 值中的较大值。
func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

// hashString 计算字符串的 SHA-256 哈希值（十六进制编码）。
// 空字符串返回空字符串。
func hashString(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(trimmed))
	return hex.EncodeToString(sum[:])
}

// buildEventHash 根据事件的关键字段生成稳定的哈希值。
// 用于事件去重（INSERT OR IGNORE）。包含以下字段：
// request_id、timestamp、endpoint、model、auth_index、source_hash、
// input_tokens、output_tokens、reasoning_tokens、cached/cache_tokens、failed、latency_ms。
func buildEventHash(event Event) string {
	parts := []string{
		event.RequestID,
		event.Timestamp,
		event.Endpoint,
		event.Model,
		event.AuthIndex,
		event.SourceHash,
		strconv.FormatInt(event.InputTokens, 10),
		strconv.FormatInt(event.OutputTokens, 10),
		strconv.FormatInt(event.ReasoningTokens, 10),
		strconv.FormatInt(maxInt64(event.CachedTokens, event.CacheTokens), 10),
		strconv.FormatBool(event.Failed),
	}
	if event.LatencyMS != nil {
		parts = append(parts, strconv.FormatInt(*event.LatencyMS, 10))
	}
	return hashString(strings.Join(parts, "|"))
}

// maskSource 对请求来源信息进行脱敏处理。
// - 邮箱地址：保留前 3 位 + "***@" + 域名（如 ali***@example.com）
// - 密钥类值：保留前 4 位 + "..." + 后 4 位（如 sk-a...xyz1）
// - 其他值：原样返回
func maskSource(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if strings.Contains(trimmed, "@") {
		parts := strings.SplitN(trimmed, "@", 2)
		prefix := parts[0]
		if len(prefix) > 3 {
			prefix = prefix[:3]
		}
		return prefix + "***@" + parts[1]
	}
	if looksSecret(trimmed) {
		if len(trimmed) <= 8 {
			return "m:****"
		}
		return "m:" + trimmed[:4] + "..." + trimmed[len(trimmed)-4:]
	}
	return trimmed
}

// looksSecret 判断字符串是否看起来像密钥或敏感信息。
// 排除含空格/路径分隔符的值；匹配 sk-/AIza 前缀或长度 >= 32 的值。
func looksSecret(value string) bool {
	if strings.ContainsAny(value, " /\\") {
		return false
	}
	return strings.HasPrefix(value, "sk-") || strings.HasPrefix(value, "AIza") || len(value) >= 32
}

// redactValue 递归脱敏 JSON 值中的敏感字段。
// 对 map 类型的值，检查 key 是否为敏感字段名（api_key、authorization、token、secret 等），
// 是则替换为 "[redacted]"；数组和嵌套 map 递归处理。
func redactValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(item))
		for key, child := range item {
			if isSecretKey(key) {
				result[key] = "[redacted]"
				continue
			}
			result[key] = redactValue(child)
		}
		return result
	case []any:
		result := make([]any, 0, len(item))
		for _, child := range item {
			result = append(result, redactValue(child))
		}
		return result
	default:
		return value
	}
}

// isSecretKey 判断字段名是否为敏感字段。
// 匹配：api_key、apikey、authorization、access_token、refresh_token、token，
// 以及包含 "secret" 的字段名。比较时不区分大小写，连字符统一转为下划线。
func isSecretKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	return normalized == "api_key" ||
		normalized == "apikey" ||
		normalized == "authorization" ||
		normalized == "access_token" ||
		normalized == "refresh_token" ||
		normalized == "token" ||
		strings.Contains(normalized, "secret")
}
