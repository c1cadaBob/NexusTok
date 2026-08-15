// Package helper 提供中继层的辅助工具
// 本文件为流式响应扫描器（StreamScanner）的单元测试
// 测试 SSE（Server-Sent Events）流的解析、chunk 处理、超时、Ping 心跳等功能
package helper

import (
	"context"           // 上下文取消测试
	"fmt"               // 格式化输出
	"io"                // IO 操作
	"net/http"          // HTTP 客户端
	"net/http/httptest" // HTTP 测试工具
	"strings"           // 字符串操作
	"sync"              // 同步原语
	"sync/atomic"       // 原子操作
	"testing"           // 测试框架
	"time"              // 时间处理

	"github.com/c1cada/NexusTok/constant"                  // 常量定义
	relaycommon "github.com/c1cada/NexusTok/relay/common"  // 中继通用类型
	"github.com/c1cada/NexusTok/setting/operation_setting" // 运营配置
	"github.com/gin-gonic/gin"                             // Gin Web 框架
	"github.com/stretchr/testify/assert"                   // 测试断言（非致命）
	"github.com/stretchr/testify/require"                  // 测试断言（致命）
)

// init 初始化测试环境
// 设置 Gin 为测试模式，减少日志输出
func init() {
	gin.SetMode(gin.TestMode)
}

// setupStreamTest 设置流式测试环境
// 创建测试用的 Gin 上下文、HTTP 响应和中继信息
//
// 参数：
//   - t: 测试实例
//   - body: 响应体读取器
//
// 返回值：
//   - *gin.Context: 测试用 Gin 上下文
//   - *http.Response: 测试用 HTTP 响应
//   - *relaycommon.RelayInfo: 测试用中继信息
func setupStreamTest(t *testing.T, body io.Reader) (*gin.Context, *http.Response, *relaycommon.RelayInfo) {
	t.Helper()

	// 保存并设置流式超时时间
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})

	// 创建测试用 HTTP 记录器和 Gin 上下文
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	// 创建测试用 HTTP 响应
	resp := &http.Response{
		Body: io.NopCloser(body),
	}

	// 创建测试用中继信息
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	return c, resp, info
}

// buildSSEBody 构建 SSE 格式的响应体
// 生成指定数量的 data chunk，最后以 [DONE] 结束
//
// 参数：
//   - n: chunk 数量
//
// 返回值：
//   - string: SSE 格式的响应体字符串
func buildSSEBody(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		// 每个 chunk 包含 id 和 content 字段
		fmt.Fprintf(&b, "data: {\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}\n", i, i)
	}
	b.WriteString("data: [DONE]\n") // 结束标记
	return b.String()
}

// slowReader 模拟慢速读取器
// 用于测试上游响应延迟的场景
type slowReader struct {
	r     io.Reader     // 底层读取器
	delay time.Duration // 每次读取的延迟时间
}

// Read 实现 io.Reader 接口
// 在读取前添加延迟，模拟慢速上游
func (s *slowReader) Read(p []byte) (int, error) {
	time.Sleep(s.delay)
	return s.r.Read(p)
}

// cancellationReader 在请求上下文取消后返回 context.Canceled，
// 用于复现响应体先于主循环报告读取错误的竞态。
type cancellationReader struct {
	started chan<- struct{}
	done    <-chan struct{}
}

// Read 等待请求取消后返回 context.Canceled，模拟底层 HTTP 响应体的行为。
func (r *cancellationReader) Read(_ []byte) (int, error) {
	select {
	case r.started <- struct{}{}:
	default:
	}
	<-r.done
	return 0, context.Canceled
}

// ---------- 基本正确性测试 ----------

// TestStreamScannerHandler_NilInputs 测试空输入处理
// 验证当响应体或回调函数为 nil 时不会 panic
func TestStreamScannerHandler_NilInputs(t *testing.T) {
	t.Parallel()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/", nil)

	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	// 响应体为 nil 时不应 panic
	StreamScannerHandler(c, nil, info, func(data string, sr *StreamResult) {})
	// 回调函数为 nil 时不应 panic
	StreamScannerHandler(c, &http.Response{Body: io.NopCloser(strings.NewReader(""))}, info, nil)
}

