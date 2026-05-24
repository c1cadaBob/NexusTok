// 即梦 AI 图片生成响应处理文件。
// 负责将即梦 API 的图片生成响应转换为 OpenAI 兼容的格式。
package jimeng

// 标准库导入
import (
	"encoding/json" // JSON 序列化/反序列化
	"fmt"           // 格式化字符串
	"io"            // IO 读写接口
	"net/http"      // HTTP 响应处理

	// 项目内部依赖
	"github.com/c1cada/NexusTok/dto"                    // 数据传输对象定义
	relaycommon "github.com/c1cada/NexusTok/relay/common"  // Relay 通用模块
	"github.com/c1cada/NexusTok/service"               // 服务层工具函数
	"github.com/c1cada/NexusTok/types"                 // 公共类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// ImageResponse 即梦图片生成 API 的响应结构体。
// 包含返回码、消息、图片数据（Base64 和 URL）、请求 ID 等信息。
type ImageResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    struct {
		BinaryDataBase64 []string `json:"binary_data_base64"`
		ImageUrls        []string `json:"image_urls"`
		RephraseResult   string   `json:"rephraser_result"`
		RequestID        string   `json:"request_id"`
		// Other fields are omitted for brevity
	} `json:"data"`
	RequestID   string `json:"request_id"`
	Status      int    `json:"status"`
	TimeElapsed string `json:"time_elapsed"`
}

// responseJimeng2OpenAIImage 将即梦图片响应转换为 OpenAI 格式。
// 遍历即梦返回的 Base64 图片数据和图片 URL，统一转换为 OpenAI ImageData 格式。
// 参数:
//   - _: Gin 上下文（未使用）
//   - response: 即梦图片生成响应
//   - info: Relay 信息，用于获取请求开始时间
// 返回:
//   - *dto.ImageResponse: OpenAI 格式的图片响应
func responseJimeng2OpenAIImage(_ *gin.Context, response *ImageResponse, info *relaycommon.RelayInfo) *dto.ImageResponse {
	imageResponse := dto.ImageResponse{
		Created: info.StartTime.Unix(),
	}

	for _, base64Data := range response.Data.BinaryDataBase64 {
		imageResponse.Data = append(imageResponse.Data, dto.ImageData{
			B64Json: base64Data,
		})
	}
	for _, imageUrl := range response.Data.ImageUrls {
		imageResponse.Data = append(imageResponse.Data, dto.ImageData{
			Url: imageUrl,
		})
	}

	return &imageResponse
}

// jimengImageHandler 处理即梦图片生成的 HTTP 响应。
// 流程：读取响应体 -> 解析 JSON -> 检查错误码 -> 转换为 OpenAI 格式 -> 写入响应。
// 即梦 API 成功时 code 为 10000，其他值表示错误。
// 参数:
//   - c: Gin 上下文，用于写入响应
//   - resp: 即梦 API 返回的 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - *dto.Usage: token 使用量（图片生成无实际 token 消耗，返回空）
//   - *types.NexusTokError: 错误信息
func jimengImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NexusTokError) {
	var jimengResponse ImageResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	err = json.Unmarshal(responseBody, &jimengResponse)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// Check if the response indicates an error
	if jimengResponse.Code != 10000 {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: jimengResponse.Message,
			Type:    "jimeng_error",
			Param:   "",
			Code:    fmt.Sprintf("%d", jimengResponse.Code),
		}, resp.StatusCode)
	}

	// Convert Jimeng response to OpenAI format
	fullTextResponse := responseJimeng2OpenAIImage(c, &jimengResponse, info)
	jsonResponse, err := json.Marshal(fullTextResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	_, err = c.Writer.Write(jsonResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	return &dto.Usage{}, nil
}
