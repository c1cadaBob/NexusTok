// usage - import.go
// 使用量数据的导入解析模块。
// 支持三种导入格式：
//   - usage_service_jsonl: 标准 JSONL 格式（每行一个 JSON 对象），或导出的事件记录
//   - legacy_usage_export: 旧版 CPA 导出格式（嵌套结构：usage.apis.{endpoint}.models.{model}.details[]）
//   - legacy_usage_payload: 旧版直接负载格式（无外层 usage 包装）
//
// 自动识别格式并解析，解析失败的行计入 Failed 统计。
// 旧版格式会生成警告信息提示元数据可能不完整。
package usage

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 导入格式常量定义
const (
	ImportFormatJSONL         = "usage_service_jsonl"    // 标准 JSONL 格式或导出事件
	ImportFormatLegacyExport  = "legacy_usage_export"    // 旧版 CPA 导出格式
	ImportFormatLegacyPayload = "legacy_usage_payload"   // 旧版直接负载格式
)

// 导入解析相关的错误定义
var (
	ErrUnsupportedImportFormat = errors.New("unsupported usage import format") // 不支持的导入格式
	ErrLegacyUsageNoDetails    = errors.New("legacy usage export does not contain request details") // 旧版格式缺少请求详情
)

// ImportParseResult 表示导入解析的结果。
type ImportParseResult struct {
	Format      string   // 识别到的导入格式
	Events      []Event  // 成功解析的事件列表
	Failed      int      // 解析失败的条目数
	Unsupported int      // 不支持的条目数（如旧版格式的汇总记录）
	Warnings    []string // 解析警告信息（如旧版格式的元数据限制提示）
}

// ParseImportPayload 解析使用量导入数据，自动识别格式。
// 格式识别规则：
//   - 以 '{' 开头：尝试按 JSON 对象解析（legacy_export、legacy_payload 或单条 JSONL）
//   - 以 '[' 开头：按 JSON 数组解析
//   - 其他：按 JSONL（每行一个 JSON）解析
//
// 当 JSON 对象解析失败且数据包含换行符时，回退到 JSONL 解析。
func ParseImportPayload(data []byte) (ImportParseResult, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return ImportParseResult{}, errors.New("empty usage import payload")
	}

	switch trimmed[0] {
	case '{':
		result, err := parseJSONObjectImport(trimmed)
		if err != nil && bytes.Contains(trimmed, []byte{'\n'}) && !errors.Is(err, ErrLegacyUsageNoDetails) {
			return parseJSONLImport(trimmed)
		}
		return result, err
	case '[':
		return parseJSONArrayImport(trimmed)
	default:
		return parseJSONLImport(trimmed)
	}
}

// parseJSONObjectImport 解析 JSON 对象格式的导入数据。
// 识别优先级：
// 1. 包含 "usage" 字段且其中有 "apis"：旧版导出格式（legacy_usage_export）
// 2. 直接包含 "apis" 字段：旧版负载格式（legacy_usage_payload）
// 3. 包含 "event_hash" 字段：导出的单条事件记录
// 4. 包含汇总字段（total_requests 等）：不支持的旧版汇总格式
// 5. 其他：作为单条使用量事件解析
func parseJSONObjectImport(data []byte) (ImportParseResult, error) {
	var record map[string]any
	if err := decodeJSON(data, &record); err != nil {
		return ImportParseResult{}, err
	}

	if usageRaw, ok := record["usage"]; ok {
		usageRecord, ok := usageRaw.(map[string]any)
		if !ok {
			return ImportParseResult{}, ErrLegacyUsageNoDetails
		}
		if hasUsageAPIs(usageRecord) {
			result, err := eventsFromLegacyUsage(usageRecord, ImportFormatLegacyExport)
			if err != nil {
				return result, err
			}
			return result, nil
		}
		return ImportParseResult{
			Format:      ImportFormatLegacyExport,
			Unsupported: 1,
		}, ErrLegacyUsageNoDetails
	}

	if hasUsageAPIs(record) {
		return eventsFromLegacyUsage(record, ImportFormatLegacyPayload)
	}

	if event, ok, err := eventFromExportedRecord(record); ok || err != nil {
		if err != nil {
			return ImportParseResult{Format: ImportFormatJSONL, Failed: 1}, err
		}
		return ImportParseResult{Format: ImportFormatJSONL, Events: []Event{event}}, nil
	}

	if looksLikeLegacyUsageSummary(record) {
		return ImportParseResult{
			Format:      ImportFormatLegacyPayload,
			Unsupported: 1,
		}, ErrLegacyUsageNoDetails
	}

	event, err := NormalizeRaw(data)
	if err != nil {
		return ImportParseResult{Format: ImportFormatJSONL, Failed: 1}, err
	}
	return ImportParseResult{Format: ImportFormatJSONL, Events: []Event{event}}, nil
}

