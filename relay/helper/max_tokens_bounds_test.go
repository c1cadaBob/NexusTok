package helper

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	relayconstant "github.com/c1cada/NexusTok/relay/constant"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// TestMaxTokensBounds 覆盖所有会进入预扣费和 provider int 转换的最大输出 token 字段。
// 这些字段如果允许极端 uint 值穿透，后续额度估算可能发生溢出或异常扣费。
func TestMaxTokensBounds(t *testing.T) {
	gin.SetMode(gin.TestMode)

	newJSONContext := func(t *testing.T, body string) *gin.Context {
		t.Helper()
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest(http.MethodPost, "/relay", bytes.NewBufferString(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		return ctx
	}

	const hugeN = "18446744073686646784"

	t.Run("OpenAI max_tokens 超上限被拒绝", func(t *testing.T) {
		ctx := newJSONContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_tokens":`+hugeN+`}`)
		_, err := GetAndValidateTextRequest(ctx, relayconstant.RelayModeChatCompletions)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("OpenAI max_completion_tokens 超上限被拒绝", func(t *testing.T) {
		ctx := newJSONContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":`+hugeN+`}`)
		_, err := GetAndValidateTextRequest(ctx, relayconstant.RelayModeChatCompletions)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("OpenAI 正常 max_completion_tokens 通过", func(t *testing.T) {
		ctx := newJSONContext(t, `{"model":"gpt-4o","messages":[{"role":"user","content":"hi"}],"max_completion_tokens":8192}`)
		req, err := GetAndValidateTextRequest(ctx, relayconstant.RelayModeChatCompletions)
		require.NoError(t, err)
		require.NotNil(t, req.MaxCompletionTokens)
		require.EqualValues(t, 8192, *req.MaxCompletionTokens)
	})

	t.Run("Claude max_tokens 超上限被拒绝", func(t *testing.T) {
		ctx := newJSONContext(t, `{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hi"}],"max_tokens":`+hugeN+`}`)
		_, err := GetAndValidateClaudeRequest(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("Claude max_tokens_to_sample 超上限被拒绝", func(t *testing.T) {
		ctx := newJSONContext(t, `{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hi"}],"max_tokens_to_sample":`+hugeN+`}`)
		_, err := GetAndValidateClaudeRequest(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_tokens is invalid")
	})

	t.Run("Claude 正常 max_tokens 通过", func(t *testing.T) {
		ctx := newJSONContext(t, `{"model":"claude-sonnet-4","messages":[{"role":"user","content":"hi"}],"max_tokens":8192}`)
		req, err := GetAndValidateClaudeRequest(ctx)
		require.NoError(t, err)
		require.NotNil(t, req.MaxTokens)
		require.EqualValues(t, 8192, *req.MaxTokens)
	})

	t.Run("Gemini maxOutputTokens 超上限被拒绝", func(t *testing.T) {
		ctx := newJSONContext(t, `{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":`+hugeN+`}}`)
		_, err := GetAndValidateGeminiRequest(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxOutputTokens is invalid")
	})

	t.Run("Gemini snake_case max_output_tokens 超上限被拒绝", func(t *testing.T) {
		ctx := newJSONContext(t, `{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"max_output_tokens":`+hugeN+`}}`)
		_, err := GetAndValidateGeminiRequest(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "maxOutputTokens is invalid")
	})

	t.Run("Gemini 正常 maxOutputTokens 通过", func(t *testing.T) {
		ctx := newJSONContext(t, `{"contents":[{"parts":[{"text":"hi"}]}],"generationConfig":{"maxOutputTokens":8192}}`)
		req, err := GetAndValidateGeminiRequest(ctx)
		require.NoError(t, err)
		require.NotNil(t, req.GenerationConfig.MaxOutputTokens)
		require.EqualValues(t, 8192, *req.GenerationConfig.MaxOutputTokens)
	})

	t.Run("Responses max_output_tokens 超上限被拒绝", func(t *testing.T) {
		ctx := newJSONContext(t, `{"model":"gpt-4o","input":"hi","max_output_tokens":`+hugeN+`}`)
		_, err := GetAndValidateResponsesRequest(ctx)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max_output_tokens is invalid")
	})

	t.Run("Responses 正常 max_output_tokens 通过", func(t *testing.T) {
		ctx := newJSONContext(t, `{"model":"gpt-4o","input":"hi","max_output_tokens":8192}`)
		req, err := GetAndValidateResponsesRequest(ctx)
		require.NoError(t, err)
		require.NotNil(t, req.MaxOutputTokens)
		require.EqualValues(t, 8192, *req.MaxOutputTokens)
	})
}
