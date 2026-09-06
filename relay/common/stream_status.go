// Package common 提供了中继层的通用工具函数和数据结构。
// 本文件实现了流式响应的状态跟踪机制，包括结束原因记录、软错误收集和状态摘要生成。
// StreamStatus 是并发安全的，可在多个 goroutine 中使用。
package common

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// StreamEndReason 表示流式响应结束的原因类型。
type StreamEndReason string

const (
	StreamEndReasonNone        StreamEndReason = ""              // 未设置结束原因
	StreamEndReasonDone        StreamEndReason = "done"          // 正常完成（收到 [DONE] 标记）
	StreamEndReasonTimeout     StreamEndReason = "timeout"       // 读取超时
	StreamEndReasonClientGone  StreamEndReason = "client_gone"   // 客户端断开连接
	StreamEndReasonScannerErr  StreamEndReason = "scanner_error" // 扫描器读取错误
	StreamEndReasonHandlerStop StreamEndReason = "handler_stop"  // 处理器主动停止
	StreamEndReasonEOF         StreamEndReason = "eof"           // 流正常结束（EOF）
	StreamEndReasonPanic       StreamEndReason = "panic"         // 处理过程中发生 panic
	StreamEndReasonPingFail    StreamEndReason = "ping_fail"     // 心跳 ping 发送失败
)

// StreamSeverity 表示流式请求结束时对运维人员的诊断严重级别。
//
// client_gone 仍然属于非完整正常结束，不能改变计费、性能统计或退款判断；
// 但它通常表示下游主动取消请求，因此只作为可预期警告展示，避免与上游故障混淆。
type StreamSeverity string

const (
	StreamSeverityOK      StreamSeverity = "ok"
	StreamSeverityWarning StreamSeverity = "warning"
	StreamSeverityError   StreamSeverity = "error"
)

// maxStreamErrorEntries 是 StreamStatus 中存储的错误条目最大数量。
// 超过此数量后，错误计数器仍会递增，但不再存储新的错误条目以控制内存使用。
const maxStreamErrorEntries = 20

// StreamErrorEntry 表示流式处理过程中的单个错误记录。
type StreamErrorEntry struct {
	Message   string    // 错误消息内容
	Timestamp time.Time // 错误发生的时间戳
}

// StreamStatus 跟踪流式响应的完整生命周期状态。
// 包括结束原因、结束错误和软错误列表。
// 该结构体是并发安全的，通过 sync.Once 保证结束原因只设置一次，
// 通过 sync.Mutex 保护错误列表的并发读写。
type StreamStatus struct {
	EndReason StreamEndReason // 流结束的原因
	EndError  error           // 流结束时的错误（如果有的话）
	endOnce   sync.Once       // 保证 EndReason 只被设置一次

	mu         sync.Mutex         // 保护 Errors 和 ErrorCount 的并发访问
	Errors     []StreamErrorEntry // 软错误列表（最多存储 maxStreamErrorEntries 条）
	ErrorCount int                // 错误总计数（包括未存储的溢出错误）
}

// NewStreamStatus 创建并返回一个新的 StreamStatus 实例。
func NewStreamStatus() *StreamStatus {
	return &StreamStatus{}
}

// SetEndReason 设置流的结束原因和关联错误。
// 使用 sync.Once 保证只有第一次调用生效，后续调用将被忽略。
// 支持 nil 接收者调用（空指针安全）。
//
// 参数：
//   - reason: 流结束的原因
//   - err: 与结束关联的错误（可为 nil）
func (s *StreamStatus) SetEndReason(reason StreamEndReason, err error) {
	if s == nil {
		return
	}
	s.endOnce.Do(func() {
		s.EndReason = reason
		s.EndError = err
	})
}

// RecordError 记录一个软错误。软错误不会终止流处理，但会被记录用于诊断。
// 错误条目数量超过 maxStreamErrorEntries 后，仅递增计数器不再存储详情。
// 支持 nil 接收者调用（空指针安全）。
//
// 参数：
//   - msg: 错误消息字符串
func (s *StreamStatus) RecordError(msg string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ErrorCount++
	if len(s.Errors) < maxStreamErrorEntries {
		s.Errors = append(s.Errors, StreamErrorEntry{
			Message:   msg,
			Timestamp: time.Now(),
		})
	}
}

// HasErrors 检查是否记录了任何软错误。
// 支持 nil 接收者调用（空指针安全），返回 false。
func (s *StreamStatus) HasErrors() bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount > 0
}

// TotalErrorCount 返回记录的错误总数。
// 支持 nil 接收者调用（空指针安全），返回 0。
func (s *StreamStatus) TotalErrorCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ErrorCount
}

// IsNormalEnd 判断流是否正常结束。
// 正常结束的定义：结束原因为 Done（收到 [DONE]）、EOF（流关闭）或 HandlerStop（处理器主动完成）。
// 支持 nil 接收者调用（空指针安全），返回 true。
func (s *StreamStatus) IsNormalEnd() bool {
	if s == nil {
		return true
	}
	return s.EndReason == StreamEndReasonDone ||
		s.EndReason == StreamEndReasonEOF ||
		s.EndReason == StreamEndReasonHandlerStop
}

// IsUpstreamCompleted 判断流是否收到明确完整结束信号且处理过程没有软错误。
// done、正常 EOF 与 handler 主动完成都代表上游生命周期已经结束；timeout、
// scanner_error、panic、ping_fail 和未完成的 client_gone 均返回 false。
func (s *StreamStatus) IsUpstreamCompleted() bool {
	return s != nil && s.IsNormalEnd() && !s.HasErrors()
}

// IsClientGone 判断流是否因为下游请求上下文被取消而中断。
//
// 该判断只认明确的 client_gone 结束原因，不根据错误文本猜测，避免把上游
// connection reset、超时或其他扫描器错误错误地降级为普通警告。
func (s *StreamStatus) IsClientGone() bool {
	return s != nil && s.EndReason == StreamEndReasonClientGone
}

// Severity 返回流状态用于日志和前端展示的严重级别。
//
// status 字段仍由消费日志按旧规则写入 error/ok；本方法只提供额外的诊断
// 维度，确保历史消费者不受影响。
func (s *StreamStatus) Severity() StreamSeverity {
	if s == nil || (s.IsNormalEnd() && !s.HasErrors()) {
		return StreamSeverityOK
	}
	if s.IsClientGone() {
		return StreamSeverityWarning
	}
	return StreamSeverityError
}

// Summary 生成流状态的摘要字符串，包含结束原因、错误信息和软错误计数。
// 格式示例："reason=done" 或 "reason=timeout end_error=\"read timeout\" soft_errors=3"
// 支持 nil 接收者调用（空指针安全），返回 "StreamStatus<nil>"。
func (s *StreamStatus) Summary() string {
	if s == nil {
		return "StreamStatus<nil>"
	}
	b := &strings.Builder{}
	fmt.Fprintf(b, "reason=%s", s.EndReason)
	if s.EndError != nil {
		fmt.Fprintf(b, " end_error=%q", s.EndError.Error())
	}
	s.mu.Lock()
	if s.ErrorCount > 0 {
		fmt.Fprintf(b, " soft_errors=%d", s.ErrorCount)
	}
	s.mu.Unlock()
	return b.String()
}
