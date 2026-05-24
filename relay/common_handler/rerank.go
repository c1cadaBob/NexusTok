// Package common_handler 提供了各种 API 请求类型的通用处理函数。
// 本文件实现了 Rerank（重排序）请求的响应处理逻辑。
package common_handler

import (
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel/xinference"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// RerankHandler 处理重排序（Rerank）请求的上游响应。
// 读取上游响应体，解析为标准的 RerankResponse 格式，并处理不同渠道的响应差异。
// 特别地，Xinference 渠道的响应格式与标准 Jina 格式不同，需要额外的转换逻辑。
//
// 参数：
//   - c: Gin 请求上下文，用于写入响应头和 JSON 响应
//   - info: 中继信息，包含渠道类型、返回文档标志、预估 token 数等
//   - resp: 上游服务返回的 HTTP 响应
//
// 返回值：
//   - *dto.Usage: 使用量统计（token 计数）
//   - *types.NexusTokError: 处理过程中的错误，成功时为 nil
func RerankHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	// 读取完整的上游响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	// 优雅关闭响应体
	service.CloseResponseBodyGracefully(resp)
	// 调试模式下打印原始响应体
	if common.DebugEnabled {
		println("reranker response body: ", string(responseBody))
	}
	var jinaResp dto.RerankResponse
	// Xinference 渠道需要特殊处理：其响应格式与标准格式不同
	if info.ChannelType == constant.ChannelTypeXinference {
		var xinRerankResponse xinference.XinRerankResponse
		err = common.Unmarshal(responseBody, &xinRerankResponse)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		// 将 Xinference 格式转换为标准 Jina 格式
		jinaRespResults := make([]dto.RerankResponseResult, len(xinRerankResponse.Results))
		for i, result := range xinRerankResponse.Results {
			respResult := dto.RerankResponseResult{
				Index:          result.Index,
				RelevanceScore: result.RelevanceScore,
			}
			// 如果请求了返回文档，则处理文档内容
			if info.ReturnDocuments {
				var document any
				if result.Document != nil {
					if doc, ok := result.Document.(string); ok {
						// Xinference 可能返回空字符串文档，此时使用原始请求中的文档
						if doc == "" {
							document = info.Documents[result.Index]
						} else {
							document = doc
						}
					} else {
						document = result.Document
					}
				}
				respResult.Document = document
			}
			jinaRespResults[i] = respResult
		}
		// Xinference 不返回详细 token 信息，使用预估值
		jinaResp = dto.RerankResponse{
			Results: jinaRespResults,
			Usage: dto.Usage{
				PromptTokens: info.GetEstimatePromptTokens(),
				TotalTokens:  info.GetEstimatePromptTokens(),
			},
		}
	} else {
		// 标准渠道直接解析为 Jina 格式
		err = common.Unmarshal(responseBody, &jinaResp)
		if err != nil {
			return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
		}
		// 将 totalTokens 同步到 promptTokens（Rerank 场景下两者相同）
		jinaResp.Usage.PromptTokens = jinaResp.Usage.TotalTokens
	}

	// 设置响应头并返回 JSON 结果
	c.Writer.Header().Set("Content-Type", "application/json")
	c.JSON(http.StatusOK, jinaResp)
	return &jinaResp.Usage, nil
}