// parseJSONArrayImport 解析 JSON 数组格式的导入数据。
// 数组中的每个元素作为独立的事件记录解析。
func parseJSONArrayImport(data []byte) (ImportParseResult, error) {
	var items []json.RawMessage
	if err := decodeJSON(data, &items); err != nil {
		return ImportParseResult{}, err
	}

	result := ImportParseResult{Format: ImportFormatJSONL}
	for _, item := range items {
		event, err := eventFromJSONRecord(item)
		if err != nil {
			result.Failed++
			continue
		}
		result.Events = append(result.Events, event)
	}
	return result, nil
}

// parseJSONLImport 按 JSONL 格式解析导入数据（每行一个 JSON 对象）。
// 空行被跳过，解析失败的行计入 Failed 统计。
func parseJSONLImport(data []byte) (ImportParseResult, error) {
	result := ImportParseResult{Format: ImportFormatJSONL}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		event, err := eventFromJSONRecord([]byte(line))
		if err != nil {
			result.Failed++
			continue
		}
		result.Events = append(result.Events, event)
	}
	if err := scanner.Err(); err != nil {
		return result, err
	}
	return result, nil
}

// eventFromJSONRecord 从单条 JSON 记录解析事件。
// 优先尝试按导出记录（event_hash 字段）解析，失败则按原始消息解析。
func eventFromJSONRecord(data []byte) (Event, error) {
	var record map[string]any
	if err := decodeJSON(data, &record); err != nil {
		return Event{}, err
	}
	if event, ok, err := eventFromExportedRecord(record); ok || err != nil {
		return event, err
	}
	return NormalizeRaw(data)
}

// eventFromExportedRecord 从导出的事件记录（含 event_hash 字段）解析为 Event。
// 支持导出时保留的所有字段（request_id、timestamp_ms、source_hash、api_key_hash 等）。
// 返回值 ok=true 表示识别为导出记录格式。
func eventFromExportedRecord(record map[string]any) (Event, bool, error) {
	eventHash := readString(record, "event_hash", "eventHash")
	if eventHash == "" {
		return Event{}, false, nil
	}

	timestampMS := readInt(record, "timestamp_ms", "timestampMs")
	timestamp := readString(record, "timestamp")
	if timestampMS <= 0 || timestamp == "" {
		parsedMS, parsedTimestamp := readTimestamp(record)
		if timestampMS <= 0 {
			timestampMS = parsedMS
		}
		if timestamp == "" {
			timestamp = parsedTimestamp
		}
	}

	inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheTokens, totalTokens := readTokenFields(record)
	if totalTokens <= 0 {
		totalTokens = inputTokens + outputTokens + reasoningTokens + maxInt64(cachedTokens, cacheTokens)
	}

	event := Event{
		RequestID:             readString(record, "request_id", "requestId"),
		EventHash:             eventHash,
		TimestampMS:           timestampMS,
		Timestamp:             timestamp,
		Provider:              readString(record, "provider"),
		Model:                 readString(record, "model"),
		Endpoint:              readString(record, "endpoint"),
		Method:                readString(record, "method"),
		Path:                  readString(record, "path"),
		AuthType:              readString(record, "auth_type", "authType"),
		AuthIndex:             readString(record, "auth_index", "authIndex", "AuthIndex"),
		Source:                readString(record, "source"),
		SourceHash:            readString(record, "source_hash", "sourceHash"),
		APIKeyHash:            readString(record, "api_key_hash", "apiKeyHash"),
		AccountSnapshot:       readString(record, "account_snapshot", "accountSnapshot"),
		AuthLabelSnapshot:     readString(record, "auth_label_snapshot", "authLabelSnapshot"),
		AuthFileSnapshot:      readString(record, "auth_file_snapshot", "authFileSnapshot"),
		AuthProviderSnapshot:  readString(record, "auth_provider_snapshot", "authProviderSnapshot"),
		AuthProjectIDSnapshot: readString(record, "auth_project_id_snapshot", "authProjectIdSnapshot"),
		AuthSnapshotAtMS:      readInt(record, "auth_snapshot_at_ms", "authSnapshotAtMs"),
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		ReasoningTokens:       reasoningTokens,
		CachedTokens:          cachedTokens,
		CacheTokens:           cacheTokens,
		TotalTokens:           totalTokens,
		LatencyMS:             readOptionalInt(record, "latency_ms", "latencyMs"),
		Failed:                readBool(record, "failed", "is_failed", "isFailed"),
		RawJSON:               readString(record, "raw_json", "rawJson"),
		CreatedAtMS:           readInt(record, "created_at_ms", "createdAtMs"),
	}
	if event.Model == "" {
		event.Model = "-"
	}
	if event.Endpoint == "" {
		event.Endpoint = "-"
	}
	if event.CreatedAtMS <= 0 {
		event.CreatedAtMS = time.Now().UnixMilli()
	}
	return event, true, nil
}

