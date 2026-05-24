// Package ali 实现阿里云通义千问渠道的重排序（Rerank）功能。
// 该文件负责将 OpenAI 格式的重排序请求转换为阿里云格式，并处理阿里云的重排序响应。
package ali

import (
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
)

// ConvertRerankRequest 将通用的重排序请求转换为阿里云重排序 API 的请求格式。
// 如果客户端未指定 returnDocuments 参数，默认设置为 true 以返回文档内容。
//
// 参数:
//   - request: 通用的重排序请求，包含查询文本、文档列表和排序参数
//
// 返回值:
//   - *AliRerankRequest: 转换后的阿里云重排序请求
func ConvertRerankRequest(request dto.RerankRequest) *AliRerankRequest {
	returnDocuments := request.ReturnDocuments
	if returnDocuments == nil {
		t := true
		returnDocuments = &t
	}
	return &AliRerankRequest{
		Model: request.Model,
		Input: AliRerankInput{
			Query:     request.Query,
			Documents: request.Documents,
		},
		Parameters: AliRerankParameters{
			TopN:            request.TopN,
			ReturnDocuments: returnDocuments,
		},
	}
}

// RerankHandler 处理阿里云重排序 API 的响应。
// 读取上游响应体，检查是否包含错误信息，将阿里云格式的重排序结果转换为通用格式后写入客户端。
//
// 参数:
//   - c: gin 请求上下文，用于写入响应
//   - resp: 阿里云上游 API 的 HTTP 响应
//   - info: 中继信息，包含请求元数据
//
// 返回值:
//   - *types.NexusTokError: 如果处理出错则返回错误信息
//   - *dto.Usage: 本次请求的 token 使用量统计
func RerankHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NexusTokError, *dto.Usage) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}
	service.CloseResponseBodyGracefully(resp)

	var aliResponse AliRerankResponse
	err = common.Unmarshal(responseBody, &aliResponse)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}

	if aliResponse.Code != "" {
		return types.WithOpenAIError(types.OpenAIError{
			Message: aliResponse.Message,
			Type:    aliResponse.Code,
			Param:   aliResponse.RequestId,
			Code:    aliResponse.Code,
		}, resp.StatusCode), nil
	}

	usage := dto.Usage{
		PromptTokens:     aliResponse.Usage.TotalTokens,
		CompletionTokens: 0,
		TotalTokens:      aliResponse.Usage.TotalTokens,
	}
	rerankResponse := dto.RerankResponse{
		Results: aliResponse.Output.Results,
		Usage:   usage,
	}

	jsonResponse, err := common.Marshal(rerankResponse)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	c.Writer.Write(jsonResponse)
	return nil, &usage
}
