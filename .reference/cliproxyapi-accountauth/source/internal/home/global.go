// 包 home - global.go
// 该文件提供了全局 Home 客户端实例的管理功能。
package home

import "sync/atomic"

// currentClient 是全局 Home 客户端实例
var currentClient atomic.Value // *Client

// SetCurrent 设置运行时集成使用的活动 Home 客户端。
func SetCurrent(client *Client) {
	currentClient.Store(client)
}

// Current 返回活动的 Home 客户端实例（如果存在）。
func Current() *Client {
	if v := currentClient.Load(); v != nil {
		if client, ok := v.(*Client); ok {
			return client
		}
	}
	return nil
}

// ClearCurrent 移除活动的 Home 客户端。
func ClearCurrent() {
	currentClient.Store((*Client)(nil))
}
