// Package common - override.go
// 本文件实现了参数覆盖（Param Override）系统，用于在请求转发到上游之前动态修改请求参数。
// 参数覆盖系统支持两种模式：
//  1. 传统覆盖模式：直接以 key-value 形式覆盖请求体中的字段。
//  2. 操作覆盖模式（operations）：通过声明式的操作列表对请求进行精细修改。
//
// 操作覆盖模式支持以下操作类型：
//   - 基础操作：set、delete、copy、move
//   - 字符串操作：prepend、append、trim_prefix、trim_suffix、ensure_prefix、ensure_suffix、
//     trim_space、to_lower、to_upper、replace、regex_replace
//   - 对象操作：prune_objects（递归删除匹配条件的对象）
//   - 请求头操作：set_header、delete_header、copy_header、move_header、pass_headers
//   - 字段同步：sync_fields（在 JSON body 和 header 之间同步值）
//   - 错误拦截：return_error（在满足条件时强制返回错误）
//
// 所有操作都支持条件执行（conditions），可以基于请求体字段、上下文变量、
// 请求头等信息决定是否执行操作。条件支持 AND/OR 逻辑和取反（invert）。
//
// 参数覆盖系统还包含审计日志功能，记录关键参数（model、service_tier 等）的修改历史。
package common

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/types"
	"github.com/samber/lo"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// negativeIndexRegexp 用于匹配 JSON 路径中的负数索引（如 arr.-1）。
var negativeIndexRegexp = regexp.MustCompile(`\.(-\d+)`)

const (
	// paramOverrideContextRequestHeaders 条件上下文中请求头的键名。
	paramOverrideContextRequestHeaders = "request_headers"
	// paramOverrideContextHeaderOverride 条件上下文中覆盖头的键名。
	paramOverrideContextHeaderOverride = "header_override"
	// paramOverrideContextAuditRecorder 条件上下文中审计记录器的键名。
	paramOverrideContextAuditRecorder = "__param_override_audit_recorder"
)

// errSourceHeaderNotFound 表示在 copy/move header 操作中源 header 不存在的错误。
var errSourceHeaderNotFound = errors.New("source header does not exist")

// paramOverrideKeyAuditPaths 定义了需要记录审计日志的关键参数路径。
// 即使非调试模式下，这些参数的修改也会被记录。
var paramOverrideKeyAuditPaths = map[string]struct{}{
	"model":          {},
	"original_model": {},
	"upstream_model": {},
	"service_tier":   {},
	"inference_geo":  {},
	"speed":          {},
}

// paramOverrideAuditRecorder 是参数覆盖操作的审计日志记录器。
// 记录每个操作的模式、路径、来源、目标和值，用于调试和追踪参数修改历史。
type paramOverrideAuditRecorder struct {
	lines []string // 审计日志行列表
}

// ConditionOperation 定义了参数覆盖操作中的条件判断。
// 条件用于决定某个操作是否应该执行。
type ConditionOperation struct {
	Path           string      `json:"path"`             // JSON 路径，指向要检查的字段
	Mode           string      `json:"mode"`             // 比较模式：full、prefix、suffix、contains、gt、gte、lt、lte
	Value          interface{} `json:"value"`            // 比较的目标值
	Invert         bool        `json:"invert"`           // 取反标志，true 时将比较结果取反
	PassMissingKey bool        `json:"pass_missing_key"` // 当 JSON 中不存在该路径时，是否视为条件通过
}

// ParamOperation 定义了参数覆盖的单个操作。
// 每个操作包含目标路径、操作模式、值和可选的条件列表。
type ParamOperation struct {
	Path       string               `json:"path"`                  // 操作的目标 JSON 路径
	Mode       string               `json:"mode"`                  // 操作模式（见文件头部注释）
	Value      interface{}          `json:"value"`                 // 操作的值（set/prepend/append 等使用）
	KeepOrigin bool                 `json:"keep_origin"`           // 是否保留原值（set 操作中已有值时不覆盖）
	From       string               `json:"from,omitempty"`        // 源路径（copy/move/replace 等使用）
	To         string               `json:"to,omitempty"`          // 目标路径（copy/move 等使用）
	Conditions []ConditionOperation `json:"conditions,omitempty"`  // 条件列表，所有条件满足时才执行操作
	Logic      string               `json:"logic,omitempty"`       // 条件逻辑：AND 或 OR（默认 OR）
}

// ParamOverrideReturnError 表示 return_error 操作产生的错误。
// 当参数覆盖中配置了 return_error 操作且条件满足时，会返回此错误终止请求。
type ParamOverrideReturnError struct {
	Message    string // 错误消息
	StatusCode int    // HTTP 状态码
	Code       string // 错误代码
	Type       string // 错误类型
	SkipRetry  bool   // 是否跳过重试
}

// Error 实现 error 接口，返回错误消息。
func (e *ParamOverrideReturnError) Error() string {
	if e == nil {
		return "param override return error"
	}
	if e.Message == "" {
		return "param override return error"
	}
	return e.Message
}

// AsParamOverrideReturnError 尝试从错误中提取 ParamOverrideReturnError 类型。
// 用于在上层调用中判断错误是否由 return_error 操作产生，以便返回正确的 HTTP 状态码。
//
// 参数：
//   - err: 待检查的错误
//
// 返回值：
//   - *ParamOverrideReturnError: 提取到的错误对象（如果匹配）
//   - bool: 是否成功提取
func AsParamOverrideReturnError(err error) (*ParamOverrideReturnError, bool) {
	if err == nil {
		return nil, false
	}
	var target *ParamOverrideReturnError
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}

// NexusTokErrorFromParamOverride 将 ParamOverrideReturnError 转换为标准的 NexusTokError。
// 确保状态码在合法范围内，设置默认的错误代码和类型，并根据 SkipRetry 标志设置重试策略。
//
// 参数：
//   - err: ParamOverrideReturnError 对象
//
// 返回值：
//   - *types.NexusTokError: 标准错误对象
func NexusTokErrorFromParamOverride(err *ParamOverrideReturnError) *types.NexusTokError {
	if err == nil {
		return types.NewError(
			errors.New("param override return error is nil"),
			types.ErrorCodeChannelParamOverrideInvalid,
			types.ErrOptionWithSkipRetry(),
		)
	}

	statusCode := err.StatusCode
	if statusCode < http.StatusContinue || statusCode > http.StatusNetworkAuthenticationRequired {
		statusCode = http.StatusBadRequest
	}

	errorCode := err.Code
	if strings.TrimSpace(errorCode) == "" {
		errorCode = string(types.ErrorCodeInvalidRequest)
	}

	errorType := err.Type
	if strings.TrimSpace(errorType) == "" {
		errorType = "invalid_request_error"
	}

	message := strings.TrimSpace(err.Message)
	if message == "" {
		message = "request blocked by param override"
	}

	opts := make([]types.NexusTokErrorOptions, 0, 1)
	if err.SkipRetry {
		opts = append(opts, types.ErrOptionWithSkipRetry())
	}

	return types.WithOpenAIError(types.OpenAIError{
		Message: message,
		Type:    errorType,
		Code:    errorCode,
	}, statusCode, opts...)
}

// ApplyParamOverride 是参数覆盖的核心执行函数。
// 根据 paramOverride 的格式自动选择执行模式：
//   - 若包含 "operations" 字段：使用操作覆盖模式，支持传统覆盖与 operations 混合执行。
//   - 否则：使用传统覆盖模式，直接以 key-value 方式覆盖请求体字段。
//
// 参数：
//   - jsonData: 原始请求体的 JSON 字节数据
//   - paramOverride: 参数覆盖配置（包含操作列表或 key-value 映射）
//   - conditionContext: 条件判断的上下文信息（如请求头、重试状态等）
//
// 返回值：
//   - []byte: 处理后的 JSON 字节数据
//   - error: 处理过程中的错误
func ApplyParamOverride(jsonData []byte, paramOverride map[string]interface{}, conditionContext map[string]interface{}) ([]byte, error) {
	if len(paramOverride) == 0 {
		return jsonData, nil
	}
	auditRecorder := getParamOverrideAuditRecorder(conditionContext)

	// 尝试断言为操作格式
	if operations, ok := tryParseOperations(paramOverride); ok {
		legacyOverride := buildLegacyParamOverride(paramOverride)
		workingJSON := jsonData
		var err error
		if len(legacyOverride) > 0 {
			workingJSON, err = applyOperationsLegacy(workingJSON, legacyOverride, auditRecorder)
			if err != nil {
				return nil, err
			}
		}

		// 使用新方法
		result, err := applyOperations(string(workingJSON), operations, conditionContext)
		return []byte(result), err
	}

	// 直接使用旧方法
	return applyOperationsLegacy(jsonData, paramOverride, auditRecorder)
}

