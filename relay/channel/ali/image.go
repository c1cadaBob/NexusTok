// ali - image.go
// 阿里云 DashScope 图像生成处理逻辑文件
// 本文件实现了阿里云图像生成请求的转换、异步任务轮询、Multipart 表单图片上传解析
// 以及阿里云图像响应到 OpenAI 兼容格式的转换等功能。
// 支持同步和异步两种图像生成模式，并提供了图像编辑（Image Edit）的表单处理能力。
package ali

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/types"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// oaiImage2AliImageRequest 将 OpenAI 格式的图像生成请求（dto.ImageRequest）转换为阿里云 DashScope 格式（AliImageRequest）。
// 参数：
//   - info: 中继信息，包含渠道配置、定价数据等上下文
//   - request: OpenAI 格式的图像生成请求
//   - isSync: 是否为同步图像生成模式（同步与异步的请求格式不同）
//
// 转换逻辑：
//  1. 设置模型名称和响应格式
//  2. 优先从 request.Extra["parameters"] 解析阿里云专用参数；若不存在，则从 OpenAI 标准字段（Size、N、Watermark）映射
//  3. 支持从 request.Extra["input"] 直接注入阿里云专用输入结构
//  4. 对于 z-image 模型，如果开启了 PromptExtend，添加 2 倍计费比率
//  5. 根据图片生成数量 N 添加对应计费比率
//  6. 同步模式使用 Messages 格式（图文混排），异步模式使用 Prompt 格式（纯文本）
func oaiImage2AliImageRequest(info *relaycommon.RelayInfo, request dto.ImageRequest, isSync bool) (*AliImageRequest, error) {
	var imageRequest AliImageRequest
	imageRequest.Model = request.Model
	imageRequest.ResponseFormat = request.ResponseFormat
	if request.Extra != nil {
		// 优先尝试从 Extra 中解析阿里云专用参数
		if val, ok := request.Extra["parameters"]; ok {
			err := common.Unmarshal(val, &imageRequest.Parameters)
			if err != nil {
				return nil, fmt.Errorf("invalid parameters field: %w", err)
			}
		} else {
			// 兼容没有parameters字段的情况，从openai标准字段中提取参数
			// 将 OpenAI 的 "宽x高" 格式转换为阿里云的 "宽*高" 格式
			imageRequest.Parameters = AliImageParameters{
				Size:      strings.Replace(request.Size, "x", "*", -1),
				N:         int(lo.FromPtrOr(request.N, uint(1))), // 默认生成 1 张
				Watermark: request.Watermark,
			}
		}
		// 支持从 Extra 中直接注入阿里云专用的输入结构
		if val, ok := request.Extra["input"]; ok {
			err := common.Unmarshal(val, &imageRequest.Input)
			if err != nil {
				return nil, fmt.Errorf("invalid input field: %w", err)
			}
		}
	}

	// z-image 模型：开启 prompt_extend 后，按 2 倍计费
	if strings.Contains(request.Model, "z-image") {
		if imageRequest.Parameters.PromptExtendValue() {
			info.PriceData.AddOtherRatio("prompt_extend", 2)
		}
	}

	// 根据请求生成的图片数量添加计费倍率
	if imageRequest.Parameters.N != 0 {
		info.PriceData.AddOtherRatio("n", float64(imageRequest.Parameters.N))
	}

	// 同步图片模型和异步图片模型请求格式不一样
	if isSync {
		// 同步模式：使用 Messages 格式，将 Prompt 包装为用户消息
		if imageRequest.Input == nil {
			imageRequest.Input = AliImageInput{
				Messages: []AliMessage{
					{
						Role: "user",
						Content: []AliMediaContent{
							{
								Text: request.Prompt,
							},
						},
					},
				},
			}
		}
	} else {
		// 异步模式：直接使用 Prompt 字段
		if imageRequest.Input == nil {
			imageRequest.Input = AliImageInput{
				Prompt: request.Prompt,
			}
		}
	}

	return &imageRequest, nil
}