// eventsFromLegacyUsage 从旧版使用量数据中解析事件列表。
// 遍历 usage.apis.{endpoint}.models.{model}.details[] 结构，
// 将每条 detail 转换为标准 Event。
// 生成警告提示元数据可能不完整，以及来源匹配可能为近似值。
func eventsFromLegacyUsage(usageRecord map[string]any, format string) (ImportParseResult, error) {
	apisRaw, ok := usageRecord["apis"].(map[string]any)
	if !ok {
		return ImportParseResult{Format: format, Unsupported: 1}, ErrLegacyUsageNoDetails
	}

	result := ImportParseResult{
		Format: format,
		Warnings: []string{
			"legacy_usage_metadata_is_partial",
			"legacy_usage_source_matching_may_be_approximate",
		},
	}
	now := time.Now().UnixMilli()
	endpointIndex := 0
	for _, endpoint := range sortedKeys(apisRaw) {
		apiRaw := apisRaw[endpoint]
		endpointIndex++
		apiEntry, ok := apiRaw.(map[string]any)
		if !ok {
			result.Failed++
			continue
		}
		modelsRaw, ok := apiEntry["models"].(map[string]any)
		if !ok {
			result.Failed++
			continue
		}

		method, path := parseEndpoint(endpoint)
		modelIndex := 0
		for _, model := range sortedKeys(modelsRaw) {
			modelRaw := modelsRaw[model]
			modelIndex++
			modelEntry, ok := modelRaw.(map[string]any)
			if !ok {
				result.Failed++
				continue
			}
			detailsRaw, ok := modelEntry["details"].([]any)
			if !ok || len(detailsRaw) == 0 {
				result.Unsupported++
				continue
			}
			for detailIndex, detailRaw := range detailsRaw {
				detail, ok := detailRaw.(map[string]any)
				if !ok {
					result.Failed++
					continue
				}
				event, err := eventFromLegacyDetail(
					endpoint,
					method,
					path,
					model,
					detail,
					endpointIndex,
					modelIndex,
					detailIndex,
					now,
				)
				if err != nil {
					result.Failed++
					continue
				}
				result.Events = append(result.Events, event)
			}
		}
	}

	if len(result.Events) == 0 {
		return result, ErrLegacyUsageNoDetails
	}
	return result, nil
}

// eventFromLegacyDetail 从旧版使用量数据的单条 detail 记录解析为 Event。
// 使用 endpoint/model/detail 索引和时间戳组合生成稳定的 request_id（legacy:{hash}）。
// 对来源信息进行脱敏处理，并生成事件哈希用于去重。
func eventFromLegacyDetail(
	endpoint string,
	method string,
	path string,
	model string,
	detail map[string]any,
	endpointIndex int,
	modelIndex int,
	detailIndex int,
	now int64,
) (Event, error) {
	timestamp := readString(detail, "timestamp", "time", "created_at", "createdAt")
	if timestamp == "" {
		return Event{}, errors.New("legacy usage detail missing timestamp")
	}
	timestampMS, normalizedTimestamp := readTimestamp(detail)

	inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheTokens, totalTokens := readTokenFields(detail)
	if totalTokens <= 0 {
		totalTokens = inputTokens + outputTokens + reasoningTokens + maxInt64(cachedTokens, cacheTokens)
	}

	sourceRaw := readString(detail, "source", "api_key", "apiKey", "key", "account", "email")
	apiKey := readString(detail, "api_key", "apiKey", "key")
	authIndex := readString(detail, "auth_index", "authIndex", "AuthIndex")
	rawJSON := legacyRawJSON(endpoint, model, detail)
	requestID := readString(detail, "request_id", "requestId", "id")
	if requestID == "" {
		requestID = legacyRequestID(endpoint, model, normalizedTimestamp, rawJSON, endpointIndex, modelIndex, detailIndex)
	}

	event := Event{
		RequestID:             requestID,
		TimestampMS:           timestampMS,
		Timestamp:             normalizedTimestamp,
		Provider:              readString(detail, "provider", "type", "auth_type", "authType"),
		Model:                 model,
		Endpoint:              endpoint,
		Method:                method,
		Path:                  path,
		AuthType:              readString(detail, "auth_type", "authType"),
		AuthIndex:             authIndex,
		Source:                maskSource(sourceRaw),
		SourceHash:            hashString(sourceRaw),
		APIKeyHash:            hashString(apiKey),
		AccountSnapshot:       readString(detail, "account_snapshot", "accountSnapshot"),
		AuthLabelSnapshot:     readString(detail, "auth_label_snapshot", "authLabelSnapshot"),
		AuthFileSnapshot:      readString(detail, "auth_file_snapshot", "authFileSnapshot"),
		AuthProviderSnapshot:  readString(detail, "auth_provider_snapshot", "authProviderSnapshot"),
		AuthProjectIDSnapshot: readString(detail, "auth_project_id_snapshot", "authProjectIdSnapshot"),
		AuthSnapshotAtMS:      readInt(detail, "auth_snapshot_at_ms", "authSnapshotAtMs"),
		InputTokens:           inputTokens,
		OutputTokens:          outputTokens,
		ReasoningTokens:       reasoningTokens,
		CachedTokens:          cachedTokens,
		CacheTokens:           cacheTokens,
		TotalTokens:           totalTokens,
		LatencyMS:             readOptionalInt(detail, "latency_ms", "latencyMs", "duration_ms", "durationMs", "elapsed_ms", "elapsedMs"),
		Failed:                readFailed(detail),
		RawJSON:               rawJSON,
		CreatedAtMS:           now,
	}
	if event.Model == "" {
		event.Model = "-"
	}
	if event.Endpoint == "" {
		event.Endpoint = "-"
	}
	event.EventHash = buildEventHash(event)
	return event, nil
}