func buildLegacyParamOverride(paramOverride map[string]interface{}) map[string]interface{} {
	if len(paramOverride) == 0 {
		return nil
	}
	legacy := make(map[string]interface{}, len(paramOverride))
	for key, value := range paramOverride {
		if strings.EqualFold(strings.TrimSpace(key), "operations") {
			continue
		}
		legacy[key] = value
	}
	return legacy
}

// ApplyParamOverrideWithRelayInfo 是面向 RelayInfo 的参数覆盖入口函数。
// 从 RelayInfo 中提取参数覆盖配置和请求头信息，构建条件上下文，
// 执行参数覆盖后将运行时头覆盖同步回 RelayInfo。
// 如果启用了审计日志，还会将操作记录存储到 RelayInfo.ParamOverrideAudit 中。
//
// 参数：
//   - jsonData: 原始请求体的 JSON 字节数据
//   - info: 中继信息，包含渠道配置、请求头等
//
// 返回值：
//   - []byte: 处理后的 JSON 字节数据
//   - error: 处理过程中的错误
func ApplyParamOverrideWithRelayInfo(jsonData []byte, info *RelayInfo) ([]byte, error) {
	paramOverride := getParamOverrideMap(info)
	if len(paramOverride) == 0 {
		return jsonData, nil
	}

	overrideCtx := BuildParamOverrideContext(info)
	var recorder *paramOverrideAuditRecorder
	if shouldEnableParamOverrideAudit(paramOverride) {
		recorder = &paramOverrideAuditRecorder{}
		overrideCtx[paramOverrideContextAuditRecorder] = recorder
	}
	result, err := ApplyParamOverride(jsonData, paramOverride, overrideCtx)
	if err != nil {
		return nil, err
	}
	syncRuntimeHeaderOverrideFromContext(info, overrideCtx)
	if info != nil {
		if recorder != nil {
			info.ParamOverrideAudit = recorder.lines
		} else {
			info.ParamOverrideAudit = nil
		}
	}
	return result, nil
}

func shouldEnableParamOverrideAudit(paramOverride map[string]interface{}) bool {
	if common.DebugEnabled {
		return true
	}
	if len(paramOverride) == 0 {
		return false
	}
	if operations, ok := tryParseOperations(paramOverride); ok {
		for _, operation := range operations {
			if shouldAuditParamPath(strings.TrimSpace(operation.Path)) ||
				shouldAuditParamPath(strings.TrimSpace(operation.To)) {
				return true
			}
		}
		for key := range buildLegacyParamOverride(paramOverride) {
			if shouldAuditParamPath(strings.TrimSpace(key)) {
				return true
			}
		}
		return false
	}
	for key := range paramOverride {
		if shouldAuditParamPath(strings.TrimSpace(key)) {
			return true
		}
	}
	return false
}

func getParamOverrideAuditRecorder(context map[string]interface{}) *paramOverrideAuditRecorder {
	if context == nil {
		return nil
	}
	recorder, _ := context[paramOverrideContextAuditRecorder].(*paramOverrideAuditRecorder)
	return recorder
}

func (r *paramOverrideAuditRecorder) recordOperation(mode, path, from, to string, value interface{}) {
	if r == nil {
		return
	}
	line := buildParamOverrideAuditLine(mode, path, from, to, value)
	if line == "" {
		return
	}
	if lo.Contains(r.lines, line) {
		return
	}
	r.lines = append(r.lines, line)
}

func shouldAuditParamPath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if common.DebugEnabled {
		return true
	}
	_, ok := paramOverrideKeyAuditPaths[path]
	return ok
}

func shouldAuditOperation(mode, path, from, to string) bool {
	if common.DebugEnabled {
		return true
	}
	for _, candidate := range []string{path, to} {
		if shouldAuditParamPath(candidate) {
			return true
		}
	}
	return false
}

func formatParamOverrideAuditValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return "<empty>"
	case string:
		return typed
	default:
		return common.GetJsonString(typed)
	}
}

func buildParamOverrideAuditLine(mode, path, from, to string, value interface{}) string {
	mode = strings.TrimSpace(mode)
	path = strings.TrimSpace(path)
	from = strings.TrimSpace(from)
	to = strings.TrimSpace(to)

	if !shouldAuditOperation(mode, path, from, to) {
		return ""
	}

	switch mode {
	case "set":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("set %s = %s", path, formatParamOverrideAuditValue(value))
	case "delete":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("delete %s", path)
	case "copy":
		if from == "" || to == "" {
			return ""
		}
		return fmt.Sprintf("copy %s -> %s", from, to)
	case "move":
		if from == "" || to == "" {
			return ""
		}
		return fmt.Sprintf("move %s -> %s", from, to)
	case "prepend":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("prepend %s with %s", path, formatParamOverrideAuditValue(value))
	case "append":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("append %s with %s", path, formatParamOverrideAuditValue(value))
	case "trim_prefix", "trim_suffix", "ensure_prefix", "ensure_suffix":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("%s %s with %s", mode, path, formatParamOverrideAuditValue(value))
	case "trim_space", "to_lower", "to_upper":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("%s %s", mode, path)
	case "replace", "regex_replace":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("%s %s from %s to %s", mode, path, from, to)
	case "set_header":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("set_header %s = %s", path, formatParamOverrideAuditValue(value))
	case "delete_header":
		if path == "" {
			return ""
		}
		return fmt.Sprintf("delete_header %s", path)
	case "copy_header", "move_header":
		if from == "" || to == "" {
			return ""
		}
		return fmt.Sprintf("%s %s -> %s", mode, from, to)
	case "pass_headers":
		return fmt.Sprintf("pass_headers %s", formatParamOverrideAuditValue(value))
	case "sync_fields":
		if from == "" || to == "" {
			return ""
		}
		return fmt.Sprintf("sync_fields %s -> %s", from, to)
	case "return_error":
		return fmt.Sprintf("return_error %s", formatParamOverrideAuditValue(value))
	default:
		if path == "" {
			return mode
		}
		return fmt.Sprintf("%s %s", mode, path)
	}
}

func getParamOverrideMap(info *RelayInfo) map[string]interface{} {
	if info == nil || info.ChannelMeta == nil {
		return nil
	}
	return info.ChannelMeta.ParamOverride
}

func getHeaderOverrideMap(info *RelayInfo) map[string]interface{} {
	if info == nil || info.ChannelMeta == nil {
		return nil
	}
	return info.ChannelMeta.HeadersOverride
}

func sanitizeHeaderOverrideMap(source map[string]interface{}) map[string]interface{} {
	if len(source) == 0 {
		return map[string]interface{}{}
	}
	target := make(map[string]interface{}, len(source))
	for key, value := range source {
		normalizedKey := normalizeHeaderContextKey(key)
		if normalizedKey == "" {
			continue
		}
		normalizedValue := strings.TrimSpace(fmt.Sprintf("%v", value))
		if normalizedValue == "" {
			if isHeaderPassthroughRuleKeyForOverride(normalizedKey) {
				target[normalizedKey] = ""
			}
			continue
		}
		target[normalizedKey] = normalizedValue
	}
	return target
}

func isHeaderPassthroughRuleKeyForOverride(key string) bool {
	key = strings.TrimSpace(strings.ToLower(key))
	if key == "" {
		return false
	}
	if key == "*" {
		return true
	}
	return strings.HasPrefix(key, "re:") || strings.HasPrefix(key, "regex:")
}

// GetEffectiveHeaderOverride 获取最终生效的请求头覆盖映射。
// 优先使用运行时头覆盖（RuntimeHeadersOverride），若未启用则使用渠道的静态头覆盖（HeadersOverride）。
// 返回的映射中的键已统一转换为小写形式。
//
// 参数：
//   - info: 中继信息
//
// 返回值：
//   - map[string]interface{}: 生效的头覆盖映射（键为小写）
func GetEffectiveHeaderOverride(info *RelayInfo) map[string]interface{} {
	if info == nil {
		return map[string]interface{}{}
	}
	if info.UseRuntimeHeadersOverride {
		return sanitizeHeaderOverrideMap(info.RuntimeHeadersOverride)
	}
	return sanitizeHeaderOverrideMap(getHeaderOverrideMap(info))
}

