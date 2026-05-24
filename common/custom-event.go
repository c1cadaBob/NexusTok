// Copyright 2014 Manu Martinez-Almeida.  All rights reserved.
// Use of this source code is governed by a MIT style
// license that can be found in the LICENSE file.

// Package common - custom-event.go
// 该文件实现了 Server-Sent Events（SSE）的自定义事件渲染
//
// SSE 是一种服务器向客户端推送事件的技术
// 基于 HTTP 协议，使用 text/event-stream 内容类型
// 参考规范：W3C Working Draft 29 October 2009
// http://www.w3.org/TR/2009/WD-eventsource-20091029/
//
// SSE 数据格式：
// - 每条消息以 "data:" 前缀开头
// - 多行数据使用多个 "data:" 前缀
// - 消息之间以空行分隔（\n\n）
// - 支持的字段：event（事件类型）、id（事件 ID）、retry（重试间隔）、data（数据）
package common

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// stringWriter 组合 io.Writer 和 writeString 方法的接口
// 用于优化字符串写入性能（避免 []byte 转换）
type stringWriter interface {
	io.Writer
	writeString(string) (int, error)
}

// stringWrapper 将 io.Writer 包装为 stringWriter
// 当底层 Writer 不支持 writeString 方法时使用
type stringWrapper struct {
	io.Writer
}

// writeString 将字符串写入 Writer（通过 []byte 转换）
func (w stringWrapper) writeString(str string) (int, error) {
	return w.Writer.Write([]byte(str))
}

// checkWriter 检查 Writer 是否支持 stringWriter 接口
// 如果支持则直接返回，否则包装为 stringWrapper
func checkWriter(writer io.Writer) stringWriter {
	if w, ok := writer.(stringWriter); ok {
		return w
	} else {
		return stringWrapper{writer}
	}
}

// Server-Sent Events 相关常量
var contentType = []string{"text/event-stream"} // SSE 内容类型
var noCache = []string{"no-cache"}              // 禁用缓存

// fieldReplacer 字段值替换器
// 将换行符转义为 \\n 和 \\r，防止破坏 SSE 格式
var fieldReplacer = strings.NewReplacer(
	"\n", "\\n",
	"\r", "\\r")

// dataReplacer 数据值替换器
// 保留 \n 用于多行数据，转义 \r
var dataReplacer = strings.NewReplacer(
	"\n", "\n",
	"\r", "\\r")

// CustomEvent 自定义 SSE 事件结构体
//
// 用于渲染 Server-Sent Events 响应
// 支持以下字段：
// - Event: 事件类型（可选，客户端通过 addEventListener 监听）
// - Id: 事件 ID（可选，用于断线重连时的 Last-Event-ID）
// - Retry: 重试间隔（毫秒，可选）
// - Data: 事件数据（必需）
type CustomEvent struct {
	Event string      // 事件类型
	Id    string      // 事件 ID
	Retry uint        // 重试间隔（毫秒）
	Data  interface{} // 事件数据

	Mutex sync.Mutex // 互斥锁，保护并发写入
}

// encode 将事件编码为 SSE 格式并写入 Writer
//
// 参数：
//   - writer: 输出 Writer
//   - event: 要编码的事件
//
// 返回值：
//   - error: 编码错误
func encode(writer io.Writer, event CustomEvent) error {
	w := checkWriter(writer)
	return writeData(w, event.Data)
}

// writeData 将数据写入 SSE 格式
//
// SSE 数据格式：每行以 "data:" 前缀开头，数据后跟两个换行符
//
// 参数：
//   - w: stringWriter 接口
//   - data: 要写入的数据
//
// 返回值：
//   - error: 写入错误
func writeData(w stringWriter, data interface{}) error {
	dataReplacer.WriteString(w, fmt.Sprint(data))
	// 如果数据以 "data" 开头，添加双换行符作为消息分隔
	if strings.HasPrefix(data.(string), "data") {
		w.writeString("\n\n")
	}
	return nil
}

// Render 渲染 SSE 事件到 HTTP 响应
//
// 实现 gin.Render 接口
// 先设置 Content-Type，再编码事件数据
//
// 参数：
//   - w: HTTP 响应写入器
//
// 返回值：
//   - error: 渲染错误
func (r CustomEvent) Render(w http.ResponseWriter) error {
	r.WriteContentType(w)
	return encode(w, r)
}

// WriteContentType 设置 SSE 响应的 Content-Type
//
// 设置 Content-Type 为 text/event-stream
// 如果未设置 Cache-Control，则设置为 no-cache
//
// 参数：
//   - w: HTTP 响应写入器
func (r CustomEvent) WriteContentType(w http.ResponseWriter) {
	r.Mutex.Lock()
	defer r.Mutex.Unlock()
	header := w.Header()
	header["Content-Type"] = contentType

	// 如果未设置 Cache-Control，则设置为 no-cache
	if _, exist := header["Cache-Control"]; !exist {
		header["Cache-Control"] = noCache
	}
}
