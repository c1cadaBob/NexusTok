// Package service 提供业务逻辑层的核心服务函数。
// 本文件 (http.go) 提供 HTTP 响应处理的工具函数，
// 包括安全关闭响应体、判断上游响应头是否应复制到客户端、
// 以及将字节数据优雅地写入 Gin 响应流。
package service

import (
	"bytes"  // 字节缓冲区，用于构造响应 Body
	"fmt"    // 格式化输出
	"io"     // IO 操作接口
	"net/http" // HTTP 标准库，提供 Response 类型
	"strings"  // 字符串操作

	"github.com/c1cada/NexusTok/common" // 项目公共工具包
	"github.com/c1cada/NexusTok/logger"  // 日志记录包

	"github.com/gin-gonic/gin" // Gin Web 框架
)

// CloseResponseBodyGracefully 安全地关闭 HTTP 响应体。
// 该函数会在关闭失败时记录系统错误日志，避免因未关闭 Body 导致资源泄漏。
// 参数:
//   - httpResponse: 需要关闭 Body 的 HTTP 响应对象，可为 nil
func CloseResponseBodyGracefully(httpResponse *http.Response) {
	// 防御性检查：响应对象或 Body 为 nil 时直接返回
	if httpResponse == nil || httpResponse.Body == nil {
		return
	}
	err := httpResponse.Body.Close()
	if err != nil {
		common.SysError("failed to close response body: " + err.Error())
	}
}

// ShouldCopyUpstreamHeader 判断上游响应头是否应复制到客户端响应。
// 以下两种情况会被过滤（返回 false）：
// 1. Content-Length 头 —— 由系统单独管理，不应直接透传
// 2. X-Oneapi-Request-Id 头 —— 需保留本地实例的请求 ID，上游的 ID 会被
//    捕获到 Gin context 中用于后续日志记录，但不会透传给客户端
// 参数:
//   - c: Gin 上下文，可为 nil
//   - k: 响应头名称
//   - v: 响应头值列表
// 返回值:
//   - bool: true 表示应复制该头，false 表示应跳过
func ShouldCopyUpstreamHeader(c *gin.Context, k string, v []string) bool {
	// Content-Length 不复制，由系统自行计算并设置
	if strings.EqualFold(k, "Content-Length") {
		return false
	}
	// 上游请求 ID 不复制到客户端，而是存入上下文供日志使用
	if strings.EqualFold(k, common.RequestIdKey) {
		if c != nil && len(v) > 0 {
			c.Set(common.UpstreamRequestIdKey, v[0])
		}
		return false
	}
	return true
}

// IOCopyBytesGracefully 将已解析的字节数据优雅地写入 Gin 响应流。
// 该函数的处理流程：
// 1. 复制上游响应头到客户端（过滤不应透传的头）
// 2. 手动设置 Content-Length（确保客户端能正确接收完整数据）
// 3. 写入 HTTP 状态码
// 4. 将数据拷贝到响应 Writer 并 Flush
//
// 设计说明：不在解析响应 Body 之前设置响应头，因为解析可能失败，
// 此时需要发送错误响应。如果头已经发送，客户端会收到矛盾的响应。
//
// 参数:
//   - c: Gin 上下文
//   - src: 上游 HTTP 响应，可为 nil（nil 时使用 200 状态码）
//   - data: 需要写入客户端的响应字节数据
func IOCopyBytesGracefully(c *gin.Context, src *http.Response, data []byte) {
	if c.Writer == nil {
		return
	}

	// 将字节数据包装为 io.ReadCloser
	body := io.NopCloser(bytes.NewBuffer(data))

	// 复制上游响应头到客户端（过滤 Content-Length 和请求 ID）
	// 注意：不在解析 Body 之前设置头，因为解析可能失败，
	// 此时需要发送错误响应。如果头已发送，HTTP 客户端会困惑。
	// 例如 Postman 会报告错误且无法正常检查响应。
	if src != nil {
		for k, v := range src.Header {
			if !ShouldCopyUpstreamHeader(c, k, v) {
				continue
			}
			c.Writer.Header().Set(k, v[0])
		}
	}

	// 在调用 WriteHeader 之前手动设置 Content-Length
	c.Writer.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))

	// 写入 HTTP 状态码（此调用会发送响应头）
	if src != nil {
		c.Writer.WriteHeader(src.StatusCode)
	} else {
		c.Writer.WriteHeader(http.StatusOK)
	}

	// 将响应数据拷贝到客户端
	_, err := io.Copy(c.Writer, body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("failed to copy response body: %s", err.Error()))
	}
	// 确保所有缓冲数据发送到客户端
	c.Writer.Flush()
}