func tryParseOperations(paramOverride map[string]interface{}) ([]ParamOperation, bool) {
	// 检查是否包含 "operations" 字段
	opsValue, exists := paramOverride["operations"]
	if !exists {
		return nil, false
	}

	var opMaps []map[string]interface{}
	switch ops := opsValue.(type) {
	case []interface{}:
		opMaps = make([]map[string]interface{}, 0, len(ops))
		for _, op := range ops {
			opMap, ok := op.(map[string]interface{})
			if !ok {
				return nil, false
			}
			opMaps = append(opMaps, opMap)
		}
	case []map[string]interface{}:
		opMaps = ops
	default:
		return nil, false
	}

	operations := make([]ParamOperation, 0, len(opMaps))
	for _, opMap := range opMaps {
		operation := ParamOperation{}

		// 断言必要字段
		if path, ok := opMap["path"].(string); ok {
			operation.Path = path
		}
		if mode, ok := opMap["mode"].(string); ok {
			operation.Mode = mode
		} else {
			return nil, false // mode 是必需的
		}

		// 可选字段
		if value, exists := opMap["value"]; exists {
			operation.Value = value
		}
		if keepOrigin, ok := opMap["keep_origin"].(bool); ok {
			operation.KeepOrigin = keepOrigin
		}
		if from, ok := opMap["from"].(string); ok {
			operation.From = from
		}
		if to, ok := opMap["to"].(string); ok {
			operation.To = to
		}
		if logic, ok := opMap["logic"].(string); ok {
			operation.Logic = logic
		} else {
			operation.Logic = "OR" // 默认为OR
		}

		// 解析条件
		if conditions, exists := opMap["conditions"]; exists {
			parsedConditions, err := parseConditionOperations(conditions)
			if err != nil {
				return nil, false
			}
			operation.Conditions = append(operation.Conditions, parsedConditions...)
		}

		operations = append(operations, operation)
	}
	return operations, true
}

func checkConditions(jsonStr, contextJSON string, conditions []ConditionOperation, logic string) (bool, error) {
	if len(conditions) == 0 {
		return true, nil // 没有条件，直接通过
	}
	results := make([]bool, len(conditions))
	for i, condition := range conditions {
		result, err := checkSingleCondition(jsonStr, contextJSON, condition)
		if err != nil {
			return false, err
		}
		results[i] = result
	}

	if strings.ToUpper(logic) == "AND" {
		return lo.EveryBy(results, func(item bool) bool { return item }), nil
	}
	return lo.SomeBy(results, func(item bool) bool { return item }), nil
}

func checkSingleCondition(jsonStr, contextJSON string, condition ConditionOperation) (bool, error) {
	// 处理负数索引
	path := processNegativeIndex(jsonStr, condition.Path)
	value := gjson.Get(jsonStr, path)
	if !value.Exists() && contextJSON != "" {
		value = gjson.Get(contextJSON, condition.Path)
	}
	if !value.Exists() {
		if condition.PassMissingKey {
			return true, nil
		}
		return false, nil
	}

	// 利用gjson的类型解析
	targetBytes, err := common.Marshal(condition.Value)
	if err != nil {
		return false, fmt.Errorf("failed to marshal condition value: %v", err)
	}
	targetValue := gjson.ParseBytes(targetBytes)

	result, err := compareGjsonValues(value, targetValue, strings.ToLower(condition.Mode))
	if err != nil {
		return false, fmt.Errorf("comparison failed for path %s: %v", condition.Path, err)
	}

	if condition.Invert {
		result = !result
	}
	return result, nil
}

func processNegativeIndex(jsonStr string, path string) string {
	matches := negativeIndexRegexp.FindAllStringSubmatch(path, -1)

	if len(matches) == 0 {
		return path
	}

	result := path
	for _, match := range matches {
		negIndex := match[1]
		index, _ := strconv.Atoi(negIndex)

		arrayPath := strings.Split(path, negIndex)[0]
		if strings.HasSuffix(arrayPath, ".") {
			arrayPath = arrayPath[:len(arrayPath)-1]
		}

		array := gjson.Get(jsonStr, arrayPath)
		if array.IsArray() {
			length := len(array.Array())
			actualIndex := length + index
			if actualIndex >= 0 && actualIndex < length {
				result = strings.Replace(result, match[0], "."+strconv.Itoa(actualIndex), 1)
			}
		}
	}

	return result
}

// compareGjsonValues 直接比较两个gjson.Result，支持所有比较模式
func compareGjsonValues(jsonValue, targetValue gjson.Result, mode string) (bool, error) {
	switch mode {
	case "full":
		return compareEqual(jsonValue, targetValue)
	case "prefix":
		return strings.HasPrefix(jsonValue.String(), targetValue.String()), nil
	case "suffix":
		return strings.HasSuffix(jsonValue.String(), targetValue.String()), nil
	case "contains":
		return strings.Contains(jsonValue.String(), targetValue.String()), nil
	case "gt":
		return compareNumeric(jsonValue, targetValue, "gt")
	case "gte":
		return compareNumeric(jsonValue, targetValue, "gte")
	case "lt":
		return compareNumeric(jsonValue, targetValue, "lt")
	case "lte":
		return compareNumeric(jsonValue, targetValue, "lte")
	default:
		return false, fmt.Errorf("unsupported comparison mode: %s", mode)
	}
}

func compareEqual(jsonValue, targetValue gjson.Result) (bool, error) {
	// 对null值特殊处理：两个都是null返回true，一个是null另一个不是返回false
	if jsonValue.Type == gjson.Null || targetValue.Type == gjson.Null {
		return jsonValue.Type == gjson.Null && targetValue.Type == gjson.Null, nil
	}

	// 对布尔值特殊处理
	if (jsonValue.Type == gjson.True || jsonValue.Type == gjson.False) &&
		(targetValue.Type == gjson.True || targetValue.Type == gjson.False) {
		return jsonValue.Bool() == targetValue.Bool(), nil
	}

	// 如果类型不同，报错
	if jsonValue.Type != targetValue.Type {
		return false, fmt.Errorf("compare for different types, got %v and %v", jsonValue.Type, targetValue.Type)
	}

	switch jsonValue.Type {
	case gjson.True, gjson.False:
		return jsonValue.Bool() == targetValue.Bool(), nil
	case gjson.Number:
		return jsonValue.Num == targetValue.Num, nil
	case gjson.String:
		return jsonValue.String() == targetValue.String(), nil
	default:
		return jsonValue.String() == targetValue.String(), nil
	}
}

func compareNumeric(jsonValue, targetValue gjson.Result, operator string) (bool, error) {
	// 只有数字类型才支持数值比较
	if jsonValue.Type != gjson.Number || targetValue.Type != gjson.Number {
		return false, fmt.Errorf("numeric comparison requires both values to be numbers, got %v and %v", jsonValue.Type, targetValue.Type)
	}

	jsonNum := jsonValue.Num
	targetNum := targetValue.Num

	switch operator {
	case "gt":
		return jsonNum > targetNum, nil
	case "gte":
		return jsonNum >= targetNum, nil
	case "lt":
		return jsonNum < targetNum, nil
	case "lte":
		return jsonNum <= targetNum, nil
	default:
		return false, fmt.Errorf("unsupported numeric operator: %s", operator)
	}
}

// applyOperationsLegacy 原参数覆盖方法
func applyOperationsLegacy(jsonData []byte, paramOverride map[string]interface{}, auditRecorder *paramOverrideAuditRecorder) ([]byte, error) {
	reqMap := make(map[string]interface{})
	err := common.Unmarshal(jsonData, &reqMap)
	if err != nil {
		return nil, err
	}

	for key, value := range paramOverride {
		reqMap[key] = value
		auditRecorder.recordOperation("set", key, "", "", value)
	}

	return common.Marshal(reqMap)
}

