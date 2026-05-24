// Package types - rw_map.go
// 该文件定义了线程安全的读写映射（RWMap）
//
// 主要类型：
// - RWMap：基于读写锁的并发安全映射
//
// 核心功能：
// - 支持泛型键值类型
// - 读操作使用读锁（RLock），写操作使用写锁（Lock）
// - 适用于读多写少的并发场景
package types

import (
	"sync"

	"github.com/c1cada/NexusTok/common"
)

// RWMap 线程安全的读写映射
// 基于 sync.RWMutex 实现并发安全的键值存储
// 适用于读多写少的并发场景
//
// 类型参数：
//   - K: 键类型，必须是可比较类型
//   - V: 值类型
type RWMap[K comparable, V any] struct {
	data  map[K]V       // 底层数据存储
	mutex sync.RWMutex  // 读写锁，保护并发访问
}

// UnmarshalJSON 从 JSON 字节数据反序列化到 RWMap
// 实现 json.Unmarshaler 接口，使用写锁保护数据一致性
//
// 参数：
//   - b: JSON 字节数据
//
// 返回值：
//   - error: 反序列化错误
func (m *RWMap[K, V]) UnmarshalJSON(b []byte) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data = make(map[K]V)
	return common.Unmarshal(b, &m.data)
}

// MarshalJSON 将 RWMap 序列化为 JSON 字节数据
// 实现 json.Marshaler 接口，使用读锁保护数据一致性
//
// 返回值：
//   - []byte: JSON 字节数据
//   - error: 序列化错误
func (m *RWMap[K, V]) MarshalJSON() ([]byte, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return common.Marshal(m.data)
}

// NewRWMap 创建并返回一个新的空 RWMap
//
// 返回值：
//   - *RWMap[K, V]: 新的 RWMap 实例
func NewRWMap[K comparable, V any]() *RWMap[K, V] {
	return &RWMap[K, V]{
		data: make(map[K]V),
	}
}

// Get 获取指定键的值
// 使用读锁保护并发访问
//
// 参数：
//   - key: 要查找的键
//
// 返回值：
//   - V: 键对应的值
//   - bool: 键是否存在，存在返回 true
func (m *RWMap[K, V]) Get(key K) (V, bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	value, exists := m.data[key]
	return value, exists
}

// Set 设置键值对
// 使用写锁保护并发访问，如果键已存在则覆盖
//
// 参数：
//   - key: 要设置的键
//   - value: 要设置的值
func (m *RWMap[K, V]) Set(key K, value V) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data[key] = value
}

// AddAll 批量添加键值对
// 将另一个 map 中的所有键值对添加到 RWMap 中
// 使用写锁保护并发访问
//
// 参数：
//   - other: 要合并的 map
func (m *RWMap[K, V]) AddAll(other map[K]V) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	for k, v := range other {
		m.data[k] = v
	}
}

// Clear 清空 RWMap 中的所有键值对
// 使用写锁保护并发访问
func (m *RWMap[K, V]) Clear() {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data = make(map[K]V)
}

// ReadAll 返回 RWMap 的副本
// 使用读锁保护并发访问，返回的是数据的深拷贝
//
// 返回值：
//   - map[K]V: 包含所有键值对的新 map
func (m *RWMap[K, V]) ReadAll() map[K]V {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	copiedMap := make(map[K]V)
	for k, v := range m.data {
		copiedMap[k] = v
	}
	return copiedMap
}

// Len 返回 RWMap 中的键值对数量
// 使用读锁保护并发访问
//
// 返回值：
//   - int: 键值对数量
func (m *RWMap[K, V]) Len() int {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return len(m.data)
}

// LoadFromJsonString 从 JSON 字符串加载数据到 RWMap
// 使用写锁保护并发访问，会先清空现有数据再加载
//
// 参数：
//   - m: 目标 RWMap
//   - jsonStr: JSON 格式的字符串
//
// 返回值：
//   - error: 反序列化错误
func LoadFromJsonString[K comparable, V any](m *RWMap[K, V], jsonStr string) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data = make(map[K]V)
	return common.Unmarshal([]byte(jsonStr), &m.data)
}

// LoadFromJsonStringWithCallback 从 JSON 字符串加载数据并在成功后执行回调
// 使用写锁保护并发访问，加载成功后调用 onSuccess 回调函数
//
// 参数：
//   - m: 目标 RWMap
//   - jsonStr: JSON 格式的字符串
//   - onSuccess: 加载成功后的回调函数，可为 nil
//
// 返回值：
//   - error: 反序列化错误
func LoadFromJsonStringWithCallback[K comparable, V any](m *RWMap[K, V], jsonStr string, onSuccess func()) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.data = make(map[K]V)
	err := common.Unmarshal([]byte(jsonStr), &m.data)
	if err == nil && onSuccess != nil {
		onSuccess()
	}
	return err
}

// MarshalJSONString 将 RWMap 序列化为 JSON 字符串
// 如果序列化失败则返回空 JSON 对象 "{}"
//
// 返回值：
//   - string: JSON 格式的字符串
func (m *RWMap[K, V]) MarshalJSONString() string {
	bytes, err := m.MarshalJSON()
	if err != nil {
		return "{}"
	}
	return string(bytes)
}
