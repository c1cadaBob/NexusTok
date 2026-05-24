// Package openai 的 Responses 压缩响应处理文件。
// 负责处理 OpenAI Responses API 的压缩（compaction）响应。
// 压缩响应用于长对话场景，将之前的对话历史压缩为更小的表示。
package openai

import (
	"io"
	"net/http"

	// 项目内部包
	"github.com/c1cada/NexusTok/common"                              // 通用工具（JSON 等）
	"github.com/c1cada/NexusTok/dto"                                 // 数据传输对象
	"github.com/c1cada/NexusTok/service"                             // 服务层
	"github.com/c1cada/NexusTok/types"                               // 类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin"                                       // Gin Web 框架
)

// OaiResponsesCompactionHandler 处理 Responses API 的压缩响应。
// 压缩响应用于长对话场景，将之前的对话历史压缩为更小的表示。
// 处理流程：
// 1. 读取并解析压缩响应
// 2. 检查错误响应
// 3. 原样返回响应体
// 4. 提取使用量统计
// 参数:
//   - c: Gin 上下文
//   - resp: 上游 HTTP 响应
// 返回:
//   - *dto.Usage: 使用量统计
//   - *types.NexusTokError: 错误信息（成功时为 nil）
func OaiResponsesCompactionHandler(c *gin.Context, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	defer service.CloseResponseBodyGracefully(resp)

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}

	// 解析压缩响应
	var compactResp dto.OpenAIResponsesCompactionResponse
	if err := common.Unmarshal(responseBody, &compactResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 检查上游错误
	if oaiError := compactResp.GetOpenAIError(); oaiError != nil && oaiError.Type != "" {
		return nil, types.WithOpenAIError(*oaiError, resp.StatusCode)
	}

	// 原样返回响应体
	service.IOCopyBytesGracefully(c, resp, responseBody)

	// 提取使用量统计
	usage := dto.Usage{}
	if compactResp.Usage != nil {
		usage.PromptTokens = compactResp.Usage.InputTokens
		usage.CompletionTokens = compactResp.Usage.OutputTokens
		usage.TotalTokens = compactResp.Usage.TotalTokens
		// 提取缓存 token 信息
		if compactResp.Usage.InputTokensDetails != nil {
			usage.PromptTokensDetails.CachedTokens = compactResp.Usage.InputTokensDetails.CachedTokens
		}
	}

	return &usage, nil
}