func applyOperations(jsonStr string, operations []ParamOperation, conditionContext map[string]interface{}) (string, error) {
	context := ensureContextMap(conditionContext)
	auditRecorder := getParamOverrideAuditRecorder(context)
	contextJSON, err := marshalContextJSON(context)
	if err != nil {
		return "", fmt.Errorf("failed to marshal condition context: %v", err)
	}

	result := jsonStr
	for _, op := range operations {
		// 检查条件是否满足
		ok, err := checkConditions(result, contextJSON, op.Conditions, op.Logic)
		if err != nil {
			return "", err
		}
		if !ok {
			continue // 条件不满足，跳过当前操作
		}
		// 处理路径中的负数索引
		opPath := processNegativeIndex(result, op.Path)
		var opPaths []string
		if isPathBasedOperation(op.Mode) {
			opPaths, err = resolveOperationPaths(result, opPath)
			if err != nil {
				return "", err
			}
			if len(opPaths) == 0 {
				continue
			}
		}

		switch op.Mode {
		case "delete":
			for _, path := range opPaths {
				result, err = deleteValue(result, path)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("delete", path, "", "", nil)
			}
		case "set":
			for _, path := range opPaths {
				if op.KeepOrigin && gjson.Get(result, path).Exists() {
					continue
				}
				result, err = sjson.Set(result, path, op.Value)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("set", path, "", "", op.Value)
			}
		case "move":
			opFrom := processNegativeIndex(result, op.From)
			opTo := processNegativeIndex(result, op.To)
			result, err = moveValue(result, opFrom, opTo)
			if err == nil {
				auditRecorder.recordOperation("move", "", opFrom, opTo, nil)
			}
		case "copy":
			if op.From == "" || op.To == "" {
				return "", fmt.Errorf("copy from/to is required")
			}
			opFrom := processNegativeIndex(result, op.From)
			opTo := processNegativeIndex(result, op.To)
			result, err = copyValue(result, opFrom, opTo)
			if err == nil {
				auditRecorder.recordOperation("copy", "", opFrom, opTo, nil)
			}
		case "prepend":
			for _, path := range opPaths {
				result, err = modifyValue(result, path, op.Value, op.KeepOrigin, true)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("prepend", path, "", "", op.Value)
			}
		case "append":
			for _, path := range opPaths {
				result, err = modifyValue(result, path, op.Value, op.KeepOrigin, false)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("append", path, "", "", op.Value)
			}
		case "trim_prefix":
			for _, path := range opPaths {
				result, err = trimStringValue(result, path, op.Value, true)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("trim_prefix", path, "", "", op.Value)
			}
		case "trim_suffix":
			for _, path := range opPaths {
				result, err = trimStringValue(result, path, op.Value, false)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("trim_suffix", path, "", "", op.Value)
			}
		case "ensure_prefix":
			for _, path := range opPaths {
				result, err = ensureStringAffix(result, path, op.Value, true)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("ensure_prefix", path, "", "", op.Value)
			}
		case "ensure_suffix":
			for _, path := range opPaths {
				result, err = ensureStringAffix(result, path, op.Value, false)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("ensure_suffix", path, "", "", op.Value)
			}
		case "trim_space":
			for _, path := range opPaths {
				result, err = transformStringValue(result, path, strings.TrimSpace)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("trim_space", path, "", "", nil)
			}
		case "to_lower":
			for _, path := range opPaths {
				result, err = transformStringValue(result, path, strings.ToLower)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("to_lower", path, "", "", nil)
			}
		case "to_upper":
			for _, path := range opPaths {
				result, err = transformStringValue(result, path, strings.ToUpper)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("to_upper", path, "", "", nil)
			}
		case "replace":
			for _, path := range opPaths {
				result, err = replaceStringValue(result, path, op.From, op.To)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("replace", path, op.From, op.To, nil)
			}
		case "regex_replace":
			for _, path := range opPaths {
				result, err = regexReplaceStringValue(result, path, op.From, op.To)
				if err != nil {
					break
				}
				auditRecorder.recordOperation("regex_replace", path, op.From, op.To, nil)
			}
		case "return_error":
			auditRecorder.recordOperation("return_error", op.Path, "", "", op.Value)
			returnErr, parseErr := parseParamOverrideReturnError(op.Value)
			if parseErr != nil {
				return "", parseErr
			}
			return "", returnErr
		case "prune_objects":
			for _, path := range opPaths {
				result, err = pruneObjects(result, path, contextJSON, op.Value)
				if err != nil {
					break
				}
			}
		case "set_header":
			err = setHeaderOverrideInContext(context, op.Path, op.Value, op.KeepOrigin)
			if err == nil {
				auditRecorder.recordOperation("set_header", op.Path, "", "", op.Value)
				contextJSON, err = marshalContextJSON(context)
			}
		case "delete_header":
			err = deleteHeaderOverrideInContext(context, op.Path)
			if err == nil {
				auditRecorder.recordOperation("delete_header", op.Path, "", "", nil)
				contextJSON, err = marshalContextJSON(context)
			}
		case "copy_header":
			sourceHeader := strings.TrimSpace(op.From)
			targetHeader := strings.TrimSpace(op.To)
			if sourceHeader == "" {
				sourceHeader = strings.TrimSpace(op.Path)
			}
			if targetHeader == "" {
				targetHeader = strings.TrimSpace(op.Path)
			}
			err = copyHeaderInContext(context, sourceHeader, targetHeader, op.KeepOrigin)
			if errors.Is(err, errSourceHeaderNotFound) {
				err = nil
			}
			if err == nil {
				auditRecorder.recordOperation("copy_header", "", sourceHeader, targetHeader, nil)
				contextJSON, err = marshalContextJSON(context)
			}
		case "move_header":
			sourceHeader := strings.TrimSpace(op.From)
			targetHeader := strings.TrimSpace(op.To)
			if sourceHeader == "" {
				sourceHeader = strings.TrimSpace(op.Path)
			}
			if targetHeader == "" {
				targetHeader = strings.TrimSpace(op.Path)
			}
			err = moveHeaderInContext(context, sourceHeader, targetHeader, op.KeepOrigin)
			if errors.Is(err, errSourceHeaderNotFound) {
				err = nil
			}
			if err == nil {
				auditRecorder.recordOperation("move_header", "", sourceHeader, targetHeader, nil)
				contextJSON, err = marshalContextJSON(context)
			}
		case "pass_headers":
			headerNames, parseErr := parseHeaderPassThroughNames(op.Value)
			if parseErr != nil {
				return "", parseErr
			}
			for _, headerName := range headerNames {
				if err = copyHeaderInContext(context, headerName, headerName, op.KeepOrigin); err != nil {
					if errors.Is(err, errSourceHeaderNotFound) {
						err = nil
						continue
					}
					break
				}
			}
			if err == nil {
				auditRecorder.recordOperation("pass_headers", "", "", "", headerNames)
				contextJSON, err = marshalContextJSON(context)
			}
		case "sync_fields":
			result, err = syncFieldsBetweenTargets(result, context, op.From, op.To)
			if err == nil {
				auditRecorder.recordOperation("sync_fields", "", op.From, op.To, nil)
				contextJSON, err = marshalContextJSON(context)
			}
		default:
			return "", fmt.Errorf("unknown operation: %s", op.Mode)
		}
		if err != nil {
			return "", fmt.Errorf("operation %s failed: %w", op.Mode, err)
		}
	}
	return result, nil
}

func parseParamOverrideReturnError(value interface{}) (*ParamOverrideReturnError, error) {
	result := &ParamOverrideReturnError{
		StatusCode: http.StatusBadRequest,
		Code:       string(types.ErrorCodeInvalidRequest),
		Type:       "invalid_request_error",
		SkipRetry:  true,
	}

	switch raw := value.(type) {
	case nil:
		return nil, fmt.Errorf("return_error value is required")
	case string:
		result.Message = strings.TrimSpace(raw)
	case map[string]interface{}:
		if message, ok := raw["message"].(string); ok {
			result.Message = strings.TrimSpace(message)
		}
		if result.Message == "" {
			if message, ok := raw["msg"].(string); ok {
				result.Message = strings.TrimSpace(message)
			}
		}

		if code, exists := raw["code"]; exists {
			codeStr := strings.TrimSpace(fmt.Sprintf("%v", code))
			if codeStr != "" {
				result.Code = codeStr
			}
		}
		if errType, ok := raw["type"].(string); ok {
			errType = strings.TrimSpace(errType)
			if errType != "" {
				result.Type = errType
			}
		}
		if skipRetry, ok := raw["skip_retry"].(bool); ok {
			result.SkipRetry = skipRetry
		}

		if statusCodeRaw, exists := raw["status_code"]; exists {
			statusCode, ok := parseOverrideInt(statusCodeRaw)
			if !ok {
				return nil, fmt.Errorf("return_error status_code must be an integer")
			}
			result.StatusCode = statusCode
		} else if statusRaw, exists := raw["status"]; exists {
			statusCode, ok := parseOverrideInt(statusRaw)
			if !ok {
				return nil, fmt.Errorf("return_error status must be an integer")
			}
			result.StatusCode = statusCode
		}
	default:
		return nil, fmt.Errorf("return_error value must be string or object")
	}

	if result.Message == "" {
		return nil, fmt.Errorf("return_error message is required")
	}
	if result.StatusCode < http.StatusContinue || result.StatusCode > http.StatusNetworkAuthenticationRequired {
		return nil, fmt.Errorf("return_error status code out of range: %d", result.StatusCode)
	}

	return result, nil
}

