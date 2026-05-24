// Package middleware - stats.go
// 该文件实现了 HTTP 连接统计中间件
//
// 功能说明：
// - 使用原子操作实时统计当前活跃的 HTTP 连接数
// - 每个请求开始时连接数 +1，结束时 -1
// - 通过 GetStats 函数可查询当前活跃连接数
//
// 使用场景：
// - 系统监控：了解当前服务器负载情况
// - 健康检查：判断服务器是否过载
// - 性能分析：结合其他指标分析系统瓶颈
package middleware

import (
	"sync/atomic"

	"github.com/gin-gonic/gin"
)

// HTTPStats HTTP 连接统计数据结构
// 使用原子操作保证并发安全，无需加锁
type HTTPStats struct {
	// activeConnections 当前活跃的 HTTP 连接数
	// 使用 int64 类型以支持 atomic 操作
	activeConnections int64
}

// globalStats 全局 HTTP 统计实例
// 所有请求共享同一个统计实例，确保数据一致性
var globalStats = &HTTPStats{}

// StatsMiddleware HTTP 连接统计中间件工厂函数
// 创建并返回一个 Gin 中间件，用于统计当前活跃的 HTTP 连接数
//
// 工作原理：
// 1. 请求开始时，使用 atomic.AddInt64 原子递增连接数
// 2. 使用 defer 确保请求结束时原子递减连接数
// 3. 无论请求正常结束还是发生 panic，连接数都会正确减少
func StatsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 增加活跃连接数
		atomic.AddInt64(&globalStats.activeConnections, 1)

		// 确保在请求结束时减少连接数
		defer func() {
			atomic.AddInt64(&globalStats.activeConnections, -1)
		}()

		c.Next()
	}
}

// StatsInfo HTTP 统计信息响应结构体
// 用于 API 响应，以 JSON 格式返回统计数据
type StatsInfo struct {
	// ActiveConnections 当前活跃的 HTTP 连接数
	ActiveConnections int64 `json:"active_connections"`
}

// GetStats 获取当前 HTTP 统计信息
// 使用 atomic.LoadInt64 原子读取，保证并发安全
//
// 返回值：
//   - StatsInfo：包含当前活跃连接数的统计信息
func GetStats() StatsInfo {
	return StatsInfo{
		ActiveConnections: atomic.LoadInt64(&globalStats.activeConnections),
	}
}
