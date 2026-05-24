// Package cachex - namespace.go
// 该文件定义了缓存命名空间（Namespace）类型
//
// 主要类型：
// - Namespace：缓存命名空间，用于隔离不同的缓存用途
//
// 用途：
// - 为不同的缓存用例提供键隔离（如 channel_affinity:v1）
// - 自动为缓存键添加命名空间前缀
package cachex

import "strings"

// Namespace 表示缓存命名空间，用于隔离不同的缓存用途
// 通过为缓存键添加命名空间前缀，避免不同业务场景的键冲突
// 例如 "channel_affinity:v1" 作为命名空间，实际存储键为 "channel_affinity:v1:mykey"
type Namespace string

// prefix 返回命名空间的前缀字符串
// 如果命名空间为空，返回空字符串
// 否则返回 "namespace:" 格式的前缀（自动去除尾部冒号并添加）
func (n Namespace) prefix() string {
	ns := strings.TrimSpace(string(n))
	ns = strings.TrimRight(ns, ":")
	if ns == "" {
		return ""
	}
	return ns + ":"
}

// FullKey 将原始键转换为带命名空间前缀的完整键
// 如果键已经包含此前缀，则不重复添加
// 例如：Namespace("channel_affinity:v1").FullKey("mykey") 返回 "channel_affinity:v1:mykey"
func (n Namespace) FullKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	p := n.prefix()
	if p == "" {
		return strings.TrimLeft(key, ":")
	}
	if strings.HasPrefix(key, p) {
		return key
	}
	return p + strings.TrimLeft(key, ":")
}

// MatchPattern 返回用于 Redis SCAN 命令的匹配模式
// 如果命名空间为空，返回 "*"（匹配所有键）
// 否则返回 "namespace:*" 格式的模式
func (n Namespace) MatchPattern() string {
	p := n.prefix()
	if p == "" {
		return "*"
	}
	return p + "*"
}
