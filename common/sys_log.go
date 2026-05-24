// Package common - sys_log.go
// 该文件实现了系统日志输出功能
//
// 提供统一的日志输出接口，将日志写入 Gin 框架的默认输出流：
// - SysLog：普通系统日志（标准输出）
// - SysError：错误日志（标准错误输出）
// - FatalLog：致命错误日志（标准错误输出，输出后终止进程）
// - LogStartupSuccess：启动成功日志（显示版本、端口、网络地址）
//
// 并发安全：
// - 使用 LogWriterMu（读写锁）保护日志输出的并发安全
// - 日志文件轮转时需要获取写锁，日常读写只需读锁
package common

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// LogWriterMu 保护 gin.DefaultWriter/gin.DefaultErrorWriter 的并发访问
//
// 用途：
// - 日志文件轮转时：获取写锁（Lock），交换 writer 并关闭旧文件
// - 日常日志读写时：获取读锁（RLock），允许并发写入
//
// 这是导出变量，供外部模块（如日志轮转组件）使用
var LogWriterMu sync.RWMutex

// SysLog 输出普通系统日志到标准输出
//
// 日志格式：[SYS] 2006/01/02 - 15:04:05 | <消息>
//
// 参数：
//   - s: 日志消息内容
func SysLog(s string) {
	t := time.Now()
	LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultWriter, "[SYS] %v | %s \n", t.Format("2006/01/02 - 15:04:05"), s)
	LogWriterMu.RUnlock()
}

// SysError 输出系统错误日志到标准错误输出
//
// 日志格式：[SYS] 2006/01/02 - 15:04:05 | <消息>
//
// 参数：
//   - s: 错误消息内容
func SysError(s string) {
	t := time.Now()
	LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultErrorWriter, "[SYS] %v | %s \n", t.Format("2006/01/02 - 15:04:05"), s)
	LogWriterMu.RUnlock()
}

// FatalLog 输出致命错误日志并终止进程
//
// 日志格式：[FATAL] 2006/01/02 - 15:04:05 | <消息>
// 输出后调用 os.Exit(1) 终止进程，退出码为 1
//
// 参数：
//   - v: 要输出的任意值（会被 fmt.Sprintf 格式化）
func FatalLog(v ...any) {
	t := time.Now()
	LogWriterMu.RLock()
	_, _ = fmt.Fprintf(gin.DefaultErrorWriter, "[FATAL] %v | %v \n", t.Format("2006/01/02 - 15:04:05"), v)
	LogWriterMu.RUnlock()
	os.Exit(1)
}

// LogStartupSuccess 输出服务启动成功的日志
//
// 显示内容：
// - 系统名称和版本号（绿色高亮）
// - 启动耗时（毫秒）
// - 本地访问地址（非容器环境）
// - 网络访问地址（所有网络接口 IP）
//
// 参数：
//   - startTime: 服务启动的开始时间，用于计算启动耗时
//   - port: 服务监听的端口号
func LogStartupSuccess(startTime time.Time, port string) {
	// 计算启动耗时
	duration := time.Since(startTime)
	durationMs := duration.Milliseconds()

	// 获取网络接口 IP 地址列表
	networkIps := GetNetworkIps()

	LogWriterMu.RLock()
	defer LogWriterMu.RUnlock()

	// 输出系统名称、版本和启动耗时
	fmt.Fprintf(gin.DefaultWriter, "\n")
	fmt.Fprintf(gin.DefaultWriter, "  \033[32m%s %s\033[0m  ready in %d ms\n", SystemName, Version, durationMs)
	fmt.Fprintf(gin.DefaultWriter, "\n")

	// 非容器环境下显示本地访问地址
	if !IsRunningInContainer() {
		fmt.Fprintf(gin.DefaultWriter, "  ➜  \033[1mLocal:\033[0m   http://localhost:%s/\n", port)
	}

	// 显示所有网络接口的访问地址
	for _, ip := range networkIps {
		fmt.Fprintf(gin.DefaultWriter, "  ➜  \033[1mNetwork:\033[0m http://%s:%s/\n", ip, port)
	}

	fmt.Fprintf(gin.DefaultWriter, "\n")
}
