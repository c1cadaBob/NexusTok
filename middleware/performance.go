// Package middleware - performance.go
// 该文件实现了系统性能监控中间件
//
// 功能说明：
// - 在请求处理前检查系统资源使用情况
// - 当 CPU、内存或磁盘使用率超过配置阈值时，拒绝新请求
// - 根据请求路径返回不同格式的错误响应：
//   - /v1/messages 路径返回 Claude 格式错误
//   - 其他路径返回 OpenAI 格式错误
//
// 配置来源：
// - 通过 common.GetPerformanceMonitorConfig() 获取性能监控配置
// - 配置项包括：是否启用、CPU 阈值、内存阈值、磁盘阈值
//
// 监控指标：
// - CPU 使用率（百分比）
// - 内存使用率（百分比）
// - 磁盘使用率（百分比）
package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
)

// SystemPerformanceCheck 系统性能检查中间件工厂函数
// 创建并返回一个 Gin 中间件，在请求处理前检查系统资源使用情况
//
// 错误响应格式：
// - /v1/messages 路径：Claude API 错误格式（ToClaudeError）
// - 其他路径：OpenAI API 错误格式（ToOpenAIError）
//
// 返回 HTTP 503 Service Unavailable 当系统资源超限时
func SystemPerformanceCheck() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 仅检查 Relay 接口 (/v1, /v1beta 等)
		// 这里简单判断路径前缀，可以根据实际路由调整
		path := c.Request.URL.Path
		if strings.HasPrefix(path, "/v1/messages") {
			if err := checkSystemPerformance(); err != nil {
				c.JSON(err.StatusCode, gin.H{
					"error": err.ToClaudeError(),
				})
				c.Abort()
				return
			}
		} else {
			if err := checkSystemPerformance(); err != nil {
				c.JSON(err.StatusCode, gin.H{
					"error": err.ToOpenAIError(),
				})
				c.Abort()
				return
			}
		}
		c.Next()
	}
}

// checkSystemPerformance 检查系统性能是否超过配置阈值
// 依次检查 CPU、内存、磁盘使用率，任一超限即返回错误
//
// 返回值：
//   - nil：系统资源正常，可以处理请求
//   - *types.NexusTokError：资源超限错误，包含 HTTP 503 状态码和错误信息
func checkSystemPerformance() *types.NexusTokError {
	config := common.GetPerformanceMonitorConfig()
	if !config.Enabled {
		return nil
	}

	status := common.GetSystemStatus()

	// 检查 CPU
	if config.CPUThreshold > 0 && int(status.CPUUsage) > config.CPUThreshold {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("system cpu overloaded (current: %.1f%%, threshold: %d%%)", status.CPUUsage, config.CPUThreshold),
			"system_cpu_overloaded", http.StatusServiceUnavailable)
	}

	// 检查内存
	if config.MemoryThreshold > 0 && int(status.MemoryUsage) > config.MemoryThreshold {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("system memory overloaded (current: %.1f%%, threshold: %d%%)", status.MemoryUsage, config.MemoryThreshold),
			"system_memory_overloaded", http.StatusServiceUnavailable)
	}

	// 检查磁盘
	if config.DiskThreshold > 0 && int(status.DiskUsage) > config.DiskThreshold {
		return types.NewErrorWithStatusCode(
			fmt.Errorf("system disk overloaded (current: %.1f%%, threshold: %d%%)", status.DiskUsage, config.DiskThreshold),
			"system_disk_overloaded", http.StatusServiceUnavailable)
	}

	return nil
}
