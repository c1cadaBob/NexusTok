// Package types - set.go
// 该文件定义了泛型集合（Set）数据结构
//
// 主要类型：
// - Set：基于 map 的泛型集合
//
// 核心功能：
// - 支持任意可比较类型的元素
// - 添加、删除、检查元素是否存在
// - 获取集合大小和所有元素
package types

// Set 泛型集合数据结构
// 基于 map 实现的集合，支持任意可比较类型的元素
// 不保证元素的顺序
//
// 类型参数：
//   - T: 元素类型，必须是可比较类型
type Set[T comparable] struct {
	items map[T]struct{} // 底层存储，使用空结构体节省内存
}

// NewSet 创建并返回一个新的空 Set
//
// 返回值：
//   - *Set[T]: 新的 Set 实例
func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		items: make(map[T]struct{}),
	}
}

// Add 向 Set 中添加一个元素
// 如果元素已存在，则操作为空
//
// 参数：
//   - item: 要添加的元素
func (s *Set[T]) Add(item T) {
	s.items[item] = struct{}{}
}

// Remove 从 Set 中移除一个元素
func (s *Set[T]) Remove(item T) {
	delete(s.items, item)
}

// Contains 检查 Set 是否包含某个元素
func (s *Set[T]) Contains(item T) bool {
	_, exists := s.items[item]
	return exists
}

// Len 返回 Set 中元素的数量
func (s *Set[T]) Len() int {
	return len(s.items)
}

// Items 返回 Set 中所有元素组成的切片
// 注意：由于 map 的无序性，返回的切片元素顺序是随机的
func (s *Set[T]) Items() []T {
	items := make([]T, 0, s.Len())
	for item := range s.items {
		items = append(items, item)
	}
	return items
}
