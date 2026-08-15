// 本文件是 relay/common 包中 StreamStatus 结构体的单元测试集。
// 覆盖了结束原因设置（首次生效、并发安全）、错误记录（基本功能、容量上限、并发安全）、
// 正常结束判断和状态摘要生成等功能。
package common

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestStreamStatus_SetEndReason_FirstWins 测试结束原因只接受第一次设置（sync.Once 语义）。
func TestStreamStatus_SetEndReason_FirstWins(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	s.SetEndReason(StreamEndReasonDone, nil)
	s.SetEndReason(StreamEndReasonTimeout, nil)
	s.SetEndReason(StreamEndReasonClientGone, fmt.Errorf("context canceled"))

	assert.Equal(t, StreamEndReasonDone, s.EndReason)
	assert.Nil(t, s.EndError)
}

// TestStreamStatus_SetEndReason_WithError 测试带错误信息的结束原因设置。
func TestStreamStatus_SetEndReason_WithError(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	expectedErr := fmt.Errorf("read: connection reset")
	s.SetEndReason(StreamEndReasonScannerErr, expectedErr)

	assert.Equal(t, StreamEndReasonScannerErr, s.EndReason)
	assert.Equal(t, expectedErr, s.EndError)
}

// TestStreamStatus_SetEndReason_NilSafe 测试 nil 接收者的空指针安全性。
func TestStreamStatus_SetEndReason_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	s.SetEndReason(StreamEndReasonDone, nil)
}

// TestStreamStatus_SetEndReason_Concurrent 测试并发设置结束原因的安全性。
func TestStreamStatus_SetEndReason_Concurrent(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	reasons := []StreamEndReason{
		StreamEndReasonDone,
		StreamEndReasonTimeout,
		StreamEndReasonClientGone,
		StreamEndReasonScannerErr,
		StreamEndReasonHandlerStop,
		StreamEndReasonEOF,
		StreamEndReasonPanic,
		StreamEndReasonPingFail,
	}

	var wg sync.WaitGroup
	for _, r := range reasons {
		wg.Add(1)
		go func(reason StreamEndReason) {
			defer wg.Done()
			s.SetEndReason(reason, nil)
		}(r)
	}
	wg.Wait()

	assert.NotEqual(t, StreamEndReasonNone, s.EndReason)
}

// TestStreamStatus_RecordError_Basic 测试基本的错误记录功能。
func TestStreamStatus_RecordError_Basic(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	s.RecordError("bad json")
	s.RecordError("another bad json")
	s.RecordError("client gone")

	assert.True(t, s.HasErrors())
	assert.Equal(t, 3, s.TotalErrorCount())
	assert.Len(t, s.Errors, 3)
}

// TestStreamStatus_RecordError_CapAtMax 测试错误记录超过上限后仅递增计数器。
func TestStreamStatus_RecordError_CapAtMax(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	for i := 0; i < 30; i++ {
		s.RecordError(fmt.Sprintf("error_%d", i))
	}

	assert.Equal(t, maxStreamErrorEntries, len(s.Errors))
	assert.Equal(t, 30, s.TotalErrorCount())
}

// TestStreamStatus_RecordError_NilSafe 测试 nil 接收者的错误记录空指针安全性。
func TestStreamStatus_RecordError_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	s.RecordError("should not panic")
}

// TestStreamStatus_RecordError_Concurrent 测试并发错误记录的安全性。
func TestStreamStatus_RecordError_Concurrent(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			s.RecordError(fmt.Sprintf("error_%d", idx))
		}(i)
	}
	wg.Wait()

	assert.Equal(t, 100, s.TotalErrorCount())
	assert.LessOrEqual(t, len(s.Errors), maxStreamErrorEntries)
}

// TestStreamStatus_HasErrors_Empty 测试空状态下的 HasErrors 返回 false。
func TestStreamStatus_HasErrors_Empty(t *testing.T) {
	t.Parallel()
	s := NewStreamStatus()
	assert.False(t, s.HasErrors())
	assert.Equal(t, 0, s.TotalErrorCount())
}

