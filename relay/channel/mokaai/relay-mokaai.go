// Moka AI Embedding 请求和响应处理文件。
// 负责将 OpenAI 格式的 Embedding 请求转换为 Moka AI 格式，
// 并将 Moka AI 的 Embedding 响应转换为 OpenAI 兼容格式。
package mokaai

// 标准库导入
import (
	"encoding/json" // JSON 序列化/反序列化
	"io"            // IO 读写接口
	"net/http"      // HTTP 响应处理

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"                 // 公共工具函数（JSON 处理等）
	"github.com/c1cada/NexusTok/dto"                    // 数据传输对象定义
	relaycommon "github.com/c1cada/NexusTok/relay/common"  // Relay 通用模块
	"github.com/c1cada/NexusTok/service"               // 服务层工具函数
	"github.com/c1cada/NexusTok/types"                 // 公共类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// embeddingRequestOpenAI2Moka 将 OpenAI 格式的 Embedding 请求转换为 Moka AI 格式。
// 主要处理 input 字段的类型转换：
// - string -> []string: 单个字符串转为字符串切片
// - []string -> []string: 直接使用
// - []interface{} -> []string: 遍历提取字符串元素
// 参数:
//   - request: OpenAI 格式的通用请求对象
// 返回:
//   - *dto.EmbeddingRequest: Moka AI 格式的 Embedding 请求
func embeddingRequestOpenAI2Moka(request dto.GeneralOpenAIRequest) *dto.EmbeddingRequest {
	var input []string // Change input to []string

	switch v := request.Input.(type) {
	case string:
		input = []string{v} // Convert string to []string
	case []string:
		input = v // Already a []string, no conversion needed
	case []interface{}:
		for _, part := range v {
			if str, ok := part.(string); ok {
				input = append(input, str) // Append each string to the slice
			}
		}
	}
	return &dto.EmbeddingRequest{
		Input: input,
		Model: request.Model,
	}
}

// embeddingResponseMoka2OpenAI 将 Moka AI 的 Embedding 响应转换为 OpenAI 格式。
// 将 Moka AI 返回的嵌入数据列表转换为 OpenAI 兼容的响应结构。
// 参数:
//   - response: Moka AI 格式的 Embedding 响应
// 返回:
//   - *dto.OpenAIEmbeddingResponse: OpenAI 格式的 Embedding 响应
func embeddingResponseMoka2OpenAI(response *dto.EmbeddingResponse) *dto.OpenAIEmbeddingResponse {
	openAIEmbeddingResponse := dto.OpenAIEmbeddingResponse{
		Object: "list",
		Data:   make([]dto.OpenAIEmbeddingResponseItem, 0, len(response.Data)),
		Model:  "baidu-embedding",
		Usage:  response.Usage,
	}
	for _, item := range response.Data {
		openAIEmbeddingResponse.Data = append(openAIEmbeddingResponse.Data, dto.OpenAIEmbeddingResponseItem{
			Object:    item.Object,
			Index:     item.Index,
			Embedding: item.Embedding,
		})
	}
	return &openAIEmbeddingResponse
}

// mokaEmbeddingHandler 处理 Moka AI Embedding API 的 HTTP 响应。
// 流程：读取响应体 -> 解析 JSON -> 转换为 OpenAI 格式 -> 写入响应。
// 参数:
//   - c: Gin 上下文，用于写入响应
//   - info: Relay 信息
//   - resp: Moka AI API 返回的 HTTP 响应
// 返回:
//   - *dto.Usage: token 使用量信息
//   - *types.NexusTokError: 错误信息
func mokaEmbeddingHandler(c *gin.Context, info *relaycommon.RelayInfo, resp *http.Response) (*dto.Usage, *types.NexusTokError) {
	var baiduResponse dto.EmbeddingResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	service.CloseResponseBodyGracefully(resp)
	err = json.Unmarshal(responseBody, &baiduResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	// if baiduResponse.ErrorMsg != "" {
	// 	return &dto.OpenAIErrorWithStatusCode{
	// 		Error: dto.OpenAIError{
	// 			Type:    "baidu_error",
	// 			Param:   "",
	// 		},
	// 		StatusCode: resp.StatusCode,
	// 	}, nil
	// }
	fullTextResponse := embeddingResponseMoka2OpenAI(&baiduResponse)
	jsonResponse, err := common.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	service.IOCopyBytesGracefully(c, resp, jsonResponse)
	return &fullTextResponse.Usage, nil
}