func parseOverrideInt(v interface{}) (int, bool) {
	switch value := v.(type) {
	case int:
		return value, true
	case float64:
		if value != float64(int(value)) {
			return 0, false
		}
		return int(value), true
	default:
		return 0, false
	}
}

func ensureContextMap(conditionContext map[string]interface{}) map[string]interface{} {
	if conditionContext != nil {
		return conditionContext
	}
	return make(map[string]interface{})
}

func marshalContextJSON(context map[string]interface{}) (string, error) {
	if context == nil || len(context) == 0 {
		return "", nil
	}
	ctxBytes, err := common.Marshal(context)
	if err != nil {
		return "", err
	}
	return string(ctxBytes), nil
}

func setHeaderOverrideInContext(context map[string]interface{}, headerName string, value interface{}, keepOrigin bool) error {
	headerName = normalizeHeaderContextKey(headerName)
	if headerName == "" {
		return fmt.Errorf("header name is required")
	}

	rawHeaders := ensureMapKeyInContext(context, paramOverrideContextHeaderOverride)
	if keepOrigin {
		if existing, ok := rawHeaders[headerName]; ok {
			existingValue := strings.TrimSpace(fmt.Sprintf("%v", existing))
			if existingValue != "" {
				return nil
			}
		}
	}

	headerValue, hasValue, err := resolveHeaderOverrideValue(context, headerName, value)
	if err != nil {
		return err
	}
	if !hasValue {
		delete(rawHeaders, headerName)
		return nil
	}

	rawHeaders[headerName] = headerValue
	return nil
}

func resolveHeaderOverrideValue(context map[string]interface{}, headerName string, value interface{}) (string, bool, error) {
	if value == nil {
		return "", false, fmt.Errorf("header value is required")
	}

	if mapping, ok := value.(map[string]interface{}); ok {
		return resolveHeaderOverrideValueByMapping(context, headerName, mapping)
	}
	if mapping, ok := value.(map[string]string); ok {
		converted := make(map[string]interface{}, len(mapping))
		for key, item := range mapping {
			converted[key] = item
		}
		return resolveHeaderOverrideValueByMapping(context, headerName, converted)
	}

	headerValue := strings.TrimSpace(fmt.Sprintf("%v", value))
	if headerValue == "" {
		return "", false, nil
	}
	return headerValue, true, nil
}

func resolveHeaderOverrideValueByMapping(context map[string]interface{}, headerName string, mapping map[string]interface{}) (string, bool, error) {
	if len(mapping) == 0 {
		return "", false, fmt.Errorf("header value mapping cannot be empty")
	}

	appendTokens, err := parseHeaderAppendTokens(mapping)
	if err != nil {
		return "", false, err
	}
	keepOnlyDeclared := parseHeaderKeepOnlyDeclared(mapping)

	sourceValue, exists := getHeaderValueFromContext(context, headerName)
	sourceTokens := make([]string, 0)
	if exists {
		sourceTokens = splitHeaderListValue(sourceValue)
	}

	wildcardValue, hasWildcard := mapping["*"]
	resultTokens := make([]string, 0, len(sourceTokens)+len(appendTokens))
	for _, token := range sourceTokens {
		replacementRaw, hasReplacement := mapping[token]
		if !hasReplacement && hasWildcard && !keepOnlyDeclared {
			replacementRaw = wildcardValue
			hasReplacement = true
		}
		if !hasReplacement {
			if keepOnlyDeclared {
				continue
			}
			resultTokens = append(resultTokens, token)
			continue
		}
		replacementTokens, err := parseHeaderReplacementTokens(replacementRaw)
		if err != nil {
			return "", false, err
		}
		resultTokens = append(resultTokens, replacementTokens...)
	}

	resultTokens = append(resultTokens, appendTokens...)
	resultTokens = lo.Uniq(resultTokens)
	if len(resultTokens) == 0 {
		return "", false, nil
	}
	return strings.Join(resultTokens, ","), true, nil
}

func parseHeaderAppendTokens(mapping map[string]interface{}) ([]string, error) {
	appendRaw, ok := mapping["$append"]
	if !ok {
		return nil, nil
	}
	return parseHeaderReplacementTokens(appendRaw)
}

func parseHeaderKeepOnlyDeclared(mapping map[string]interface{}) bool {
	keepOnlyDeclaredRaw, ok := mapping["$keep_only_declared"]
	if !ok {
		return false
	}
	keepOnlyDeclared, ok := keepOnlyDeclaredRaw.(bool)
	if !ok {
		return false
	}
	return keepOnlyDeclared
}

func parseHeaderReplacementTokens(value interface{}) ([]string, error) {
	switch raw := value.(type) {
	case nil:
		return nil, nil
	case string:
		return splitHeaderListValue(raw), nil
	case []string:
		tokens := make([]string, 0, len(raw))
		for _, item := range raw {
			tokens = append(tokens, splitHeaderListValue(item)...)
		}
		return lo.Uniq(tokens), nil
	case []interface{}:
		tokens := make([]string, 0, len(raw))
		for _, item := range raw {
			itemTokens, err := parseHeaderReplacementTokens(item)
			if err != nil {
				return nil, err
			}
			tokens = append(tokens, itemTokens...)
		}
		return lo.Uniq(tokens), nil
	case map[string]interface{}, map[string]string:
		return nil, fmt.Errorf("header replacement value must be string, array or null")
	default:
		token := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if token == "" {
			return nil, nil
		}
		return []string{token}, nil
	}
}

func splitHeaderListValue(raw string) []string {
	items := strings.Split(raw, ",")
	return lo.FilterMap(items, func(item string, _ int) (string, bool) {
		token := strings.TrimSpace(item)
		if token == "" {
			return "", false
		}
		return token, true
	})
}

func copyHeaderInContext(context map[string]interface{}, fromHeader, toHeader string, keepOrigin bool) error {
	fromHeader = normalizeHeaderContextKey(fromHeader)
	toHeader = normalizeHeaderContextKey(toHeader)
	if fromHeader == "" || toHeader == "" {
		return fmt.Errorf("copy_header from/to is required")
	}
	value, exists := getHeaderValueFromContext(context, fromHeader)
	if !exists {
		return fmt.Errorf("%w: %s", errSourceHeaderNotFound, fromHeader)
	}
	return setHeaderOverrideInContext(context, toHeader, value, keepOrigin)
}

func moveHeaderInContext(context map[string]interface{}, fromHeader, toHeader string, keepOrigin bool) error {
	fromHeader = normalizeHeaderContextKey(fromHeader)
	toHeader = normalizeHeaderContextKey(toHeader)
	if fromHeader == "" || toHeader == "" {
		return fmt.Errorf("move_header from/to is required")
	}
	if err := copyHeaderInContext(context, fromHeader, toHeader, keepOrigin); err != nil {
		return err
	}
	if strings.EqualFold(fromHeader, toHeader) {
		return nil
	}
	return deleteHeaderOverrideInContext(context, fromHeader)
}

func deleteHeaderOverrideInContext(context map[string]interface{}, headerName string) error {
	headerName = normalizeHeaderContextKey(headerName)
	if headerName == "" {
		return fmt.Errorf("header name is required")
	}
	rawHeaders := ensureMapKeyInContext(context, paramOverrideContextHeaderOverride)
	delete(rawHeaders, headerName)
	return nil
}