// TestStreamScannerHandler_EmptyBody 测试空响应体
// 验证当响应体为空时，回调函数不应被调用
func TestStreamScannerHandler_EmptyBody(t *testing.T) {
	t.Parallel()

	c, resp, info := setupStreamTest(t, strings.NewReader(""))

	var called atomic.Bool
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		called.Store(true)
	})

	assert.False(t, called.Load(), "空响应体时不应调用回调函数")
}

// TestStreamScannerHandler_1000Chunks 测试 1000 个 chunk 的处理
// 验证扫描器能正确处理中等数量的 chunk
func TestStreamScannerHandler_1000Chunks(t *testing.T) {
	t.Parallel()

	const numChunks = 1000
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(numChunks), count.Load())        // 应处理所有 chunk
	assert.Equal(t, numChunks, info.ReceivedResponseCount) // 计数器应正确
}

// TestStreamScannerHandler_10000Chunks 测试 10000 个 chunk 的处理
// 验证扫描器能正确处理大量 chunk，并记录处理时间
func TestStreamScannerHandler_10000Chunks(t *testing.T) {
	t.Parallel()

	const numChunks = 10000
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	start := time.Now()

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	elapsed := time.Since(start)
	assert.Equal(t, int64(numChunks), count.Load()) // 应处理所有 chunk
	assert.Equal(t, numChunks, info.ReceivedResponseCount)
	t.Logf("处理 10000 个 chunk 耗时: %v", elapsed)
}

// TestStreamScannerHandler_OrderPreserved 测试 chunk 顺序保持
// 验证扫描器保持 chunk 的原始顺序，不因并发处理而乱序
func TestStreamScannerHandler_OrderPreserved(t *testing.T) {
	t.Parallel()

	const numChunks = 500
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var mu sync.Mutex
	received := make([]string, 0, numChunks) // 收集接收到的数据

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		mu.Lock()
		received = append(received, data) // 保持顺序
		mu.Unlock()
	})

	require.Equal(t, numChunks, len(received))
	// 验证每个 chunk 的顺序正确
	for i := 0; i < numChunks; i++ {
		expected := fmt.Sprintf("{\"id\":%d,\"choices\":[{\"delta\":{\"content\":\"token_%d\"}}]}", i, i)
		assert.Equal(t, expected, received[i], "chunk %d 顺序错误", i)
	}
}

// TestStreamScannerHandler_DoneStopsScanner 测试 [DONE] 标记停止扫描器
// 验证收到 [DONE] 后不再处理后续数据
func TestStreamScannerHandler_DoneStopsScanner(t *testing.T) {
	t.Parallel()

	// 在 [DONE] 后添加不应被处理的数据
	body := buildSSEBody(50) + "data: should_not_appear\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(50), count.Load(), "[DONE] 之后的数据不应被处理")
}

// TestStreamScannerHandler_StopStopsStream 测试 Stop 方法停止流
// 验证回调函数调用 Stop 后立即停止处理后续 chunk
func TestStreamScannerHandler_StopStopsStream(t *testing.T) {
	t.Parallel()

	const numChunks = 200
	body := buildSSEBody(numChunks)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	const stopAt int64 = 50 // 在第 50 个 chunk 时停止
	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= stopAt {
			sr.Stop(fmt.Errorf("在第 %d 个 chunk 时停止", n)) // 触发停止
		}
	})

	assert.Equal(t, stopAt, count.Load()) // 应只处理到 stopAt 个 chunk
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason) // 停止原因应为 HandlerStop
}