// TestStreamStatus_HasErrors_NilSafe 测试 nil 接收者的 HasErrors 空指针安全性。
func TestStreamStatus_HasErrors_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.False(t, s.HasErrors())
	assert.Equal(t, 0, s.TotalErrorCount())
}

// TestStreamStatus_IsNormalEnd 测试各种结束原因的正常结束判断。
func TestStreamStatus_IsNormalEnd(t *testing.T) {
	t.Parallel()
	tests := []struct {
		reason StreamEndReason
		normal bool
	}{
		{StreamEndReasonDone, true},
		{StreamEndReasonEOF, true},
		{StreamEndReasonHandlerStop, true},
		{StreamEndReasonTimeout, false},
		{StreamEndReasonClientGone, false},
		{StreamEndReasonScannerErr, false},
		{StreamEndReasonPanic, false},
		{StreamEndReasonPingFail, false},
		{StreamEndReasonNone, false},
	}
	for _, tt := range tests {
		s := NewStreamStatus()
		s.SetEndReason(tt.reason, nil)
		assert.Equal(t, tt.normal, s.IsNormalEnd(), "reason=%s", tt.reason)
	}
}

// TestStreamStatus_IsNormalEnd_NilSafe 测试 nil 接收者的 IsNormalEnd 空指针安全性。
func TestStreamStatus_IsNormalEnd_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.True(t, s.IsNormalEnd())
}

// TestStreamStatus_SeverityDistinguishesClientInterruption 测试客户端中断与真实错误
// 的展示级别不同，同时确认 client_gone 不会改变 IsNormalEnd 的历史语义。
func TestStreamStatus_SeverityDistinguishesClientInterruption(t *testing.T) {
	t.Parallel()

	clientGone := NewStreamStatus()
	clientGone.SetEndReason(StreamEndReasonClientGone, fmt.Errorf("context canceled"))
	assert.False(t, clientGone.IsNormalEnd())
	assert.True(t, clientGone.IsClientGone())
	assert.Equal(t, StreamSeverityWarning, clientGone.Severity())

	timeout := NewStreamStatus()
	timeout.SetEndReason(StreamEndReasonTimeout, fmt.Errorf("read timeout"))
	assert.False(t, timeout.IsClientGone())
	assert.Equal(t, StreamSeverityError, timeout.Severity())

	success := NewStreamStatus()
	success.SetEndReason(StreamEndReasonDone, nil)
	assert.Equal(t, StreamSeverityOK, success.Severity())
}

// TestStreamStatus_SeverityNilSafe 测试空状态的严重级别回退。
func TestStreamStatus_SeverityNilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.Equal(t, StreamSeverityOK, s.Severity())
	assert.False(t, s.IsClientGone())
}

// TestStreamStatus_Summary 测试状态摘要的生成格式。
func TestStreamStatus_Summary(t *testing.T) {
	t.Parallel()

	s := NewStreamStatus()
	s.SetEndReason(StreamEndReasonDone, nil)
	summary := s.Summary()
	assert.Contains(t, summary, "reason=done")
	assert.NotContains(t, summary, "soft_errors")

	s2 := NewStreamStatus()
	s2.SetEndReason(StreamEndReasonTimeout, nil)
	s2.RecordError("bad json")
	s2.RecordError("write failed")
	summary2 := s2.Summary()
	assert.Contains(t, summary2, "reason=timeout")
	assert.Contains(t, summary2, "soft_errors=2")
}

// TestStreamStatus_Summary_NilSafe 测试 nil 接收者的 Summary 空指针安全性。
func TestStreamStatus_Summary_NilSafe(t *testing.T) {
	t.Parallel()
	var s *StreamStatus
	assert.Equal(t, "StreamStatus<nil>", s.Summary())
}