func parseHeaderPassThroughNames(value interface{}) ([]string, error) {
	normalizeNames := func(values []string) []string {
		names := lo.FilterMap(values, func(item string, _ int) (string, bool) {
			headerName := normalizeHeaderContextKey(item)
			if headerName == "" {
				return "", false
			}
			return headerName, true
		})
		return lo.Uniq(names)
	}

	switch raw := value.(type) {
	case nil:
		return nil, fmt.Errorf("pass_headers value is required")
	case string:
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" {
			return nil, fmt.Errorf("pass_headers value is required")
		}
		if strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "{") {
			var parsed interface{}
			if err := common.UnmarshalJsonStr(trimmed, &parsed); err == nil {
				return parseHeaderPassThroughNames(parsed)
			}
		}
		names := normalizeNames(strings.Split(trimmed, ","))
		if len(names) == 0 {
			return nil, fmt.Errorf("pass_headers value is invalid")
		}
		return names, nil
	case []interface{}:
		names := lo.FilterMap(raw, func(item interface{}, _ int) (string, bool) {
			headerName := normalizeHeaderContextKey(fmt.Sprintf("%v", item))
			if headerName == "" {
				return "", false
			}
			return headerName, true
		})
		names = lo.Uniq(names)
		if len(names) == 0 {
			return nil, fmt.Errorf("pass_headers value is invalid")
		}
		return names, nil
	case []string:
		names := lo.FilterMap(raw, func(item string, _ int) (string, bool) {
			headerName := normalizeHeaderContextKey(item)
			if headerName == "" {
				return "", false
			}
			return headerName, true
		})
		names = lo.Uniq(names)
		if len(names) == 0 {
			return nil, fmt.Errorf("pass_headers value is invalid")
		}
		return names, nil
	case map[string]interface{}:
		candidates := make([]string, 0, 8)
		if headersRaw, ok := raw["headers"]; ok {
			names, err := parseHeaderPassThroughNames(headersRaw)
			if err == nil {
				candidates = append(candidates, names...)
			}
		}
		if namesRaw, ok := raw["names"]; ok {
			names, err := parseHeaderPassThroughNames(namesRaw)
			if err == nil {
				candidates = append(candidates, names...)
			}
		}
		if headerRaw, ok := raw["header"]; ok {
			names, err := parseHeaderPassThroughNames(headerRaw)
			if err == nil {
				candidates = append(candidates, names...)
			}
		}
		names := normalizeNames(candidates)
		if len(names) == 0 {
			return nil, fmt.Errorf("pass_headers value is invalid")
		}
		return names, nil
	default:
		return nil, fmt.Errorf("pass_headers value must be string, array or object")
	}
}

type syncTarget struct {
	kind string
	key  string
}

func parseSyncTarget(spec string) (syncTarget, error) {
	raw := strings.TrimSpace(spec)
	if raw == "" {
		return syncTarget{}, fmt.Errorf("sync_fields target is required")
	}

	idx := strings.Index(raw, ":")
	if idx < 0 {
		// Backward compatibility: treat bare value as JSON path.
		return syncTarget{
			kind: "json",
			key:  raw,
		}, nil
	}

	kind := strings.ToLower(strings.TrimSpace(raw[:idx]))
	key := strings.TrimSpace(raw[idx+1:])
	if key == "" {
		return syncTarget{}, fmt.Errorf("sync_fields target key is required: %s", raw)
	}

	switch kind {
	case "json", "body":
		return syncTarget{
			kind: "json",
			key:  key,
		}, nil
	case "header":
		return syncTarget{
			kind: "header",
			key:  key,
		}, nil
	default:
		return syncTarget{}, fmt.Errorf("sync_fields target prefix is invalid: %s", raw)
	}
}

func readSyncTargetValue(jsonStr string, context map[string]interface{}, target syncTarget) (interface{}, bool, error) {
	switch target.kind {
	case "json":
		path := processNegativeIndex(jsonStr, target.key)
		value := gjson.Get(jsonStr, path)
		if !value.Exists() || value.Type == gjson.Null {
			return nil, false, nil
		}
		if value.Type == gjson.String && strings.TrimSpace(value.String()) == "" {
			return nil, false, nil
		}
		return value.Value(), true, nil
	case "header":
		value, ok := getHeaderValueFromContext(context, target.key)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, false, nil
		}
		return value, true, nil
	default:
		return nil, false, fmt.Errorf("unsupported sync_fields target kind: %s", target.kind)
	}
}

func writeSyncTargetValue(jsonStr string, context map[string]interface{}, target syncTarget, value interface{}) (string, error) {
	switch target.kind {
	case "json":
		path := processNegativeIndex(jsonStr, target.key)
		nextJSON, err := sjson.Set(jsonStr, path, value)
		if err != nil {
			return "", err
		}
		return nextJSON, nil
	case "header":
		if err := setHeaderOverrideInContext(context, target.key, value, false); err != nil {
			return "", err
		}
		return jsonStr, nil
	default:
		return "", fmt.Errorf("unsupported sync_fields target kind: %s", target.kind)
	}
}

func syncFieldsBetweenTargets(jsonStr string, context map[string]interface{}, fromSpec string, toSpec string) (string, error) {
	fromTarget, err := parseSyncTarget(fromSpec)
	if err != nil {
		return "", err
	}
	toTarget, err := parseSyncTarget(toSpec)
	if err != nil {
		return "", err
	}

	fromValue, fromExists, err := readSyncTargetValue(jsonStr, context, fromTarget)
	if err != nil {
		return "", err
	}
	toValue, toExists, err := readSyncTargetValue(jsonStr, context, toTarget)
	if err != nil {
		return "", err
	}

	// If one side exists and the other side is missing, sync the missing side.
	if fromExists && !toExists {
		return writeSyncTargetValue(jsonStr, context, toTarget, fromValue)
	}
	if toExists && !fromExists {
		return writeSyncTargetValue(jsonStr, context, fromTarget, toValue)
	}
	return jsonStr, nil
}

func ensureMapKeyInContext(context map[string]interface{}, key string) map[string]interface{} {
	if context == nil {
		return map[string]interface{}{}
	}
	if existing, ok := context[key]; ok {
		if mapVal, ok := existing.(map[string]interface{}); ok {
			return mapVal
		}
	}
	result := make(map[string]interface{})
	context[key] = result
	return result
}

func getHeaderValueFromContext(context map[string]interface{}, headerName string) (string, bool) {
	headerName = normalizeHeaderContextKey(headerName)
	if headerName == "" {
		return "", false
	}
	for _, key := range []string{paramOverrideContextHeaderOverride, paramOverrideContextRequestHeaders} {
		source := ensureMapKeyInContext(context, key)
		raw, ok := source[headerName]
		if !ok {
			continue
		}
		value := strings.TrimSpace(fmt.Sprintf("%v", raw))
		if value != "" {
			return value, true
		}
	}
	return "", false
}

func normalizeHeaderContextKey(key string) string {
	return strings.TrimSpace(strings.ToLower(key))
}

func buildRequestHeadersContext(headers map[string]string) map[string]interface{} {
	if len(headers) == 0 {
		return map[string]interface{}{}
	}
	entries := lo.Entries(headers)
	normalizedEntries := lo.FilterMap(entries, func(item lo.Entry[string, string], _ int) (lo.Entry[string, string], bool) {
		normalized := normalizeHeaderContextKey(item.Key)
		value := strings.TrimSpace(item.Value)
		if normalized == "" || value == "" {
			return lo.Entry[string, string]{}, false
		}
		return lo.Entry[string, string]{Key: normalized, Value: value}, true
	})
	return lo.SliceToMap(normalizedEntries, func(item lo.Entry[string, string]) (string, interface{}) {
		return item.Key, item.Value
	})
}

func syncRuntimeHeaderOverrideFromContext(info *RelayInfo, context map[string]interface{}) {
	if info == nil || context == nil {
		return
	}
	raw, exists := context[paramOverrideContextHeaderOverride]
	if !exists {
		return
	}
	rawMap, ok := raw.(map[string]interface{})
	if !ok {
		return
	}
	info.RuntimeHeadersOverride = sanitizeHeaderOverrideMap(rawMap)
	info.UseRuntimeHeadersOverride = true
}

