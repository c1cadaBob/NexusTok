// Package channel 实现中继渠道的通用 API 请求处理
// 本文件为请求头覆盖（Header Override）功能的单元测试
package channel

import (
	"net/http"          // HTTP 客户端
	"net/http/httptest" // HTTP 测试工具
	"strings"
	"testing" // 测试框架

	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继通用类型
	"github.com/gin-gonic/gin"                            // Gin Web 框架
	"github.com/stretchr/testify/require"                 // 测试断言
)

// TestApplyUpstreamContentLengthSetsBodySize 验证 BodyStorage 包装后的上游请求会回填 ContentLength。
func TestApplyUpstreamContentLengthSetsBodySize(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", struct {
		*strings.Reader
	}{strings.NewReader(`{"model":"gpt-4.1"}`)})
	req.ContentLength = 0
	info := &relaycommon.RelayInfo{UpstreamRequestBodySize: 21}

	applyUpstreamContentLength(req, info)

	require.Equal(t, int64(21), req.ContentLength)
}

// TestApplyUpstreamContentLengthKeepsExistingLength 验证已有明确长度时不覆盖 net/http 的判断。
func TestApplyUpstreamContentLengthKeepsExistingLength(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", strings.NewReader("payload"))
	require.Equal(t, int64(len("payload")), req.ContentLength)

	applyUpstreamContentLength(req, &relaycommon.RelayInfo{UpstreamRequestBodySize: 99})

	require.Equal(t, int64(len("payload")), req.ContentLength)
}

// TestApplyUpstreamContentLengthIgnoresMissingSize 验证未声明转换后 body 大小时保持默认长度语义。
func TestApplyUpstreamContentLengthIgnoresMissingSize(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "https://example.com/v1/chat/completions", struct {
		*strings.Reader
	}{strings.NewReader("payload")})
	req.ContentLength = 0

	applyUpstreamContentLength(req, &relaycommon.RelayInfo{})

	require.Zero(t, req.ContentLength)
}

// TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules 测试渠道测试模式跳过透传规则
// 验证在渠道测试模式下，通配符 "*" 透传规则不会传递客户端请求头
func TestProcessHeaderOverride_ChannelTestSkipsPassthroughRules(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123") // 设置测试请求头

	// 配置渠道测试模式，使用通配符透传规则
	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "", // 通配符：透传所有客户端请求头
			},
		},
	}

	// 渠道测试模式下应跳过透传规则，返回空头映射
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Empty(t, headers)
}

// TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder 测试渠道测试模式跳过客户端头占位符
// 验证在渠道测试模式下，{client_header:...} 占位符不会被解析
func TestProcessHeaderOverride_ChannelTestSkipsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	// 配置渠道测试模式，使用客户端头占位符
	info := &relaycommon.RelayInfo{
		IsChannelTest: true,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}", // 占位符：从客户端请求头中提取值
			},
		},
	}

	// 渠道测试模式下应跳过占位符解析
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	_, ok := headers["x-upstream-trace"]
	require.False(t, ok) // 不应存在该头
}

// TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder 测试非测试模式保留客户端头占位符
// 验证在正常模式下，{client_header:...} 占位符会被正确解析为客户端请求头的值
func TestProcessHeaderOverride_NonTestKeepsClientHeaderPlaceholder(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")

	// 配置非测试模式，使用客户端头占位符
	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Upstream-Trace": "{client_header:X-Trace-Id}",
			},
		},
	}

	// 非测试模式下应正确解析占位符
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-upstream-trace"]) // 占位符应被替换为客户端请求头的值
}

// TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap 测试运行时覆盖为最终头映射
// 验证运行时头覆盖（RuntimeHeadersOverride）优先于渠道静态配置（HeadersOverride）
func TestProcessHeaderOverride_RuntimeOverrideIsFinalHeaderMap(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)

	// 配置运行时头覆盖和渠道静态头覆盖
	info := &relaycommon.RelayInfo{
		IsChannelTest:             false,
		UseRuntimeHeadersOverride: true, // 启用运行时头覆盖
		RuntimeHeadersOverride: map[string]any{
			"x-static":  "runtime-value", // 运行时覆盖的值
			"x-runtime": "runtime-only",  // 仅运行时存在的头
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value", // 渠道静态配置的值（应被运行时覆盖）
				"X-Legacy": "legacy-only",  // 仅渠道静态配置存在的头（不应出现）
			},
		},
	}

	// 运行时头覆盖应完全替代渠道静态配置
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "runtime-value", headers["x-static"]) // 运行时值覆盖静态值
	require.Equal(t, "runtime-only", headers["x-runtime"]) // 运行时独有头存在
	_, exists := headers["x-legacy"]
	require.False(t, exists) // 渠道静态独有头不应存在
}

// TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding 测试透传规则跳过 Accept-Encoding 头
// 验证通配符 "*" 透传规则不会传递 Accept-Encoding 头（避免影响响应压缩）
func TestProcessHeaderOverride_PassthroughSkipsAcceptEncoding(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	ctx.Request.Header.Set("X-Trace-Id", "trace-123")
	ctx.Request.Header.Set("Accept-Encoding", "gzip") // 设置 Accept-Encoding 头

	// 配置通配符透传规则
	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		ChannelMeta: &relaycommon.ChannelMeta{
			HeadersOverride: map[string]any{
				"*": "", // 通配符：透传所有客户端请求头
			},
		},
	}

	// 应透传普通头但跳过 Accept-Encoding
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "trace-123", headers["x-trace-id"]) // 普通头应被透传

	_, hasAcceptEncoding := headers["accept-encoding"]
	require.False(t, hasAcceptEncoding) // Accept-Encoding 不应被透传
}

// TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders 测试 pass_headers 模板设置运行时头
// 验证 pass_headers 操作模式能正确将客户端请求头传递到运行时头覆盖
func TestProcessHeaderOverride_PassHeadersTemplateSetsRuntimeHeaders(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/responses", nil)
	ctx.Request.Header.Set("Originator", "Codex CLI") // 设置 Codex CLI 来源头
	ctx.Request.Header.Set("Session_id", "sess-123")  // 设置会话 ID 头

	// 配置 pass_headers 操作模式和静态头覆盖
	info := &relaycommon.RelayInfo{
		IsChannelTest: false,
		RequestHeaders: map[string]string{
			"Originator": "Codex CLI",
			"Session_id": "sess-123",
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ParamOverride: map[string]any{
				"operations": []any{
					map[string]any{
						"mode":  "pass_headers",                                             // 操作模式：传递请求头
						"value": []any{"Originator", "Session_id", "X-Codex-Beta-Features"}, // 要传递的头列表
					},
				},
			},
			HeadersOverride: map[string]any{
				"X-Static": "legacy-value", // 静态头覆盖
			},
		},
	}

	// 应用参数覆盖，设置运行时头覆盖
	_, err := relaycommon.ApplyParamOverrideWithRelayInfo([]byte(`{"model":"gpt-4.1"}`), info)
	require.NoError(t, err)
	require.True(t, info.UseRuntimeHeadersOverride)                          // 运行时头覆盖应被启用
	require.Equal(t, "Codex CLI", info.RuntimeHeadersOverride["originator"]) // Originator 头应被传递
	require.Equal(t, "sess-123", info.RuntimeHeadersOverride["session_id"])  // Session_id 头应被传递
	_, exists := info.RuntimeHeadersOverride["x-codex-beta-features"]
	require.False(t, exists)                                                  // 不存在的头不应出现
	require.Equal(t, "legacy-value", info.RuntimeHeadersOverride["x-static"]) // 静态头应被合并

	// 验证 processHeaderOverride 能正确处理运行时头覆盖
	headers, err := processHeaderOverride(info, ctx)
	require.NoError(t, err)
	require.Equal(t, "Codex CLI", headers["originator"]) // 运行时头应正确输出
	require.Equal(t, "sess-123", headers["session_id"])
	_, exists = headers["x-codex-beta-features"]
	require.False(t, exists) // 不存在的头不应出现

	// 验证 applyHeaderOverrideToRequest 能正确将头应用到上游请求
	upstreamReq := httptest.NewRequest(http.MethodPost, "https://example.com/v1/responses", nil)
	applyHeaderOverrideToRequest(upstreamReq, headers)
	require.Equal(t, "Codex CLI", upstreamReq.Header.Get("Originator")) // 头应被应用到上游请求
	require.Equal(t, "sess-123", upstreamReq.Header.Get("Session_id"))
	require.Empty(t, upstreamReq.Header.Get("X-Codex-Beta-Features")) // 不存在的头不应出现
}