// getImageBase64sFromForm 从 Multipart 表单请求中提取图片文件并转换为 Base64 编码的 data URL 列表。
// 参数：
//   - c: Gin 上下文，包含 Multipart 表单数据
//   - fieldName: 表单字段名（当前实现兼容多种字段名格式，该参数保留但未严格使用）
//
// 返回值：Base64 data URL 字符串列表（格式为 "data:<mime>;base64,<data>"）和错误信息。
//
// 该函数兼容以下三种表单字段命名方式：
//  1. "image" — 标准单图字段
//  2. "image[]" — 数组形式的多图字段
//  3. "image[0]"、"image[1]" 等 — 带索引的数组形式
func getImageBase64sFromForm(c *gin.Context, fieldName string) ([]string, error) {
	mf := c.Request.MultipartForm
	if mf == nil {
		// 如果尚未解析 Multipart 表单，先进行解析
		if _, err := c.MultipartForm(); err != nil {
			return nil, fmt.Errorf("failed to parse image edit form request: %w", err)
		}
		mf = c.Request.MultipartForm
	}

	var imageFiles []*multipart.FileHeader
	var exists bool

	// 首先检查标准的 "image" 字段
	if imageFiles, exists = mf.File["image"]; !exists || len(imageFiles) == 0 {
		// 如果未找到，检查 "image[]" 字段
		if imageFiles, exists = mf.File["image[]"]; !exists || len(imageFiles) == 0 {
			// 如果仍未找到，遍历所有字段查找以 "image[" 开头的字段名
			foundArrayImages := false
			for fieldName, files := range mf.File {
				if strings.HasPrefix(fieldName, "image[") && len(files) > 0 {
					foundArrayImages = true
					imageFiles = append(imageFiles, files...)
				}
			}

			// 如果完全没有找到任何图片字段
			if !foundArrayImages && (len(imageFiles) == 0) {
				return nil, errors.New("image is required")
			}
		}
	}

	if len(imageFiles) == 0 {
		return nil, errors.New("image is required")
	}

	// 将每个图片文件转换为 Base64 编码的 data URL
	var imageBase64s []string
	for _, file := range imageFiles {
		// 打开上传的图片文件
		image, err := file.Open()
		if err != nil {
			return nil, errors.New("failed to open image file")
		}

		// 读取文件全部内容
		imageData, err := io.ReadAll(image)
		if err != nil {
			return nil, errors.New("failed to read image file")
		}

		// 自动检测图片的 MIME 类型（如 image/png、image/jpeg）
		mimeType := http.DetectContentType(imageData)

		// 将图片数据编码为 Base64 字符串
		base64Data := base64.StdEncoding.EncodeToString(imageData)

		// 构造 data URL 格式（"data:<mime>;base64,<data>"），阿里云接口可直接识别
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64Data)
		imageBase64s = append(imageBase64s, dataURL)
		image.Close()
	}
	return imageBase64s, nil
}

// oaiFormEdit2AliImageEdit 将 OpenAI 格式的图像编辑表单请求转换为阿里云 DashScope 格式（AliImageRequest）。
// 参数：
//   - c: Gin 上下文，包含 Multipart 表单数据（图片文件）
//   - info: 中继信息，包含渠道配置等上下文
//   - request: OpenAI 格式的图像请求（包含 Prompt 和可选参数）
//
// 转换逻辑：
//  1. 从表单中提取图片文件并转为 Base64 data URL
//  2. 构造图文混排的 Messages 格式：先添加图片内容块，再添加文本提示词
//  3. 从 OpenAI 标准字段映射参数（N、Watermark）
func oaiFormEdit2AliImageEdit(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (*AliImageRequest, error) {
	var imageRequest AliImageRequest
	imageRequest.Model = request.Model
	imageRequest.ResponseFormat = request.ResponseFormat

	// 从表单中获取上传图片的 Base64 编码
	imageBase64s, err := getImageBase64sFromForm(c, "image")
	if err != nil {
		return nil, fmt.Errorf("get image base64s from form failed: %w", err)
	}
	// 将每张图片构造为 AliMediaContent 图片内容块
	mediaContents := make([]AliMediaContent, len(imageBase64s))
	for i, b64 := range imageBase64s {
		mediaContents[i] = AliMediaContent{
			Image: b64,
		}
	}
	// 在图片内容块之后追加文本提示词
	mediaContents = append(mediaContents, AliMediaContent{
		Text: request.Prompt,
	})
	// 构造 Messages 格式的输入，阿里云接口支持图文混排
	imageRequest.Input = AliImageInput{
		Messages: []AliMessage{
			{
				Role:    "user",
				Content: mediaContents,
			},
		},
	}
	// 映射 OpenAI 标准参数
	imageRequest.Parameters = AliImageParameters{
		N:         int(lo.FromPtrOr(request.N, uint(1))), // 默认生成 1 张
		Watermark: request.Watermark,
	}
	return &imageRequest, nil
}

// updateTask 向阿里云 DashScope 异步任务接口发起查询请求，获取指定任务的当前状态和结果。
// 参数：
//   - info: 中继信息，包含渠道基础 URL 和 API 密钥
//   - taskID: 异步任务的唯一标识
//
// 返回值：阿里云响应结构体、错误信息和原始响应字节。
//
// 该函数通过 HTTP GET 请求查询任务状态，使用 Bearer Token 认证。
func updateTask(info *relaycommon.RelayInfo, taskID string) (*AliResponse, error, []byte) {
	// 拼接任务查询 URL
	url := fmt.Sprintf("%s/api/v1/tasks/%s", info.ChannelBaseUrl, taskID)

	var aliResponse AliResponse

	// 构造 HTTP GET 请求
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return &aliResponse, err, nil
	}

	// 设置 Bearer Token 认证头
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)

	// 发送请求
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		common.SysLog("updateTask client.Do err: " + err.Error())
		return &aliResponse, err, nil
	}
	defer resp.Body.Close()

	// 读取响应体
	responseBody, err := io.ReadAll(resp.Body)

	// 解析响应 JSON
	var response AliResponse
	err = common.Unmarshal(responseBody, &response)
	if err != nil {
		common.SysLog("updateTask NewDecoder err: " + err.Error())
		return &aliResponse, err, nil
	}

	return &response, nil, responseBody
}