// TestStreamScannerHandler_SkipsNonDataLines 测试跳过非数据行
// 验证扫描器正确跳过注释行（: 开头）、event 行、id 行、retry 行
func TestStreamScannerHandler_SkipsNonDataLines(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString(": comment line\n") // 注释行
	b.WriteString("event: message\n") // 事件行
	b.WriteString("id: 12345\n")      // ID 行
	b.WriteString("retry: 5000\n")    // 重试行
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "data: payload_%d\n", i) // 数据行
		b.WriteString(": interleaved comment\n") // 交错的注释行
	}
	b.WriteString("data: [DONE]\n")

	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
	})

	assert.Equal(t, int64(100), count.Load()) // 应只处理 100 个数据行
}

// TestStreamScannerHandler_DataWithExtraSpaces 测试处理带多余空格的数据
// 验证扫描器正确修剪 data 行的前导和尾随空格
func TestStreamScannerHandler_DataWithExtraSpaces(t *testing.T) {
	t.Parallel()

	body := "data:   {\"trimmed\":true}  \ndata: [DONE]\n"
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var got string
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		got = data
	})

	assert.Equal(t, "{\"trimmed\":true}", got) // 空格应被修剪
}

// ---------- 解耦测试 ----------

// TestStreamScannerHandler_ScannerDecoupledFromSlowHandler 测试扫描器与慢处理函数解耦
// 验证扫描器的读取速度不受慢处理函数影响，实现生产者-消费者解耦
func TestStreamScannerHandler_ScannerDecoupledFromSlowHandler(t *testing.T) {
	t.Parallel()

	const numChunks = 50
	const upstreamDelay = 10 * time.Millisecond // 上游延迟
	const handlerDelay = 20 * time.Millisecond  // 处理函数延迟（比上游慢）

	// 创建管道，模拟慢速上游
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < numChunks; i++ {
			fmt.Fprintf(pw, "data: {\"id\":%d}\n", i)
			time.Sleep(upstreamDelay)
		}
		fmt.Fprint(pw, "data: [DONE]\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	var count atomic.Int64
	start := time.Now()
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			time.Sleep(handlerDelay) // 模拟慢处理
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("StreamScannerHandler 未在规定时间内完成")
	}

	elapsed := time.Since(start)
	assert.Equal(t, int64(numChunks), count.Load())

	// 计算耦合模式下的理论耗时（每个 chunk 都要等待上游+处理）
	coupledTime := time.Duration(numChunks) * (upstreamDelay + handlerDelay)
	t.Logf("elapsed=%v, coupled_estimate=%v", elapsed, coupledTime)

	// 解耦模式下实际耗时应显著小于耦合模式
	assert.Less(t, elapsed, coupledTime*85/100,
		"解耦模式耗时 (%v) 应显著小于耦合模式估算 (%v)", elapsed, coupledTime)
}

// TestStreamScannerHandler_SlowUpstreamFastHandler 测试慢速上游快速处理函数
// 验证扫描器能正确处理上游延迟但处理函数快速的场景
func TestStreamScannerHandler_SlowUpstreamFastHandler(t *testing.T) {
	t.Parallel()

	const numChunks = 50
	body := buildSSEBody(numChunks)
	reader := &slowReader{r: strings.NewReader(body), delay: 2 * time.Millisecond} // 模拟慢速上游
	c, resp, info := setupStreamTest(t, reader)

	var count atomic.Int64
	start := time.Now()

	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1) // 快速处理
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("慢速上游测试超时")
	}

	elapsed := time.Since(start)
	assert.Equal(t, int64(numChunks), count.Load())
	t.Logf("慢速上游 (%d 个 chunk, 每次读取 2ms): %v", numChunks, elapsed)
}

// ---------- Ping 心跳测试 ----------

