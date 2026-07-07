// Package common - json.go
// 该文件提供了 JSON 序列化和反序列化的统一接口
//
// 所有 JSON 操作都应通过这些函数进行，而不是直接使用 encoding/json
// 这样做的目的是：
// - 保持一致性：统一的 JSON 处理入口
// - 未来可扩展：可以方便地替换为更快的 JSON 库（如 jsoniter）
// - 便于调试：可以在这些函数中添加日志或监控
//
// 注意：根据项目规范，业务代码不得直接导入 encoding/json
// 只能使用本文件中定义的函数
package common

import (
	"bytes"
	"encoding/json"
	"io"
)

// Unmarshal 反序列化 JSON 字节数据
//
// 参数：
//   - data: JSON 字节数据
//   - v: 目标对象指针
//
// 返回值：
//   - error: 反序列化错误
func Unmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// UnmarshalJsonStr 反序列化 JSON 字符串
//
// 使用 StringToByteSlice 避免不必要的内存分配
//
// 参数：
//   - data: JSON 字符串
//   - v: 目标对象指针
//
// 返回值：
//   - error: 反序列化错误
func UnmarshalJsonStr(data string, v any) error {
	return json.Unmarshal(StringToByteSlice(data), v)
}

// DecodeJson 从 Reader 流式解码 JSON
//
// 适用于从 HTTP 请求体、文件等流式数据源解码 JSON
//
// 参数：
//   - reader: 数据源 Reader
//   - v: 目标对象指针
//
// 返回值：
//   - error: 解码错误
func DecodeJson(reader io.Reader, v any) error {
	return json.NewDecoder(reader).Decode(v)
}

// Marshal 序列化对象为 JSON 字节数据
//
// 参数：
//   - v: 要序列化的对象
//
// 返回值：
//   - []byte: JSON 字节数据
//   - error: 序列化错误
func Marshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

// ValidJSON 判断字节内容是否为合法 JSON。
//
// 业务代码需要校验 JSON 字符串时应调用该封装，而不是直接调用
// encoding/json.Valid；这样所有 JSON 行为都能继续收敛在 common 包内。
func ValidJSON(data []byte) bool {
	return json.Valid(data)
}

// GetJsonType 获取 JSON 数据的类型
//
// 通过检查第一个非空白字符判断 JSON 类型
//
// 参数：
//   - data: JSON 原始数据
//
// 返回值：
//   - string: JSON 类型（"object"、"array"、"string"、"boolean"、"null"、"number"、"unknown"）
func GetJsonType(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "unknown"
	}
	firstChar := trimmed[0]
	switch firstChar {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	default:
		return "number"
	}
}

// JsonRawMessageToString 将 JSON RawMessage 转换为字符串
//
// 如果是 JSON 字符串（带引号），返回解码后的值
// 如果是其他 JSON 类型，返回原始文本
//
// 参数：
//   - data: JSON RawMessage
//
// 返回值：
//   - string: 转换后的字符串
func JsonRawMessageToString(data json.RawMessage) string {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	if trimmed[0] != '"' {
		return string(trimmed)
	}
	var value string
	if err := Unmarshal(trimmed, &value); err != nil {
		return string(trimmed)
	}
	return value
}