// asyncTaskWait 轮询等待阿里云异步图像生成任务完成。
// 参数：
//   - c: Gin 上下文，用于日志记录
//   - info: 中继信息，包含渠道配置和 API 密钥
//   - taskID: 异步任务的唯一标识
//
// 返回值：最终的阿里云响应、原始响应字节和错误信息。
//
// 轮询策略：
//  1. 初始等待 5 秒后开始首次查询
//  2. 每次查询间隔 10 秒
//  3. 最多轮询 20 次（约 200 秒超时）
//  4. 当任务状态为 FAILED、CANCELED、SUCCEEDED 或 UNKNOWN 时立即返回
//  5. 如果 TaskStatus 为空，视为非异步任务，直接返回
func asyncTaskWait(c *gin.Context, info *relaycommon.RelayInfo, taskID string) (*AliResponse, []byte, error) {
	waitSeconds := 10    // 每次轮询间隔（秒）
	step := 0            // 当前轮询次数
	maxStep := 20        // 最大轮询次数

	var taskResponse AliResponse
	var responseBody []byte

	// 初始等待 5 秒，给异步任务一定的处理时间
	time.Sleep(time.Duration(5) * time.Second)

	for {
		logger.LogDebug(c, fmt.Sprintf("asyncTaskWait step %d/%d, wait %d seconds", step, maxStep, waitSeconds))
		step++
		// 查询任务状态
		rsp, err, body := updateTask(info, taskID)
		responseBody = body
		if err != nil {
			// 查询失败时记录警告并继续重试
			logger.LogWarn(c, "asyncTaskWait UpdateTask err: "+err.Error())
			time.Sleep(time.Duration(waitSeconds) * time.Second)
			continue
		}

		// 如果 TaskStatus 为空，说明响应不是异步任务格式（可能是同步模型），直接返回
		if rsp.Output.TaskStatus == "" {
			return &taskResponse, responseBody, nil
		}

		// 根据任务状态决定是否继续轮询
		switch rsp.Output.TaskStatus {
		case "FAILED":    // 任务失败
			fallthrough
		case "CANCELED":  // 任务已取消
			fallthrough
		case "SUCCEEDED": // 任务成功完成
			fallthrough
		case "UNKNOWN":   // 未知状态
			return rsp, responseBody, nil
		}
		// 达到最大轮询次数，退出循环
		if step >= maxStep {
			break
		}
		// 等待指定间隔后继续轮询
		time.Sleep(time.Duration(waitSeconds) * time.Second)
	}

	// 超时未完成
	return nil, nil, fmt.Errorf("aliAsyncTaskWait timeout")
}

// responseAli2OpenAIImage 将阿里云图像生成响应转换为 OpenAI 兼容的 ImageResponse 格式。
// 参数：
//   - c: Gin 上下文，用于日志记录
//   - response: 阿里云响应结构体
//   - originBody: 原始响应字节，作为 metadata 附加到转换后的响应中
//   - info: 中继信息，包含请求开始时间等上下文
//   - responseFormat: 响应格式，"b64_json" 表示需要 Base64 编码的图片
//
// 转换逻辑：
//   - 优先检查 Results 字段（异步任务结果），调用 ResultToOpenAIImageDate 转换
//   - 其次检查 Choices 字段（同步生成结果），调用 ChoicesToOpenAIImageDate 转换
func responseAli2OpenAIImage(c *gin.Context, response *AliResponse, originBody []byte, info *relaycommon.RelayInfo, responseFormat string) *dto.ImageResponse {
	imageResponse := dto.ImageResponse{
		Created: info.StartTime.Unix(), // 使用请求开始时间作为 Created 时间戳
	}

	// 优先处理异步任务的结果（Results 字段）
	if len(response.Output.Results) > 0 {
		imageResponse.Data = response.Output.ResultToOpenAIImageDate(c, responseFormat)
	} else if len(response.Output.Choices) > 0 {
		// 处理同步生成的结果（Choices 字段）
		imageResponse.Data = response.Output.ChoicesToOpenAIImageDate(c, responseFormat)
	}

	// 将原始阿里云响应作为 metadata 透传给客户端，便于调试和排查
	imageResponse.Metadata = originBody
	return &imageResponse
}