// TestStreamScannerHandler_PingSentDuringSlowUpstream 测试慢速上游时发送 Ping 心跳
// 验证在上游响应缓慢时，扫描器定期发送 ": PING" 心跳保持连接
func TestStreamScannerHandler_PingSentDuringSlowUpstream(t *testing.T) {
	t.Parallel()

	// 启用 Ping 心跳，间隔 1 秒
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	// 创建慢速上游（每 500ms 发送一个 chunk，共 7 个）
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < 7; i++ {
			fmt.Fprintf(pw, "data: chunk_%d\n", i)
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Fprint(pw, "data: [DONE]\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("等待流完成超时")
	}

	assert.Equal(t, int64(7), count.Load())

	// 检查响应体中是否包含 Ping 心跳
	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	t.Logf("在 3.5 秒流中收到 %d 个 Ping", pingCount)
	assert.GreaterOrEqual(t, pingCount, 2,
		"在 3.5 秒流中（1 秒间隔）应至少收到 2 个 Ping，实际收到 %d 个", pingCount)
}

// TestStreamScannerHandler_PingDisabledByRelayInfo 测试通过 RelayInfo 禁用 Ping
// 验证当 DisablePing=true 时，即使配置启用了 Ping 也不会发送心跳
func TestStreamScannerHandler_PingDisabledByRelayInfo(t *testing.T) {
	t.Parallel()

	// 启用 Ping 心跳配置
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	// 创建慢速上游
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < 5; i++ {
			fmt.Fprintf(pw, "data: chunk_%d\n", i)
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Fprint(pw, "data: [DONE]\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})

	resp := &http.Response{Body: pr}
	// 设置 DisablePing=true 禁用 Ping
	info := &relaycommon.RelayInfo{
		DisablePing: true,
		ChannelMeta: &relaycommon.ChannelMeta{},
	}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("测试超时")
	}

	assert.Equal(t, int64(5), count.Load())

	// 验证响应体中没有 Ping 心跳
	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	assert.Equal(t, 0, pingCount, "DisablePing=true 时不应发送 Ping")
}

// ---------- StreamStatus 集成测试 ----------

// TestStreamScannerHandler_StreamStatus_DoneReason 测试正常完成的流状态
// 验证收到 [DONE] 后 StreamStatus 的 EndReason 为 StreamEndReasonDone
func TestStreamScannerHandler_StreamStatus_DoneReason(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason) // 正常完成
	assert.Nil(t, info.StreamStatus.EndError)                                     // 无错误
	assert.True(t, info.StreamStatus.IsNormalEnd())                               // 是正常结束
	assert.False(t, info.StreamStatus.HasErrors())                                // 无错误
}

// TestStreamScannerHandler_StreamStatus_EOFWithoutDone 测试无 [DONE] 标记的 EOF 结束
// 验证当响应体结束但没有 [DONE] 标记时，StreamStatus 的 EndReason 为 StreamEndReasonEOF
func TestStreamScannerHandler_StreamStatus_EOFWithoutDone(t *testing.T) {
	t.Parallel()

	// 构建没有 [DONE] 标记的响应体
	var b strings.Builder
	for i := 0; i < 5; i++ {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonEOF, info.StreamStatus.EndReason) // EOF 结束
	assert.True(t, info.StreamStatus.IsNormalEnd())                              // EOF 也算正常结束
}

// TestStreamScannerHandler_StreamStatus_HandlerStop 测试 Handler 主动停止的流状态
// 验证当回调函数调用 Stop 时，StreamStatus 的 EndReason 为 StreamEndReasonHandlerStop
func TestStreamScannerHandler_StreamStatus_HandlerStop(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(100)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 10 {
			sr.Stop(fmt.Errorf("在第 10 个 chunk 时停止")) // 主动停止
		}
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason) // Handler 停止
	assert.True(t, info.StreamStatus.HasErrors())                                        // Stop 包含错误
}

// TestStreamScannerHandler_StreamStatus_HandlerDone 测试 Handler 主动完成的流状态
// 验证当回调函数调用 Done 时，StreamStatus 的 EndReason 为 StreamEndReasonDone
func TestStreamScannerHandler_StreamStatus_HandlerDone(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(20)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		n := count.Add(1)
		if n >= 5 {
			sr.Done() // 主动完成
		}
	})

	assert.Equal(t, int64(5), count.Load()) // 应只处理 5 个 chunk
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason) // Done 完成
	assert.False(t, info.StreamStatus.HasErrors())                                // Done 无错误
}

