// MiniMax 图片生成处理文件。
// 负责将 OpenAI 格式的图片请求转换为 MiniMax 格式，
// 并将 MiniMax 的图片生成响应转换为 OpenAI 兼容格式。
// 支持宽高比转换、响应格式标准化和 GCD 算法计算宽高比。
package minimax

// 标准库导入
import (
	"fmt"       // 格式化字符串
	"io"        // IO 读写接口
	"net/http"  // HTTP 响应处理
	"strconv"   // 字符串与数字转换
	"strings"   // 字符串操作

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"                 // 公共工具函数（JSON 处理等）
	"github.com/c1cada/NexusTok/dto"                    // 数据传输对象定义
	relaycommon "github.com/c1cada/NexusTok/relay/common"  // Relay 通用模块
	"github.com/c1cada/NexusTok/service"               // 服务层工具函数
	"github.com/c1cada/NexusTok/types"                 // 公共类型定义

	// 第三方依赖
	"github.com/gin-gonic/gin" // Gin Web 框架
)

// MiniMaxImageRequest MiniMax 图片生成 API 的请求结构体。
// 包含模型名称、提示词、宽高比、响应格式、数量等参数。
type MiniMaxImageRequest struct {
	Model           string `json:"model"`
	Prompt          string `json:"prompt"`
	AspectRatio     string `json:"aspect_ratio,omitempty"`
	ResponseFormat  string `json:"response_format,omitempty"`
	N               int    `json:"n,omitempty"`
	PromptOptimizer *bool  `json:"prompt_optimizer,omitempty"`
	AigcWatermark   *bool  `json:"aigc_watermark,omitempty"`
}

// MiniMaxImageResponse MiniMax 图片生成 API 的响应结构体。
// 包含图片 URL 列表、Base64 编码数据和状态信息。
type MiniMaxImageResponse struct {
	ID   string `json:"id"`
	Data struct {
		ImageURLs   []string `json:"image_urls"`
		ImageBase64 []string `json:"image_base64"`
	} `json:"data"`
	Metadata map[string]any `json:"metadata"`
	BaseResp struct {
		StatusCode int    `json:"status_code"`
		StatusMsg  string `json:"status_msg"`
	} `json:"base_resp"`
}

// oaiImage2MiniMaxImageRequest 将 OpenAI 格式的图片请求转换为 MiniMax 格式。
// 处理逻辑：
// 1. 标准化响应格式（url/b64_json -> url/base64）
// 2. 设置默认模型为 image-01
// 3. 转换图片数量
// 4. 从 size 或 extra 参数中解析宽高比
// 5. 处理 prompt_optimizer 可选参数
// 参数:
//   - request: OpenAI 格式的图片请求
// 返回:
//   - MiniMaxImageRequest: MiniMax 格式的图片请求
func oaiImage2MiniMaxImageRequest(request dto.ImageRequest) MiniMaxImageRequest {
	responseFormat := normalizeMiniMaxResponseFormat(request.ResponseFormat)
	minimaxRequest := MiniMaxImageRequest{
		Model:          request.Model,
		Prompt:         request.Prompt,
		ResponseFormat: responseFormat,
		N:              1,
		AigcWatermark:  request.Watermark,
	}

	if request.Model == "" {
		minimaxRequest.Model = "image-01"
	}
	if request.N != nil && *request.N > 0 {
		minimaxRequest.N = int(*request.N)
	}
	if aspectRatio := aspectRatioFromImageRequest(request); aspectRatio != "" {
		minimaxRequest.AspectRatio = aspectRatio
	}
	if raw, ok := request.Extra["prompt_optimizer"]; ok {
		var promptOptimizer bool
		if err := common.Unmarshal(raw, &promptOptimizer); err == nil {
			minimaxRequest.PromptOptimizer = &promptOptimizer
		}
	}

	return minimaxRequest
}

// aspectRatioFromImageRequest 从图片请求中提取宽高比。
// 优先从 extra 参数中读取 aspect_ratio，否则根据 size 字符串映射。
// 支持的标准尺寸映射：
//   - 1024x1024 -> 1:1
//   - 1792x1024 -> 16:9
//   - 1024x1792 -> 9:16
//   - 1536x1024, 1248x832 -> 3:2
//   - 1024x1536, 832x1248 -> 2:3
//   - 1152x864 -> 4:3
//   - 864x1152 -> 3:4
//   - 1344x576 -> 21:9
//   - 其他: 通过 GCD 算法计算并标准化
//
// 参数:
//   - request: OpenAI 格式的图片请求
// 返回:
//   - string: 宽高比字符串，无法解析时返回空字符串
func aspectRatioFromImageRequest(request dto.ImageRequest) string {
	if raw, ok := request.Extra["aspect_ratio"]; ok {
		var aspectRatio string
		if err := common.Unmarshal(raw, &aspectRatio); err == nil && aspectRatio != "" {
			return aspectRatio
		}
	}

	switch request.Size {
	case "1024x1024":
		return "1:1"
	case "1792x1024":
		return "16:9"
	case "1024x1792":
		return "9:16"
	case "1536x1024", "1248x832":
		return "3:2"
	case "1024x1536", "832x1248":
		return "2:3"
	case "1152x864":
		return "4:3"
	case "864x1152":
		return "3:4"
	case "1344x576":
		return "21:9"
	}

	width, height, ok := parseImageSize(request.Size)
	if !ok {
		return ""
	}
	ratio := reduceAspectRatio(width, height)
	switch ratio {
	case "1:1", "16:9", "4:3", "3:2", "2:3", "3:4", "9:16", "21:9":
		return ratio
	default:
		return ""
	}
}

