// Package cachex - codec.go
// 该文件定义了缓存值的编解码器接口和实现
//
// 主要接口：
// - ValueCodec：值编解码器接口（Encode/Decode）
//
// 实现：
// - IntCodec：整数编解码器
// - StringCodec：字符串编解码器
// - JSONCodec：JSON 编解码器
//
// 用途：
// - 将缓存值序列化为字符串存储到 Redis
// - 从 Redis 字符串反序列化为 Go 类型
package cachex

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ValueCodec 定义缓存值的编解码器接口
// 泛型参数 V 表示缓存值的 Go 类型
// 用于将 Go 类型序列化为 Redis 字符串存储，以及从 Redis 字符串反序列化回 Go 类型
type ValueCodec[V any] interface {
	// Encode 将 Go 值序列化为字符串（用于存储到 Redis）
	Encode(v V) (string, error)
	// Decode 将字符串反序列化为 Go 值（用于从 Redis 读取）
	Decode(s string) (V, error)
}

// IntCodec 整数编解码器
// 将 int 类型与字符串相互转换
type IntCodec struct{}

// Encode 将整数转换为十进制字符串
func (c IntCodec) Encode(v int) (string, error) {
	return strconv.Itoa(v), nil
}

// Decode 将十进制字符串解析为整数
// 空字符串会返回错误
func (c IntCodec) Decode(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty int value")
	}
	return strconv.Atoi(s)
}

// StringCodec 字符串编解码器
// 直接透传，不做任何转换
type StringCodec struct{}

// Encode 直接返回原字符串
func (c StringCodec) Encode(v string) (string, error) { return v, nil }

// Decode 直接返回原字符串
func (c StringCodec) Decode(s string) (string, error) { return s, nil }

// JSONCodec JSON 编解码器
// 泛型参数 V 表示要序列化的 Go 结构体类型
// 使用 encoding/json 进行 JSON 序列化/反序列化
type JSONCodec[V any] struct{}

// Encode 将 Go 值序列化为 JSON 字符串
func (c JSONCodec[V]) Encode(v V) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// Decode 将 JSON 字符串反序列化为 Go 值
// 空字符串会返回错误
func (c JSONCodec[V]) Decode(s string) (V, error) {
	var v V
	if strings.TrimSpace(s) == "" {
		return v, fmt.Errorf("empty json value")
	}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return v, err
	}
	return v, nil
}
