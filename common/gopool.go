// Package common - gopool.go
// 该文件实现了中继请求的协程池管理
//
// 使用字节跳动的 gopool 库实现协程池
// 协程池用于并发处理中继请求，避免创建过多协程导致资源耗尽
//
// 协程池特性：
// - 最大协程数：math.MaxInt32（实际上不限制）
// - 内置 panic 处理：捕获协程 panic 并发送停止信号
// - 上下文传递：支持传递 Gin 上下文到协程
package common

import (
	"context"
	"fmt"
	"math"

	"github.com/bytedance/gopkg/util/gopool"
)

var relayGoPool gopool.Pool // 中继请求协程池

func init() {
	// 创建协程池，名称为 "gopool.RelayPool"，最大协程数为 math.MaxInt32
	relayGoPool = gopool.NewPool("gopool.RelayPool", math.MaxInt32, gopool.NewConfig())
	// 设置 panic 处理器
	// 当协程发生 panic 时：
	// 1. 从上下文中获取 stop_chan 并发送停止信号
	// 2. 记录错误日志
	relayGoPool.SetPanicHandler(func(ctx context.Context, i interface{}) {
		if stopChan, ok := ctx.Value("stop_chan").(chan bool); ok {
			SafeSendBool(stopChan, true)
		}
		SysError(fmt.Sprintf("panic in gopool.RelayPool: %v", i))
	})
}

// RelayCtxGo 在协程池中执行函数（带上下文）
//
// 使用协程池而非直接 go 启动协程，可以：
// - 控制并发数量
// - 统一处理 panic
// - 传递上下文信息
//
// 参数：
//   - ctx: 上下文（会传递给协程池的 panic 处理器）
//   - f: 要执行的函数
func RelayCtxGo(ctx context.Context, f func()) {
	relayGoPool.CtxGo(ctx, f)
}