// parseImageSize 解析图片尺寸字符串（如 "1024x1024"）为宽和高。
// 参数:
//   - size: 尺寸字符串，格式为 "宽x高"
// 返回:
//   - int: 宽度
//   - int: 高度
//   - bool: 是否解析成功
func parseImageSize(size string) (int, int, bool) {
	parts := strings.Split(size, "x")
	if len(parts) != 2 {
		return 0, 0, false
	}
	width, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	height, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	if width <= 0 || height <= 0 {
		return 0, 0, false
	}
	return width, height, true
}

// reduceAspectRatio 使用最大公约数（GCD）将宽高比化简为最简整数比。
// 例如 1536x1024 -> "3:2"。
// 参数:
//   - width: 宽度
//   - height: 高度
// 返回:
//   - string: 化简后的宽高比字符串
func reduceAspectRatio(width, height int) string {
	divisor := gcd(width, height)
	return fmt.Sprintf("%d:%d", width/divisor, height/divisor)
}

// gcd 计算两个整数的最大公约数（Greatest Common Divisor）。
// 使用辗转相除法（欧几里得算法）实现。
// 参数:
//   - a: 第一个整数
//   - b: 第二个整数
// 返回:
//   - int: 最大公约数；当 a 为 0 时返回 1 避免除零
func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a == 0 {
		return 1
	}
	return a
}

// normalizeMiniMaxResponseFormat 标准化 MiniMax 的响应格式。
// 将 OpenAI 格式的响应格式标识转换为 MiniMax 格式：
//   - "", "url" -> "url"
//   - "b64_json", "base64" -> "base64"
//
// 参数:
//   - responseFormat: 原始响应格式字符串
// 返回:
//   - string: 标准化后的响应格式
func normalizeMiniMaxResponseFormat(responseFormat string) string {
	switch strings.ToLower(responseFormat) {
	case "", "url":
		return "url"
	case "b64_json", "base64":
		return "base64"
	default:
		return responseFormat
	}
}

// responseMiniMax2OpenAIImage 将 MiniMax 图片响应转换为 OpenAI 格式。
// 遍历 MiniMax 返回的图片 URL 和 Base64 数据，转换为 OpenAI ImageData 格式。
// 同时处理 metadata 字段的序列化。
// 参数:
//   - response: MiniMax 图片生成响应
//   - info: Relay 信息，用于获取请求开始时间
// 返回:
//   - *dto.ImageResponse: OpenAI 格式的图片响应
//   - error: metadata 序列化错误
func responseMiniMax2OpenAIImage(response *MiniMaxImageResponse, info *relaycommon.RelayInfo) (*dto.ImageResponse, error) {
	imageResponse := &dto.ImageResponse{
		Created: info.StartTime.Unix(),
	}

	for _, imageURL := range response.Data.ImageURLs {
		imageResponse.Data = append(imageResponse.Data, dto.ImageData{Url: imageURL})
	}
	for _, imageBase64 := range response.Data.ImageBase64 {
		imageResponse.Data = append(imageResponse.Data, dto.ImageData{B64Json: imageBase64})
	}
	if len(response.Metadata) > 0 {
		metadata, err := common.Marshal(response.Metadata)
		if err != nil {
			return nil, err
		}
		imageResponse.Metadata = metadata
	}

	return imageResponse, nil
}

// miniMaxImageHandler 处理 MiniMax 图片生成的 HTTP 响应。
// 流程：读取响应体 -> 解析 JSON -> 检查错误状态码 -> 转换为 OpenAI 格式 -> 写入响应。
// MiniMax 的 base_resp.status_code 非 0 时表示错误。
// 参数:
//   - c: Gin 上下文，用于写入响应
//   - resp: MiniMax API 返回的 HTTP 响应
//   - info: Relay 信息
// 返回:
//   - *dto.Usage: token 使用量（图片生成无实际 token 消耗，返回空）
//   - *types.NexusTokError: 错误信息
func miniMaxImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NexusTokError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	var minimaxResponse MiniMaxImageResponse
	if err := common.Unmarshal(responseBody, &minimaxResponse); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}
	if minimaxResponse.BaseResp.StatusCode != 0 {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: minimaxResponse.BaseResp.StatusMsg,
			Type:    "minimax_image_error",
			Code:    fmt.Sprintf("%d", minimaxResponse.BaseResp.StatusCode),
		}, resp.StatusCode)
	}

	openAIResponse, err := responseMiniMax2OpenAIImage(&minimaxResponse, info)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}
	jsonResponse, err := common.Marshal(openAIResponse)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	c.Writer.WriteHeader(resp.StatusCode)
	if _, err := c.Writer.Write(jsonResponse); err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	return &dto.Usage{}, nil
}
