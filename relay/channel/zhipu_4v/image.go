// 智谱 GLM-4V 图片生成 API 的处理逻辑。
// 负责将智谱的图片生成响应转换为 OpenAI 兼容格式。
// 智谱的图片生成 API 返回的图片可能是 URL 或 Base64 编码，
// 需要统一转换为 OpenAI 的 Base64 格式。
package zhipu_4v

import (
	"io"
	"net/http"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"              // 通用工具函数（JSON 序列化等）
	"github.com/c1cada/NexusTok/dto"                  // 数据传输对象
	"github.com/c1cada/NexusTok/logger"               // 日志工具
	relaycommon "github.com/c1cada/NexusTok/relay/common" // 中继层通用结构体
	"github.com/c1cada/NexusTok/service"               // 服务层工具
	"github.com/c1cada/NexusTok/types"                 // 错误类型

	// 第三方依赖
	"github.com/gin-gonic/gin" // HTTP 框架
)

// zhipuImageRequest 智谱图片生成请求结构体。
type zhipuImageRequest struct {
	Model            string `json:"model"`                       // 模型名称
	Prompt           string `json:"prompt"`                      // 图片描述提示词
	Quality          string `json:"quality,omitempty"`           // 图片质量
	Size             string `json:"size,omitempty"`              // 图片尺寸
	WatermarkEnabled *bool  `json:"watermark_enabled,omitempty"` // 是否启用水印
	UserID           string `json:"user_id,omitempty"`           // 用户 ID
}

// zhipuImageResponse 智谱图片生成响应结构体。
type zhipuImageResponse struct {
	Created       *int64            `json:"created,omitempty"`    // 创建时间戳
	Data          []zhipuImageData  `json:"data,omitempty"`       // 图片数据列表
	ContentFilter any               `json:"content_filter,omitempty"` // 内容过滤信息
	Usage         *dto.Usage        `json:"usage,omitempty"`      // token 使用量
	Error         *zhipuImageError  `json:"error,omitempty"`      // 错误信息
	RequestID     string            `json:"request_id,omitempty"` // 请求 ID
	ExtendParam   map[string]string `json:"extendParam,omitempty"` // 扩展参数
}

// zhipuImageError 智谱图片生成错误结构体。
type zhipuImageError struct {
	Code    string `json:"code"`    // 错误代码
	Message string `json:"message"` // 错误消息
}

// zhipuImageData 智谱图片数据结构体。
// 智谱可能返回 URL 或 Base64 编码的图片，字段名不固定。
type zhipuImageData struct {
	Url      string `json:"url,omitempty"`       // 图片 URL（字段名 1）
	ImageUrl string `json:"image_url,omitempty"` // 图片 URL（字段名 2）
	B64Json  string `json:"b64_json,omitempty"`  // Base64 编码图片（字段名 1）
	B64Image string `json:"b64_image,omitempty"` // Base64 编码图片（字段名 2）
}

// openAIImagePayload OpenAI 格式的图片响应结构体。
type openAIImagePayload struct {
	Created int64             `json:"created"` // 创建时间戳
	Data    []openAIImageData `json:"data"`    // 图片数据列表
}

// openAIImageData OpenAI 格式的单张图片数据。
type openAIImageData struct {
	B64Json string `json:"b64_json"` // Base64 编码的图片数据
}

// zhipu4vImageHandler 处理智谱 GLM-4V 的图片生成响应。
// 将智谱格式的图片响应转换为 OpenAI 兼容格式。
//
// 处理逻辑：
//  1. 读取并解析智谱的响应体
//  2. 检查是否有错误返回
//  3. 遍历每张图片，按优先级获取 Base64 数据：
//     a. 直接从 b64_json 或 b64_image 字段获取
//     b. 如果只有 URL，通过 HTTP 下载并转换为 Base64
//  4. 将所有图片组装为 OpenAI 格式的响应
//
// 参数：
//   - c: Gin 上下文
//   - resp: 智谱 API 的 HTTP 响应
//   - info: 中继信息
//
// 返回：token 使用量统计和可能的错误。
func zhipu4vImageHandler(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*dto.Usage, *types.NexusTokError) {
	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError)
	}
	service.CloseResponseBodyGracefully(resp)

	// 解析智谱响应
	var zhipuResp zhipuImageResponse
	if err := common.Unmarshal(responseBody, &zhipuResp); err != nil {
		return nil, types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError)
	}

	// 检查智谱返回的错误
	if zhipuResp.Error != nil && zhipuResp.Error.Message != "" {
		return nil, types.WithOpenAIError(types.OpenAIError{
			Message: zhipuResp.Error.Message,
			Type:    "zhipu_image_error",
			Code:    zhipuResp.Error.Code,
		}, resp.StatusCode)
	}

	// 构建 OpenAI 格式的响应
	payload := openAIImagePayload{}
	if zhipuResp.Created != nil && *zhipuResp.Created != 0 {
		payload.Created = *zhipuResp.Created
	} else {
		payload.Created = info.StartTime.Unix() // 使用请求开始时间作为默认值
	}

	// 遍历每张图片，转换为 OpenAI 格式
	for _, data := range zhipuResp.Data {
		// 尝试获取图片 URL（兼容不同的字段名）
		url := data.Url
		if url == "" {
			url = data.ImageUrl
		}
		if url == "" {
			logger.LogWarn(c, "zhipu_image_missing_url")
			continue
		}

		// 按优先级获取 Base64 数据
		var b64 string
		switch {
		case data.B64Json != "":
			b64 = data.B64Json // 优先使用 b64_json 字段
		case data.B64Image != "":
			b64 = data.B64Image // 其次使用 b64_image 字段
		default:
			// 如果没有 Base64 数据，通过 URL 下载图片
			_, downloaded, err := service.GetImageFromUrl(url)
			if err != nil {
				logger.LogError(c, "zhipu_image_get_b64_failed: "+err.Error())
				continue
			}
			b64 = downloaded
		}

		if b64 == "" {
			logger.LogWarn(c, "zhipu_image_empty_b64")
			continue
		}

		imageData := openAIImageData{
			B64Json: b64,
		}
		payload.Data = append(payload.Data, imageData)
	}

	// 序列化并返回 OpenAI 格式的响应
	jsonResp, err := common.Marshal(payload)
	if err != nil {
		return nil, types.NewError(err, types.ErrorCodeBadResponseBody)
	}

	service.IOCopyBytesGracefully(c, resp, jsonResp)

	return &dto.Usage{}, nil
}
