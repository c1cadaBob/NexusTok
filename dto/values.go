// Package dto - values.go
// 该文件定义了灵活类型的 JSON 序列化/反序列化支持
//
// 主要类型：
// - StringValue：字符串值（支持从 JSON 字符串和数字反序列化）
// - IntValue：整数值（支持从 JSON 数字和字符串反序列化）
// - BoolValue：布尔值（支持从 JSON 布尔值和字符串反序列化）
//
// 用途：处理上游 API 返回的类型不一致的数据
// 例如：某些 API 返回数字 123，而另一些返回字符串 "123"
package dto

import (
	"encoding/json"
	"strconv"
)

// StringValue 字符串值类型
// 支持从 JSON 字符串和数字类型反序列化
// 序列化时始终输出为 JSON 字符串
type StringValue string

// UnmarshalJSON 自定义 JSON 反序列化
// 优先尝试解析为字符串，失败则尝试解析为数字并转换为字符串
func (s *StringValue) UnmarshalJSON(data []byte) error {
	var str string
	if err := json.Unmarshal(data, &str); err == nil {
		*s = StringValue(str)
		return nil
	}

	var raw json.Number
	if err := json.Unmarshal(data, &raw); err == nil {
		*s = StringValue(raw.String())
		return nil
	}

	return json.Unmarshal(data, &str)
}

// MarshalJSON 自定义 JSON 序列化（输出为字符串）
func (s StringValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(string(s))
}

// IntValue 整数值类型
// 支持从 JSON 数字和字符串类型反序列化
// 序列化时始终输出为 JSON 数字
type IntValue int

// UnmarshalJSON 自定义 JSON 反序列化
// 优先尝试解析为数字，失败则尝试解析为字符串并转换为数字
func (i *IntValue) UnmarshalJSON(b []byte) error {
	var n int
	if err := json.Unmarshal(b, &n); err == nil {
		*i = IntValue(n)
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return err
	}
	*i = IntValue(v)
	return nil
}

// MarshalJSON 自定义 JSON 序列化（输出为数字）
func (i IntValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(int(i))
}

// BoolValue 布尔值类型
// 支持从 JSON 布尔值和字符串（"true"/"false"）反序列化
// 序列化时始终输出为 JSON 布尔值
type BoolValue bool

// UnmarshalJSON 自定义 JSON 反序列化
// 优先尝试解析为布尔值，失败则尝试解析为字符串并转换
func (b *BoolValue) UnmarshalJSON(data []byte) error {
	var boolean bool
	if err := json.Unmarshal(data, &boolean); err == nil {
		*b = BoolValue(boolean)
		return nil
	}
	var str string
	if err := json.Unmarshal(data, &str); err != nil {
		return err
	}
	if str == "true" {
		*b = BoolValue(true)
	} else if str == "false" {
		*b = BoolValue(false)
	} else {
		return json.Unmarshal(data, &boolean)
	}
	return nil
}
// MarshalJSON 自定义 JSON 序列化（输出为布尔值）
func (b BoolValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(b))
}