// legacyRawJSON 为旧版 detail 记录生成脱敏后的原始 JSON。
// 包含格式标识、端点、模型和脱敏后的 detail 数据。
func legacyRawJSON(endpoint string, model string, detail map[string]any) string {
	record := map[string]any{
		"format":   "legacy_usage_export",
		"endpoint": endpoint,
		"model":    model,
		"detail":   redactValue(detail),
	}
	raw, _ := json.Marshal(record)
	return string(raw)
}

// legacyRequestID 为旧版 detail 记录生成稳定的请求 ID。
// 格式：legacy:{hash前16位}，基于端点索引、模型索引、detail 索引、端点、模型、时间戳和原始 JSON 的组合哈希。
func legacyRequestID(endpoint string, model string, timestamp string, rawJSON string, endpointIndex int, modelIndex int, detailIndex int) string {
	raw := strings.Join([]string{
		"legacy",
		strconv.Itoa(endpointIndex),
		strconv.Itoa(modelIndex),
		strconv.Itoa(detailIndex),
		endpoint,
		model,
		timestamp,
		rawJSON,
	}, "|")
	hash := hashString(raw)
	if len(hash) > 16 {
		hash = hash[:16]
	}
	return "legacy:" + hash
}

// parseEndpoint 从端点字符串（如 "POST /v1/chat/completions"）中提取 HTTP 方法和路径。
func parseEndpoint(endpoint string) (method string, path string) {
	if match := endpointPattern.FindStringSubmatch(endpoint); len(match) == 3 {
		return strings.ToUpper(match[1]), match[2]
	}
	return "", ""
}

// hasUsageAPIs 判断记录中是否包含非空的 apis 字段。
func hasUsageAPIs(record map[string]any) bool {
	apis, ok := record["apis"].(map[string]any)
	return ok && len(apis) > 0
}

// sortedKeys 返回 map 的排序键列表。
// 确保遍历顺序一致，使旧版格式的索引生成具有确定性。
func sortedKeys(record map[string]any) []string {
	keys := make([]string, 0, len(record))
	for key := range record {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// looksLikeLegacyUsageSummary 判断记录是否为旧版使用量汇总格式。
// 匹配 total_requests、success_count、failure_count 中的任一字段。
func looksLikeLegacyUsageSummary(record map[string]any) bool {
	_, hasTotal := record["total_requests"]
	_, hasSuccess := record["success_count"]
	_, hasFailure := record["failure_count"]
	return hasTotal || hasSuccess || hasFailure
}

// readBool 从记录中读取布尔值。
// 支持 bool、json.Number、float64、string 类型的自动转换。
func readBool(record map[string]any, keys ...string) bool {
	raw := first(record, keys...)
	switch value := raw.(type) {
	case bool:
		return value
	case json.Number:
		parsed, _ := value.Int64()
		return parsed != 0
	case float64:
		return value != 0
	case string:
		normalized := strings.ToLower(strings.TrimSpace(value))
		return normalized == "1" || normalized == "true" || normalized == "yes" || normalized == "on"
	default:
		return false
	}
}

// decodeJSON 解析 JSON 数据，使用 json.Number 保留数字精度。
// 验证输入只包含一个 JSON 值（防止多值注入）。
func decodeJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("usage import payload contains multiple JSON values")
	}
	return nil
}