// SyncRuntimeHeaderOverrideFromContext 导出版本，供 service 层调用
func SyncRuntimeHeaderOverrideFromContext(info *RelayInfo, context map[string]interface{}) {
	syncRuntimeHeaderOverrideFromContext(info, context)
}

func moveValue(jsonStr, fromPath, toPath string) (string, error) {
	sourceValue := gjson.Get(jsonStr, fromPath)
	if !sourceValue.Exists() {
		return jsonStr, fmt.Errorf("source path does not exist: %s", fromPath)
	}
	result, err := sjson.Set(jsonStr, toPath, sourceValue.Value())
	if err != nil {
		return "", err
	}
	return sjson.Delete(result, fromPath)
}

func copyValue(jsonStr, fromPath, toPath string) (string, error) {
	sourceValue := gjson.Get(jsonStr, fromPath)
	if !sourceValue.Exists() {
		return jsonStr, fmt.Errorf("source path does not exist: %s", fromPath)
	}
	return sjson.Set(jsonStr, toPath, sourceValue.Value())
}

func isPathBasedOperation(mode string) bool {
	switch mode {
	case "delete", "set", "prepend", "append", "trim_prefix", "trim_suffix", "ensure_prefix", "ensure_suffix", "trim_space", "to_lower", "to_upper", "replace", "regex_replace", "prune_objects":
		return true
	default:
		return false
	}
}

func resolveOperationPaths(jsonStr, path string) ([]string, error) {
	if !strings.Contains(path, "*") {
		return []string{path}, nil
	}
	return expandWildcardPaths(jsonStr, path)
}

func expandWildcardPaths(jsonStr, path string) ([]string, error) {
	var root interface{}
	if err := common.Unmarshal([]byte(jsonStr), &root); err != nil {
		return nil, err
	}

	segments := strings.Split(path, ".")
	paths := collectWildcardPaths(root, segments, nil)
	return lo.Uniq(paths), nil
}

func collectWildcardPaths(node interface{}, segments []string, prefix []string) []string {
	if len(segments) == 0 {
		return []string{strings.Join(prefix, ".")}
	}

	segment := strings.TrimSpace(segments[0])
	if segment == "" {
		return nil
	}
	isLast := len(segments) == 1

	if segment == "*" {
		switch typed := node.(type) {
		case map[string]interface{}:
			keys := lo.Keys(typed)
			sort.Strings(keys)
			return lo.FlatMap(keys, func(key string, _ int) []string {
				return collectWildcardPaths(typed[key], segments[1:], append(prefix, key))
			})
		case []interface{}:
			return lo.FlatMap(lo.Range(len(typed)), func(index int, _ int) []string {
				return collectWildcardPaths(typed[index], segments[1:], append(prefix, strconv.Itoa(index)))
			})
		default:
			return nil
		}
	}

	switch typed := node.(type) {
	case map[string]interface{}:
		if isLast {
			return []string{strings.Join(append(prefix, segment), ".")}
		}
		next, exists := typed[segment]
		if !exists {
			return nil
		}
		return collectWildcardPaths(next, segments[1:], append(prefix, segment))
	case []interface{}:
		index, err := strconv.Atoi(segment)
		if err != nil || index < 0 || index >= len(typed) {
			return nil
		}
		if isLast {
			return []string{strings.Join(append(prefix, segment), ".")}
		}
		return collectWildcardPaths(typed[index], segments[1:], append(prefix, segment))
	default:
		return nil
	}
}

func deleteValue(jsonStr, path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return jsonStr, nil
	}
	return sjson.Delete(jsonStr, path)
}

func modifyValue(jsonStr, path string, value interface{}, keepOrigin, isPrepend bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	switch {
	case current.IsArray():
		return modifyArray(jsonStr, path, value, isPrepend)
	case current.Type == gjson.String:
		return modifyString(jsonStr, path, value, isPrepend)
	case current.Type == gjson.JSON:
		return mergeObjects(jsonStr, path, value, keepOrigin)
	}
	return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
}

func modifyArray(jsonStr, path string, value interface{}, isPrepend bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	var newArray []interface{}
	// 添加新值
	addValue := func() {
		if arr, ok := value.([]interface{}); ok {
			newArray = append(newArray, arr...)
		} else {
			newArray = append(newArray, value)
		}
	}
	// 添加原值
	addOriginal := func() {
		current.ForEach(func(_, val gjson.Result) bool {
			newArray = append(newArray, val.Value())
			return true
		})
	}
	if isPrepend {
		addValue()
		addOriginal()
	} else {
		addOriginal()
		addValue()
	}
	return sjson.Set(jsonStr, path, newArray)
}

func modifyString(jsonStr, path string, value interface{}, isPrepend bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	valueStr := fmt.Sprintf("%v", value)
	var newStr string
	if isPrepend {
		newStr = valueStr + current.String()
	} else {
		newStr = current.String() + valueStr
	}
	return sjson.Set(jsonStr, path, newStr)
}

func trimStringValue(jsonStr, path string, value interface{}, isPrefix bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}

	if value == nil {
		return jsonStr, fmt.Errorf("trim value is required")
	}
	valueStr := fmt.Sprintf("%v", value)

	var newStr string
	if isPrefix {
		newStr = strings.TrimPrefix(current.String(), valueStr)
	} else {
		newStr = strings.TrimSuffix(current.String(), valueStr)
	}
	return sjson.Set(jsonStr, path, newStr)
}

func ensureStringAffix(jsonStr, path string, value interface{}, isPrefix bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}

	if value == nil {
		return jsonStr, fmt.Errorf("ensure value is required")
	}
	valueStr := fmt.Sprintf("%v", value)
	if valueStr == "" {
		return jsonStr, fmt.Errorf("ensure value is required")
	}

	currentStr := current.String()
	if isPrefix {
		if strings.HasPrefix(currentStr, valueStr) {
			return jsonStr, nil
		}
		return sjson.Set(jsonStr, path, valueStr+currentStr)
	}

	if strings.HasSuffix(currentStr, valueStr) {
		return jsonStr, nil
	}
	return sjson.Set(jsonStr, path, currentStr+valueStr)
}

func transformStringValue(jsonStr, path string, transform func(string) string) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	return sjson.Set(jsonStr, path, transform(current.String()))
}

func replaceStringValue(jsonStr, path, from, to string) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	if from == "" {
		return jsonStr, fmt.Errorf("replace from is required")
	}
	return sjson.Set(jsonStr, path, strings.ReplaceAll(current.String(), from, to))
}

func regexReplaceStringValue(jsonStr, path, pattern, replacement string) (string, error) {
	current := gjson.Get(jsonStr, path)
	if current.Type != gjson.String {
		return jsonStr, fmt.Errorf("operation not supported for type: %v", current.Type)
	}
	if pattern == "" {
		return jsonStr, fmt.Errorf("regex pattern is required")
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return jsonStr, err
	}
	return sjson.Set(jsonStr, path, re.ReplaceAllString(current.String(), replacement))
}

type pruneObjectsOptions struct {
	conditions []ConditionOperation
	logic      string
	recursive  bool
}

func pruneObjects(jsonStr, path, contextJSON string, value interface{}) (string, error) {
	options, err := parsePruneObjectsOptions(value)
	if err != nil {
		return "", err
	}

	if path == "" {
		var root interface{}
		if err := common.Unmarshal([]byte(jsonStr), &root); err != nil {
			return "", err
		}
		cleaned, _, err := pruneObjectsNode(root, options, contextJSON, true)
		if err != nil {
			return "", err
		}
		cleanedBytes, err := common.Marshal(cleaned)
		if err != nil {
			return "", err
		}
		return string(cleanedBytes), nil
	}

	target := gjson.Get(jsonStr, path)
	if !target.Exists() {
		return jsonStr, nil
	}

	var targetNode interface{}
	if target.Type == gjson.JSON {
		if err := common.Unmarshal([]byte(target.Raw), &targetNode); err != nil {
			return "", err
		}
	} else {
		targetNode = target.Value()
	}

	cleaned, _, err := pruneObjectsNode(targetNode, options, contextJSON, true)
	if err != nil {
		return "", err
	}
	cleanedBytes, err := common.Marshal(cleaned)
	if err != nil {
		return "", err
	}
	return sjson.SetRaw(jsonStr, path, string(cleanedBytes))
}

