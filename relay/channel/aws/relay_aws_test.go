// Package aws 实现 Amazon Bedrock 渠道的中继适配器
// 本文件为 AWS Bedrock 客户端请求的单元测试
package aws

import (
	"bytes"              // 字节缓冲区
	"net/http"           // HTTP 客户端
	"net/http/httptest"  // HTTP 测试工具
	"testing"            // 测试框架

	"github.com/c1cada/NexusTok/common"               // 公共工具包
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继通用类型
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime" // AWS Bedrock 运行时 SDK
	"github.com/gin-gonic/gin"                            // Gin Web 框架
	"github.com/stretchr/testify/require"                 // 测试断言
)

// TestDoAwsClientRequest_AppliesRuntimeHeaderOverrideToAnthropicBeta 测试运行时头覆盖应用到 Anthropic Beta
// 验证当设置 RuntimeHeadersOverride 时，anthropic-beta 头能正确注入到 Bedrock 请求体中
// 这是 Claude 模型的 Computer Use 功能所需的 beta 特性头
func TestDoAwsClientRequest_AppliesRuntimeHeaderOverrideToAnthropicBeta(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/v1/messages", nil)

	// 配置中继信息，启用运行时头覆盖
	info := &relaycommon.RelayInfo{
		OriginModelName:           "claude-3-5-sonnet-20240620", // 模型名称
		IsStream:                  false,                        // 非流式请求
		UseRuntimeHeadersOverride: true,                         // 启用运行时头覆盖
		RuntimeHeadersOverride: map[string]any{
			"anthropic-beta": "computer-use-2025-01-24", // Computer Use beta 特性
		},
		ChannelMeta: &relaycommon.ChannelMeta{
			ApiKey:            "access-key|secret-key|us-east-1", // AWS 凭证格式：AccessKey|SecretKey|Region
			UpstreamModelName: "claude-3-5-sonnet-20240620",      // 上游模型名称
		},
	}

	// 构建请求体（Claude 消息格式）
	requestBody := bytes.NewBufferString(`{"messages":[{"role":"user","content":"hello"}],"max_tokens":128}`)
	adaptor := &Adaptor{}

	// 执行 AWS 客户端请求
	_, err := doAwsClientRequest(ctx, info, adaptor, requestBody)
	require.NoError(t, err)

	// 验证 AWS 请求类型为 InvokeModelInput
	awsReq, ok := adaptor.AwsReq.(*bedrockruntime.InvokeModelInput)
	require.True(t, ok)

	// 解析请求体中的 JSON
	var payload map[string]any
	require.NoError(t, common.Unmarshal(awsReq.Body, &payload))

	// 验证 anthropic_beta 字段已正确注入
	anthropicBeta, exists := payload["anthropic_beta"]
	require.True(t, exists)

	// 验证 anthropic_beta 值为数组且包含正确的 beta 特性
	values, ok := anthropicBeta.([]any)
	require.True(t, ok)
	require.Equal(t, []any{"computer-use-2025-01-24"}, values)
}
