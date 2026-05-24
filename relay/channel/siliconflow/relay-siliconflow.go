// Package siliconflow 的 Rerank 响应处理文件。
// 负责处理 SiliconFlow Rerank API 的响应，转换为统一格式返回给客户端。
package siliconflow

import (
	"io"
	"net/http"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	// 第三方依赖
	"github.com/gin-gonic/gin"
)

// siliconflowRerankHandler 处理 SiliconFlow Rerank API 的响应。
// 读取上游响应体，解析为 SFRerankResponse 格式，
// 提取 token 使用量信息，转换为统一的 RerankResponse 格式返回。
// 参数:
//   - c: Gin 上下文，用于写入响应
//   - info: 中继请求的上下文信息
//   - resp: 上游 SiliconFlow API 的 HTTP 响应
//
// 返回:
//   - *dto.Usage: token 使用量统计
//   - *types.NexusTokError: 处理过程中的错误信息
func siliconflowRerankHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)
	var siliconflowResp SFRerankResponse
	err = common.Unmarshal(responseBody, &siliconflowResp)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	usage := &dto.Usage{
		PromptTokens:     siliconflowResp.Meta.Tokens.InputTokens,
		CompletionTokens: siliconflowResp.Meta.Tokens.OutputTokens,
		TotalTokens:      siliconflowResp.Meta.Tokens.InputTokens + siliconflowResp.Meta.Tokens.OutputTokens,
	}
	rerankResp := &dto.RerankResponse{
		Results: siliconflowResp.Results,
		Usage:   *usage,
	}

	jsonResponse, err := common.Marshal(rerankResp)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return usage, nil
}