func parsePruneObjectsOptions(value interface{}) (pruneObjectsOptions, error) {
	opts := pruneObjectsOptions{
		logic:     "AND",
		recursive: true,
	}

	switch raw := value.(type) {
	case nil:
		return opts, fmt.Errorf("prune_objects value is required")
	case string:
		v := strings.TrimSpace(raw)
		if v == "" {
			return opts, fmt.Errorf("prune_objects value is required")
		}
		opts.conditions = []ConditionOperation{
			{
				Path:  "type",
				Mode:  "full",
				Value: v,
			},
		}
	case map[string]interface{}:
		if logic, ok := raw["logic"].(string); ok && strings.TrimSpace(logic) != "" {
			opts.logic = logic
		}
		if recursive, ok := raw["recursive"].(bool); ok {
			opts.recursive = recursive
		}

		if condRaw, exists := raw["conditions"]; exists {
			conditions, err := parseConditionOperations(condRaw)
			if err != nil {
				return opts, err
			}
			opts.conditions = append(opts.conditions, conditions...)
		}

		if whereRaw, exists := raw["where"]; exists {
			whereMap, ok := whereRaw.(map[string]interface{})
			if !ok {
				return opts, fmt.Errorf("prune_objects where must be object")
			}
			for key, val := range whereMap {
				key = strings.TrimSpace(key)
				if key == "" {
					continue
				}
				opts.conditions = append(opts.conditions, ConditionOperation{
					Path:  key,
					Mode:  "full",
					Value: val,
				})
			}
		}

		if matchType, exists := raw["type"]; exists {
			opts.conditions = append(opts.conditions, ConditionOperation{
				Path:  "type",
				Mode:  "full",
				Value: matchType,
			})
		}
	default:
		return opts, fmt.Errorf("prune_objects value must be string or object")
	}

	if len(opts.conditions) == 0 {
		return opts, fmt.Errorf("prune_objects conditions are required")
	}
	return opts, nil
}

func parseConditionOperations(raw interface{}) ([]ConditionOperation, error) {
	switch typed := raw.(type) {
	case map[string]interface{}:
		entries := lo.Entries(typed)
		conditions := lo.FilterMap(entries, func(item lo.Entry[string, interface{}], _ int) (ConditionOperation, bool) {
			path := strings.TrimSpace(item.Key)
			if path == "" {
				return ConditionOperation{}, false
			}
			return ConditionOperation{
				Path:  path,
				Mode:  "full",
				Value: item.Value,
			}, true
		})
		if len(conditions) == 0 {
			return nil, fmt.Errorf("conditions object must contain at least one key")
		}
		return conditions, nil
	case []interface{}:
		items := typed
		result := make([]ConditionOperation, 0, len(items))
		for _, item := range items {
			itemMap, ok := item.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("condition must be object")
			}
			path, _ := itemMap["path"].(string)
			mode, _ := itemMap["mode"].(string)
			if strings.TrimSpace(path) == "" || strings.TrimSpace(mode) == "" {
				return nil, fmt.Errorf("condition path/mode is required")
			}
			condition := ConditionOperation{
				Path: path,
				Mode: mode,
			}
			if value, exists := itemMap["value"]; exists {
				condition.Value = value
			}
			if invert, ok := itemMap["invert"].(bool); ok {
				condition.Invert = invert
			}
			if passMissingKey, ok := itemMap["pass_missing_key"].(bool); ok {
				condition.PassMissingKey = passMissingKey
			}
			result = append(result, condition)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("conditions must be an array or object")
	}
}

func pruneObjectsNode(node interface{}, options pruneObjectsOptions, contextJSON string, isRoot bool) (interface{}, bool, error) {
	switch value := node.(type) {
	case []interface{}:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			next, drop, err := pruneObjectsNode(item, options, contextJSON, false)
			if err != nil {
				return nil, false, err
			}
			if drop {
				continue
			}
			result = append(result, next)
		}
		return result, false, nil
	case map[string]interface{}:
		shouldDrop, err := shouldPruneObject(value, options, contextJSON)
		if err != nil {
			return nil, false, err
		}
		if shouldDrop && !isRoot {
			return nil, true, nil
		}
		if !options.recursive {
			return value, false, nil
		}
		for key, child := range value {
			next, drop, err := pruneObjectsNode(child, options, contextJSON, false)
			if err != nil {
				return nil, false, err
			}
			if drop {
				delete(value, key)
				continue
			}
			value[key] = next
		}
		return value, false, nil
	default:
		return node, false, nil
	}
}

func shouldPruneObject(node map[string]interface{}, options pruneObjectsOptions, contextJSON string) (bool, error) {
	nodeBytes, err := common.Marshal(node)
	if err != nil {
		return false, err
	}
	return checkConditions(string(nodeBytes), contextJSON, options.conditions, options.logic)
}

func mergeObjects(jsonStr, path string, value interface{}, keepOrigin bool) (string, error) {
	current := gjson.Get(jsonStr, path)
	var currentMap, newMap map[string]interface{}

	// 解析当前值
	if err := common.Unmarshal([]byte(current.Raw), &currentMap); err != nil {
		return "", err
	}
	// 解析新值
	switch v := value.(type) {
	case map[string]interface{}:
		newMap = v
	default:
		jsonBytes, _ := common.Marshal(v)
		if err := common.Unmarshal(jsonBytes, &newMap); err != nil {
			return "", err
		}
	}
	// 合并
	result := make(map[string]interface{})
	for k, v := range currentMap {
		result[k] = v
	}
	for k, v := range newMap {
		if !keepOrigin || result[k] == nil {
			result[k] = v
		}
	}
	return sjson.Set(jsonStr, path, result)
}

// BuildParamOverrideContext 构建参数覆盖操作可用的上下文信息。
// 内置以下上下文字段：
//   - model / upstream_model: 渠道映射后的上游模型名
//   - original_model: 请求最初指定的模型名
//   - request_path: 请求路径
//   - request_headers: 请求头（键已小写化）
//   - header_override: 当前的头覆盖映射
//   - retry_index: 重试索引
//   - is_retry: 是否为重试请求
//   - retry: 重试信息对象（包含 index 和 is_retry）
//   - last_error: 上一次错误的详细信息（仅重试时存在）
//   - is_channel_test: 是否为渠道测试请求
//
// 参数：
//   - info: 中继信息
//
// 返回值：
//   - map[string]interface{}: 上下文映射
func BuildParamOverrideContext(info *RelayInfo) map[string]interface{} {
	if info == nil {
		return nil
	}

	ctx := make(map[string]interface{})
	if info.ChannelMeta != nil && info.ChannelMeta.UpstreamModelName != "" {
		ctx["model"] = info.ChannelMeta.UpstreamModelName
		ctx["upstream_model"] = info.ChannelMeta.UpstreamModelName
	}
	if info.OriginModelName != "" {
		ctx["original_model"] = info.OriginModelName
		if _, exists := ctx["model"]; !exists {
			ctx["model"] = info.OriginModelName
		}
	}

	if info.RequestURLPath != "" {
		requestPath := info.RequestURLPath
		if requestPath != "" {
			ctx["request_path"] = requestPath
		}
	}

	ctx[paramOverrideContextRequestHeaders] = buildRequestHeadersContext(info.RequestHeaders)

	headerOverrideSource := GetEffectiveHeaderOverride(info)
	ctx[paramOverrideContextHeaderOverride] = sanitizeHeaderOverrideMap(headerOverrideSource)

	ctx["retry_index"] = info.RetryIndex
	ctx["is_retry"] = info.RetryIndex > 0
	ctx["retry"] = map[string]interface{}{
		"index":    info.RetryIndex,
		"is_retry": info.RetryIndex > 0,
	}

	if info.LastError != nil {
		code := string(info.LastError.GetErrorCode())
		errorType := string(info.LastError.GetErrorType())
		lastError := map[string]interface{}{
			"status_code": info.LastError.StatusCode,
			"message":     info.LastError.Error(),
			"code":        code,
			"error_code":  code,
			"type":        errorType,
			"error_type":  errorType,
			"skip_retry":  types.IsSkipRetryError(info.LastError),
		}
		ctx["last_error"] = lastError
		ctx["last_error_status_code"] = info.LastError.StatusCode
		ctx["last_error_message"] = info.LastError.Error()
		ctx["last_error_code"] = code
		ctx["last_error_type"] = errorType
	}

	ctx["is_channel_test"] = info.IsChannelTest
	return ctx
}