// aliImageHandler 是阿里云图像生成请求的主处理函数。
// 负责接收上游阿里云的图像生成响应，处理同步/异步两种模式，并将结果转换为 OpenAI 兼容格式返回给客户端。
// 参数：
//   - a: 阿里云适配器实例，包含 IsSyncImageModel 等配置
//   - c: Gin 上下文
//   - resp: 上游阿里云的 HTTP 响应
//   - info: 中继信息，包含渠道配置、定价数据等
//
// 返回值：NexusTok 错误对象和 Token 用量。
//
// 处理流程：
//  1. 读取并解析上游响应体为 AliResponse
//  2. 检查响应是否包含错误信息
//  3. 同步模式：直接使用响应数据
//  4. 异步模式：通过 asyncTaskWait 轮询等待任务完成
//  5. 检查任务最终状态（仅异步模式）
//  6. 将阿里云响应转换为 OpenAI 兼容格式
//  7. 根据实际生成的图片数量更新计费比率
//  8. 将转换后的 JSON 响应写回客户端
func aliImageHandler(a *Adaptor, c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (*types.NexusTokError, *dto.Usage) {
	// 从上下文获取客户端请求的响应格式
	responseFormat := c.GetString("response_format")

	// 读取上游响应体
	var aliTaskResponse AliResponse
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeReadResponseBodyFailed, http.StatusInternalServerError), nil
	}
	service.CloseResponseBodyGracefully(resp)
	// 解析响应 JSON
	err = common.Unmarshal(responseBody, &aliTaskResponse)
	if err != nil {
		return types.NewOpenAIError(err, types.ErrorCodeBadResponseBody, http.StatusInternalServerError), nil
	}

	// 检查响应级别的错误信息
	if aliTaskResponse.Message != "" {
		logger.LogError(c, "ali_async_task_failed: "+aliTaskResponse.Message)
		return types.NewError(errors.New(aliTaskResponse.Message), types.ErrorCodeBadResponse), nil
	}

	var (
		aliResponse    *AliResponse // 最终使用的阿里云响应
		originRespBody []byte       // 原始响应字节
	)

	if a.IsSyncImageModel {
		// 同步图像模型：直接使用初始响应
		aliResponse = &aliTaskResponse
		originRespBody = responseBody
	} else {
		// 异步图像模型：轮询等待任务完成
		aliResponse, originRespBody, err = asyncTaskWait(c, info, aliTaskResponse.Output.TaskId)
		if err != nil {
			return types.NewError(err, types.ErrorCodeBadResponse), nil
		}
		// 检查异步任务的最终状态
		if aliResponse.Output.TaskStatus != "SUCCEEDED" {
			return types.WithOpenAIError(types.OpenAIError{
				Message: aliResponse.Output.Message,
				Type:    "ali_error",
				Param:   "",
				Code:    aliResponse.Output.Code,
			}, resp.StatusCode), nil
		}
	}

	// 记录调试日志
	if a.IsSyncImageModel {
		logger.LogDebug(c, "ali_sync_image_result: "+string(originRespBody))
	} else {
		logger.LogDebug(c, "ali_async_image_result: "+string(originRespBody))
	}

	// 将阿里云响应转换为 OpenAI 兼容格式
	imageResponses := responseAli2OpenAIImage(c, aliResponse, originRespBody, info, responseFormat)

	// 根据实际生成的图片数量更新计费比率
	if aliResponse.Usage.ImageCount != 0 {
		// 优先使用阿里云返回的 ImageCount
		info.PriceData.AddOtherRatio("n", float64(aliResponse.Usage.ImageCount))
	} else if len(imageResponses.Data) != 0 {
		// 否则使用转换后的图片数据数量
		info.PriceData.AddOtherRatio("n", float64(len(imageResponses.Data)))
	}

	// 将转换后的响应序列化为 JSON
	jsonResponse, err := common.Marshal(imageResponses)
	if err != nil {
		return types.NewError(err, types.ErrorCodeBadResponseBody), nil
	}
	// 将 JSON 响应写回客户端
	service.IOCopyBytesGracefully(c, resp, jsonResponse)

	return nil, &dto.Usage{}
}
