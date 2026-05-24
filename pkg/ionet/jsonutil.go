// Package ionet - jsonutil.go
// 该文件提供了 JSON 解析的辅助函数
//
// 核心功能：
// - decodeWithFlexibleTimes：灵活的时间戳解析
// - 容忍缺少时区信息的时间戳字符串
// - 自动将时间戳规范化为 RFC3339Nano 格式
package ionet

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/samber/lo"
)

// decodeWithFlexibleTimes 反序列化 API 响应，同时容忍缺少时区信息的时间戳字符串
// 先将 JSON 解析为中间格式，规范化所有时间字符串为 RFC3339Nano，再重新序列化为目标结构体
// 解决某些 API 返回的时间戳缺少时区信息（如 "2024-01-01T00:00:00"）导致解析失败的问题
func decodeWithFlexibleTimes(data []byte, target interface{}) error {
	var intermediate interface{}
	if err := json.Unmarshal(data, &intermediate); err != nil {
		return err
	}

	normalized := normalizeTimeValues(intermediate)
	reencoded, err := json.Marshal(normalized)
	if err != nil {
		return err
	}

	return json.Unmarshal(reencoded, target)
}

// decodeData 从包含 "data" 字段的 API 响应中提取数据
// 泛型参数 T 表示目标数据类型
// API 响应格式：{"data": {...}}，提取 data 字段的内容到 target
func decodeData[T any](data []byte, target *T) error {
	var wrapper struct {
		Data T `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return err
	}
	*target = wrapper.Data
	return nil
}

// decodeDataWithFlexibleTimes 结合 data 字段提取和灵活时间解析
// 从 {"data": {...}} 格式的响应中提取数据，同时容忍时间戳格式问题
func decodeDataWithFlexibleTimes[T any](data []byte, target *T) error {
	var wrapper struct {
		Data T `json:"data"`
	}
	if err := decodeWithFlexibleTimes(data, &wrapper); err != nil {
		return err
	}
	*target = wrapper.Data
	return nil
}

// normalizeTimeValues 递归遍历 JSON 值树，规范化所有时间字符串
// 处理 map（JSON 对象）、slice（JSON 数组）和 string（可能的时间值）三种类型
func normalizeTimeValues(value interface{}) interface{} {
	switch v := value.(type) {
	case map[string]interface{}:
		return lo.MapValues(v, func(val interface{}, _ string) interface{} {
			return normalizeTimeValues(val)
		})
	case []interface{}:
		return lo.Map(v, func(item interface{}, _ int) interface{} {
			return normalizeTimeValues(item)
		})
	case string:
		if normalized, changed := normalizeTimeString(v); changed {
			return normalized
		}
		return v
	default:
		return value
	}
}

// normalizeTimeString 尝试将时间字符串规范化为 RFC3339Nano 格式
// 支持以下格式：
//   - RFC3339Nano（已标准格式，直接返回）
//   - RFC3339（已标准格式，直接返回）
//   - 不带时区的 ISO8601 变体（自动补充 UTC 时区）
//
// 返回值：
//   - string：规范化后的时间字符串
//   - bool：是否发生了规范化（用于判断是否需要重新序列化）
func normalizeTimeString(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input, false
	}

	if _, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
		return trimmed, trimmed != input
	}
	if _, err := time.Parse(time.RFC3339, trimmed); err == nil {
		return trimmed, trimmed != input
	}

	layouts := []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05.999999",
		"2006-01-02T15:04:05",
	}

	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC().Format(time.RFC3339Nano), true
		}
	}

	return input, false
}
