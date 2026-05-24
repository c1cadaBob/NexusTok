// Package common - go-channel.go
// 该文件提供了 Go Channel 的安全操作工具函数
//
// 包含的功能：
// - 安全发送：向已关闭的 Channel 发送数据不会 panic
// - 超时发送：带超时的 Channel 发送操作
package common

import (
	"time"
)

// SafeSendBool 安全地向 bool Channel 发送值
//
// 如果 Channel 已关闭，不会 panic，而是返回 true 表示 Channel 已关闭
// 使用 defer/recover 捕获 panic
//
// 参数：
//   - ch: bool 类型的 Channel
//   - value: 要发送的值
//
// 返回值：
//   - closed: Channel 是否已关闭
func SafeSendBool(ch chan bool, value bool) (closed bool) {
	defer func() {
		// 如果发生 panic，说明 Channel 已关闭
		if recover() != nil {
			closed = true
		}
	}()

	// 如果 Channel 已关闭，这行会 panic
	ch <- value

	// 如果执行到这里，说明 Channel 未关闭
	return false
}

// SafeSendString 安全地向 string Channel 发送值
//
// 如果 Channel 已关闭，不会 panic，而是返回 true 表示 Channel 已关闭
//
// 参数：
//   - ch: string 类型的 Channel
//   - value: 要发送的值
//
// 返回值：
//   - closed: Channel 是否已关闭
func SafeSendString(ch chan string, value string) (closed bool) {
	defer func() {
		if recover() != nil {
			closed = true
		}
	}()

	ch <- value

	return false
}

// SafeSendStringTimeout 带超时地向 string Channel 发送值
//
// 如果发送成功返回 true，超时或 Channel 已关闭返回 false
//
// 参数：
//   - ch: string 类型的 Channel
//   - value: 要发送的值
//   - timeout: 超时时间（秒）
//
// 返回值：
//   - closed: 是否发送成功（true=成功，false=超时或已关闭）
func SafeSendStringTimeout(ch chan string, value string, timeout int) (closed bool) {
	defer func() {
		if recover() != nil {
			closed = false
		}
	}()

	select {
	case ch <- value:
		return true
	case <-time.After(time.Duration(timeout) * time.Second):
		return false
	}
}