// TestStreamScannerHandler_StreamStatus_Timeout 测试超时的流状态
// 验证当上游响应超时时，StreamStatus 的 EndReason 为 StreamEndReasonTimeout
func TestStreamScannerHandler_StreamStatus_Timeout(t *testing.T) {
	// 不并行：修改全局超时常量
	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 2 // 设置 2 秒超时
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	// 创建一个发送 1 个 chunk 后延迟 10 秒的响应体
	pr, pw := io.Pipe()
	go func() {
		fmt.Fprint(pw, "data: {\"id\":1}\n")
		time.Sleep(10 * time.Second) // 远超超时时间
		pw.Close()
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("等待流超时超时")
	}

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonTimeout, info.StreamStatus.EndReason) // 超时结束
	assert.False(t, info.StreamStatus.IsNormalEnd())                                 // 超时不是正常结束
}

// TestStreamScannerHandler_StreamStatus_ClientGoneScannerError 测试客户端取消先触发
// 响应体读取错误时仍然归类为 client_gone，而不是 scanner_error。
func TestStreamScannerHandler_StreamStatus_ClientGoneScannerError(t *testing.T) {
	t.Parallel()

	requestContext, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil).
		WithContext(requestContext)

	readStarted := make(chan struct{}, 1)
	resp := &http.Response{
		Body: io.NopCloser(&cancellationReader{
			started: readStarted,
			done:    requestContext.Done(),
		}),
	}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	finished := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})
		close(finished)
	}()

	select {
	case <-readStarted:
		cancel()
	case <-time.After(2 * time.Second):
		t.Fatal("扫描器未开始读取响应体")
	}

	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("客户端取消后的流处理未结束")
	}

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonClientGone, info.StreamStatus.EndReason)
	assert.Equal(t, context.Canceled, info.StreamStatus.EndError)
	assert.Equal(t, relaycommon.StreamSeverityWarning, info.StreamStatus.Severity())
}

// TestStreamScannerHandler_StreamStatus_SoftErrors 测试软错误的流状态
// 验证当回调函数报告软错误（Error）时，StreamStatus 记录错误但不中断流
func TestStreamScannerHandler_StreamStatus_SoftErrors(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(10)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("chunk 处理软错误")) // 报告软错误
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason) // 仍然正常完成
	assert.True(t, info.StreamStatus.HasErrors())                                 // 有错误
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount())                      // 每个 chunk 1 个错误，共 10 个
}

// TestStreamScannerHandler_StreamStatus_MultipleErrorsPerChunk 测试每个 chunk 多个错误
// 验证每个 chunk 可以报告多个错误，StreamStatus 正确累计
func TestStreamScannerHandler_StreamStatus_MultipleErrorsPerChunk(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		sr.Error(fmt.Errorf("错误 A")) // 第一个错误
		sr.Error(fmt.Errorf("错误 B")) // 第二个错误
	})

	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason)
	assert.Equal(t, 10, info.StreamStatus.TotalErrorCount()) // 5 个 chunk × 2 个错误 = 10
}

// TestStreamScannerHandler_StreamStatus_ErrorThenStop 测试先报告错误再停止
// 验证当回调函数先报告软错误再调用 Stop 时，StreamStatus 正确记录
func TestStreamScannerHandler_StreamStatus_ErrorThenStop(t *testing.T) {
	t.Parallel()

	// 使用没有 [DONE] 标记的大响应体，避免扫描器的 [DONE] 和 Handler 的 Stop 之间的竞争
	var b strings.Builder
	for i := 0; i < 100; i++ {
		fmt.Fprintf(&b, "data: {\"id\":%d}\n", i)
	}
	c, resp, info := setupStreamTest(t, strings.NewReader(b.String()))

	var count atomic.Int64
	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
		count.Add(1)
		sr.Error(fmt.Errorf("软错误")) // 报告软错误
		sr.Stop(fmt.Errorf("致命错误")) // 立即停止
	})

	assert.Equal(t, int64(1), count.Load()) // 只处理了 1 个 chunk
	require.NotNil(t, info.StreamStatus)
	assert.Equal(t, relaycommon.StreamEndReasonHandlerStop, info.StreamStatus.EndReason) // 停止原因为 HandlerStop
	assert.Equal(t, 2, info.StreamStatus.TotalErrorCount())                              // 1 个软错误 + 1 个致命错误 = 2
}

