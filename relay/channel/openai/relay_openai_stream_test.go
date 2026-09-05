package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/c1cada/NexusTok/constant"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/c1cada/NexusTok/types"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// notifyingResponseWriter 在第一个实际下游写入时通知测试，用来验证首个 SSE 不会
// 等待第二个上游事件。其余行为全部复用 Gin 原始 ResponseWriter。
type notifyingResponseWriter struct {
	gin.ResponseWriter
	firstWrite chan struct{}
	once       sync.Once
}

func (w *notifyingResponseWriter) notifyFirstWrite() {
	w.once.Do(func() {
		close(w.firstWrite)
	})
}

func (w *notifyingResponseWriter) Write(data []byte) (int, error) {
	w.notifyFirstWrite()
	return w.ResponseWriter.Write(data)
}

func (w *notifyingResponseWriter) WriteString(data string) (int, error) {
	w.notifyFirstWrite()
	return w.ResponseWriter.WriteString(data)
}

// TestOaiStreamHandlerFlushesFirstChunkWithoutWaitingForSecondSSE 回归覆盖通用 Chat
// 流的首字路径：第一条有效 SSE 到达并处理后必须立即写出，不能等下一个上游 chunk。
func TestOaiStreamHandlerFlushesFirstChunkWithoutWaitingForSecondSSE(t *testing.T) {
	oldMode := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(oldMode) })

	oldTimeout := constant.StreamingTimeout
	constant.StreamingTimeout = 30
	t.Cleanup(func() { constant.StreamingTimeout = oldTimeout })

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	writer := &notifyingResponseWriter{
		ResponseWriter: c.Writer,
		firstWrite:     make(chan struct{}),
	}
	c.Writer = writer

	reader, upstream := io.Pipe()
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       reader,
		Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
	}
	info := &relaycommon.RelayInfo{
		ChannelMeta: &relaycommon.ChannelMeta{
			UpstreamModelName: "gpt-stream-test",
		},
		IsStream:           true,
		RelayMode:          relayconstant.RelayModeChatCompletions,
		RelayFormat:        types.RelayFormatOpenAI,
		ShouldIncludeUsage: false,
		DisablePing:        true,
	}

	type result struct {
		err error
	}
	done := make(chan result, 1)
	go func() {
		_, relayErr := OaiStreamHandler(c, info, resp)
		if relayErr != nil {
			done <- result{err: relayErr}
			return
		}
		done <- result{}
	}()

	_, err := io.WriteString(upstream, "data: {\"id\":\"chatcmpl-first\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-stream-test\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"first\"},\"finish_reason\":null}]}\n\n")
	require.NoError(t, err)

	select {
	case <-writer.firstWrite:
		// 第一条数据已经写入；此时尚未发送第二个上游事件。
	case <-time.After(time.Second):
		t.Fatal("首个 SSE 在第二个上游事件到达前未被转发")
	}

	_, err = io.WriteString(upstream, "data: {\"id\":\"chatcmpl-first\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-stream-test\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n")
	require.NoError(t, err)
	_, err = io.WriteString(upstream, "data: {\"id\":\"chatcmpl-first\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"gpt-stream-test\",\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n")
	require.NoError(t, err)
	_, err = io.WriteString(upstream, "data: [DONE]\n\n")
	require.NoError(t, err)
	require.NoError(t, upstream.Close())

	select {
	case outcome := <-done:
		require.NoError(t, outcome.err)
	case <-time.After(3 * time.Second):
		t.Fatal("流式处理未在预期时间内结束")
	}

	got := recorder.Body.String()
	require.Contains(t, got, `"content":"first"`)
	require.Equal(t, 1, strings.Count(got, `"finish_reason":"stop"`))
	require.NotContains(t, got, `"usage":{"prompt_tokens":2`)
	require.Equal(t, 1, strings.Count(got, "data: [DONE]"))
	require.Equal(t, "text/event-stream", recorder.Header().Get("Content-Type"))
}
