// Package helper 提供了中继层的各种辅助函数和数据结构。
// 本文件实现了 StreamResult，用于在流式数据处理回调中记录错误、
// 信号停止或标记正常完成。它是 StreamScannerHandler 中 dataHandler 回调
// 与流控制逻辑之间的桥梁。
package helper

import (
	relaycommon "github.com/c1cada/NexusTok/relay/common"
)

// StreamResult 在每次 dataHandler 回调调用时传入，提供记录软错误、
// 信号致命停止或标记正常完成的方法。
// StreamScannerHandler 在每次回调后检查 IsStopped() 以决定是否继续处理。
type StreamResult struct {
	status  *relaycommon.StreamStatus // 关联的流状态对象
	stopped bool                      // 当前 chunk 处理后是否应停止
}

// newStreamResult 创建一个新的 StreamResult 实例。
func newStreamResult(status *relaycommon.StreamStatus) *StreamResult {
	return &StreamResult{status: status}
}

// Error 记录一个软错误。流处理会继续进行，不会因此停止。
// 可在单个 chunk 处理中多次调用。
//
// 参数：
//   - err: 要记录的错误，nil 值会被忽略
func (r *StreamResult) Error(err error) {
	if err == nil {
		return
	}
	r.status.RecordError(err.Error())
}

// Stop 记录一个致命错误并标记流在当前 chunk 处理后停止。
// 与 Error 不同，Stop 会终止流的后续处理。
//
// 参数：
//   - err: 致命错误，可为 nil（此时仅标记停止但不记录错误）
func (r *StreamResult) Stop(err error) {
	if err != nil {
		r.status.RecordError(err.Error())
	}
	r.status.SetEndReason(relaycommon.StreamEndReasonHandlerStop, err)
	r.stopped = true
}

// Done 信号处理器已正常完成处理（例如收到 Dify 的 "message_end" 事件）。
// 流在当前 chunk 处理后停止，结束原因为 StreamEndReasonDone。
func (r *StreamResult) Done() {
	r.status.SetEndReason(relaycommon.StreamEndReasonDone, nil)
	r.stopped = true
}

// IsStopped 返回在当前 chunk 处理中是否调用了 Stop() 或 Done()。
func (r *StreamResult) IsStopped() bool {
	return r.stopped
}

// reset 清除当前 chunk 的 stopped 标志，使对象可在下一个 chunk 中复用。
func (r *StreamResult) reset() {
	r.stopped = false
}