// TestStreamScannerHandler_StreamStatus_InitializedIfNil 测试 StreamStatus 自动初始化
// 验证当 info.StreamStatus 为 nil 时，扫描器会自动初始化
func TestStreamScannerHandler_StreamStatus_InitializedIfNil(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(1)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	assert.Nil(t, info.StreamStatus) // 初始为 nil

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.NotNil(t, info.StreamStatus) // 处理后应被初始化
}

// TestStreamScannerHandler_StreamStatus_PreInitialized 测试预初始化的 StreamStatus
// 验证当 info.StreamStatus 已预初始化时，扫描器正确合并状态
func TestStreamScannerHandler_StreamStatus_PreInitialized(t *testing.T) {
	t.Parallel()

	body := buildSSEBody(5)
	c, resp, info := setupStreamTest(t, strings.NewReader(body))

	// 预初始化 StreamStatus 并添加一个预存错误
	info.StreamStatus = relaycommon.NewStreamStatus()
	info.StreamStatus.RecordError("预存错误")

	StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {})

	assert.Equal(t, relaycommon.StreamEndReasonDone, info.StreamStatus.EndReason) // 正常完成
	assert.Equal(t, 1, info.StreamStatus.TotalErrorCount())                       // 预存错误应被保留
}

// TestStreamScannerHandler_PingInterleavesWithSlowUpstream 测试 Ping 与慢速上游交错
// 验证 Ping 心跳在慢速上游响应期间正确交错发送
func TestStreamScannerHandler_PingInterleavesWithSlowUpstream(t *testing.T) {
	t.Parallel()

	// 启用 Ping 心跳，间隔 1 秒
	setting := operation_setting.GetGeneralSetting()
	oldEnabled := setting.PingIntervalEnabled
	oldSeconds := setting.PingIntervalSeconds
	setting.PingIntervalEnabled = true
	setting.PingIntervalSeconds = 1
	t.Cleanup(func() {
		setting.PingIntervalEnabled = oldEnabled
		setting.PingIntervalSeconds = oldSeconds
	})

	// 创建慢速上游（每 500ms 发送一个 chunk，共 10 个，总耗时 5 秒）
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		for i := 0; i < 10; i++ {
			fmt.Fprintf(pw, "data: chunk_%d\n", i)
			time.Sleep(500 * time.Millisecond)
		}
		fmt.Fprint(pw, "data: [DONE]\n")
	}()

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() {
		constant.StreamingTimeout = oldTimeout
	})

	resp := &http.Response{Body: pr}
	info := &relaycommon.RelayInfo{ChannelMeta: &relaycommon.ChannelMeta{}}

	var count atomic.Int64
	done := make(chan struct{})
	go func() {
		StreamScannerHandler(c, resp, info, func(data string, sr *StreamResult) {
			count.Add(1)
		})
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("测试超时")
	}

	assert.Equal(t, int64(10), count.Load())

	// 检查响应体中 Ping 心跳的数量
	body := recorder.Body.String()
	pingCount := strings.Count(body, ": PING")
	t.Logf("在 5 秒流中（10 个 chunk，500ms 间隔）收到 %d 个 Ping", pingCount)
	assert.GreaterOrEqual(t, pingCount, 3,
		"在 5 秒流中（1 秒 Ping 间隔）应至少收到 3 个 Ping，实际收到 %d 个", pingCount)
}
