// home - global.go
// 本文件维护全局唯一的 Home 客户端实例，提供线程安全的设置、获取和清除操作。
// Home 客户端用于运行时组件与 Home 控制平面的通信。
package home

import "sync/atomic"

// currentClient 是全局活跃的 Home 客户端实例，使用原子值保证并发安全。
var currentClient atomic.Value // *Client

// SetCurrent 设置当前活跃的 Home 客户端实例。
// 运行时集成组件通过此函数注入客户端。
//
// 参数：
//   - client: 要设置为当前活跃的 Home 客户端实例
func SetCurrent(client *Client) {
	currentClient.Store(client)
}

// Current 返回当前活跃的 Home 客户端实例。
// 如果没有设置客户端则返回 nil。
//
// 返回值：
//   - *Client: 当前活跃的 Home 客户端实例，未设置时返回 nil
func Current() *Client {
	if v := currentClient.Load(); v != nil {
		if client, ok := v.(*Client); ok {
			return client
		}
	}
	return nil
}

// ClearCurrent 清除当前活跃的 Home 客户端实例。
// 将全局客户端引用设为 nil。
func ClearCurrent() {
	currentClient.Store((*Client)(nil))
}
