// openai - openai_videos_handlers.go
// 提供 OpenAI 兼容的视频生成 API 处理器，支持 xAI (Grok) 视频生成服务。
// 包含视频创建、检索、编辑和扩展等操作的请求构建、参数验证和响应转换逻辑。
// 支持通过 OpenAI 标准路径（/v1/videos）和 xAI 原生路径访问视频生成能力。
package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// 视频生成 API 路径和默认配置常量
const (
	// videosPath 是 OpenAI 兼容的视频 API 基础路径
	videosPath              = "/v1/videos"
	// xaiVideosGenerationsAPI 是 xAI 视频生成的原生 API 路径
	xaiVideosGenerationsAPI = "/v1/videos/generations"
	// xaiVideosEditsAPI 是 xAI 视频编辑的原生 API 路径
	xaiVideosEditsAPI       = "/v1/videos/edits"
	// xaiVideosExtensionsAPI 是 xAI 视频扩展的原生 API 路径
	xaiVideosExtensionsAPI  = "/v1/videos/extensions"
	// defaultXAIVideosModel 是 xAI 视频生成的默认模型名称
	defaultXAIVideosModel   = "grok-imagine-video"
	// xaiVideosHandlerType 是视频处理器类型的标识符，用于路由和认证管理
	xaiVideosHandlerType    = "openai-video"
	// defaultVideosSeconds 是视频默认时长（秒）
	defaultVideosSeconds    = "4"
	// defaultVideosSize 是视频默认分辨率尺寸
	defaultVideosSize       = "720x1280"
	// defaultVideosResolution 是视频默认分辨率标签
	defaultVideosResolution = "720p"
	// maxXAIVideoReferences 是 xAI 视频生成支持的最大参考图片数量
	maxXAIVideoReferences   = 7
)

// xaiVideoCreateMetadata 存储 xAI 视频创建请求的元数据信息，
// 用于构建标准化的 API 响应。
type xaiVideoCreateMetadata struct {
	// Model 是视频生成使用的模型名称
	Model     string
	// Prompt 是视频生成的文本提示词
	Prompt    string
	// Seconds 是请求的视频时长（秒）
	Seconds   string
	// Size 是视频的分辨率尺寸（如 "720x1280"）
	Size      string
	// CreatedAt 是请求创建的 Unix 时间戳
	CreatedAt int64
}

// videosModelBase 从模型名称中提取基础模型名称（去除前缀）。
// 例如 "xai/grok-imagine-video" 返回 "grok-imagine-video"。
func videosModelBase(model string) string {
	_, baseModel := imagesModelParts(model)
	return strings.ToLower(strings.TrimSpace(baseModel))
}

// isXAIVideosModel 检查给定模型名称是否为 xAI 视频生成模型。
// 支持的前缀包括 "xai"、"x-ai"、"grok" 或无前缀。
func isXAIVideosModel(model string) bool {
	prefix, baseModel := imagesModelParts(model)
	baseModel = strings.ToLower(strings.TrimSpace(baseModel))
	if baseModel != defaultXAIVideosModel {
		return false
	}

	prefix = strings.ToLower(strings.TrimSpace(prefix))
	return prefix == "" || prefix == "xai" || prefix == "x-ai" || prefix == "grok"
}

// isSupportedVideosModel 检查给定模型是否为支持的视频生成模型。
// 当前仅支持 xAI 视频模型。
func isSupportedVideosModel(model string) bool {
	return isXAIVideosModel(model)
}

// rejectUnsupportedVideosModel 检查模型是否受支持，如果不支持则返回 400 错误响应。
// 返回 true 表示模型不受支持（已发送错误响应），false 表示模型受支持。
func rejectUnsupportedVideosModel(c *gin.Context, model string) bool {
	if isSupportedVideosModel(model) {
		return false
	}

	c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
		Error: handlers.ErrorDetail{
			Message: fmt.Sprintf("Model %s is not supported on %s. Use %s.", model, videosPath, defaultXAIVideosModel),
			Type:    "invalid_request_error",
		},
	})
	return true
}

// rejectUnsupportedNativeVideosModel 检查原生 xAI 视频 API 的模型是否受支持。
// 与 rejectUnsupportedVideosModel 类似，但错误消息包含所有原生 API 路径。
func rejectUnsupportedNativeVideosModel(c *gin.Context, model string) bool {
	if isSupportedVideosModel(model) {
		return false
	}

	c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
		Error: handlers.ErrorDetail{
			Message: fmt.Sprintf("Model %s is not supported on %s, %s, or %s. Use %s.", model, xaiVideosGenerationsAPI, xaiVideosEditsAPI, xaiVideosExtensionsAPI, defaultXAIVideosModel),
			Type:    "invalid_request_error",
		},
	})
	return true
}

// canonicalXAIVideosModel 将模型名称规范化为 xAI 视频生成的标准模型名称。
// 如果基础模型匹配默认模型，则返回默认模型名称。
func canonicalXAIVideosModel(model string) string {
	if videosModelBase(model) == defaultXAIVideosModel {
		return defaultXAIVideosModel
	}
	return defaultXAIVideosModel
}

// readVideosCreateRequest 读取视频创建请求，支持 JSON 和 multipart/form-data 格式。
// 根据 Content-Type 自动选择解析方式。
func readVideosCreateRequest(c *gin.Context) ([]byte, error) {
	contentType := strings.ToLower(strings.TrimSpace(c.ContentType()))
	switch contentType {
	case "multipart/form-data", "application/x-www-form-urlencoded":
		return videosCreateRequestFromForm(c)
	default:
		rawJSON, err := handlers.ReadRequestBody(c)
		if err != nil {
			return nil, err
		}
		if !json.Valid(rawJSON) {
			return nil, fmt.Errorf("body must be valid JSON")
		}
		return rawJSON, nil
	}
}

// readXAIVideosNativeRequest 读取 xAI 原生视频 API 请求（仅支持 JSON 格式）。
func readXAIVideosNativeRequest(c *gin.Context) ([]byte, error) {
	rawJSON, err := handlers.ReadRequestBody(c)
	if err != nil {
		return nil, err
	}
	if !json.Valid(rawJSON) {
		return nil, fmt.Errorf("body must be valid JSON")
	}
	return rawJSON, nil
}

// videosCreateRequestFromForm 从 multipart/form-data 表单中提取视频创建参数。
// 支持的字段包括 model、prompt、seconds、size、aspect_ratio、resolution、
// input_reference[image_url]、input_reference[file_id] 和 reference_image_urls。
func videosCreateRequestFromForm(c *gin.Context) ([]byte, error) {
	rawJSON := []byte(`{}`)
	for _, field := range []string{"model", "prompt", "seconds", "size", "aspect_ratio", "resolution"} {
		if value := strings.TrimSpace(c.PostForm(field)); value != "" {
			rawJSON, _ = sjson.SetBytes(rawJSON, field, value)
		}
	}
	if value := strings.TrimSpace(firstPostForm(c, "input_reference[image_url]", "input_reference.image_url", "image_url")); value != "" {
		rawJSON, _ = sjson.SetBytes(rawJSON, "input_reference.image_url", value)
	}
	if value := strings.TrimSpace(firstPostForm(c, "input_reference[file_id]", "input_reference.file_id", "file_id")); value != "" {
		rawJSON, _ = sjson.SetBytes(rawJSON, "input_reference.file_id", value)
	}
	if refs := strings.TrimSpace(c.PostForm("reference_image_urls")); refs != "" {
		for _, ref := range strings.Split(refs, ",") {
			if ref = strings.TrimSpace(ref); ref != "" {
				rawJSON, _ = sjson.SetBytes(rawJSON, "reference_image_urls.-1", ref)
			}
		}
	}
	return rawJSON, nil
}

// firstPostForm 从多个可能的表单字段名中获取第一个非空值。
// 用于处理不同客户端可能使用不同字段名的情况。
func firstPostForm(c *gin.Context, keys ...string) string {
	for _, key := range keys {
		if value := c.PostForm(key); strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// buildXAIVideosCreateRequest 构建 xAI 视频生成的请求体。
// 处理 prompt、duration、aspect_ratio、resolution、image 和 reference_images 等参数。
// 返回构建好的请求体、元数据和可能的错误。
func buildXAIVideosCreateRequest(rawJSON []byte, model string) ([]byte, xaiVideoCreateMetadata, error) {
	prompt := strings.TrimSpace(gjson.GetBytes(rawJSON, "prompt").String())
	if prompt == "" {
		return nil, xaiVideoCreateMetadata{}, fmt.Errorf("prompt is required")
	}

	seconds, duration, err := normalizeXAIVideosSeconds(gjson.GetBytes(rawJSON, "seconds").String())
	if err != nil {
		return nil, xaiVideoCreateMetadata{}, err
	}

	size, aspectRatio, resolution, err := xaiVideosSizeOptions(gjson.GetBytes(rawJSON, "size").String())
	if err != nil {
		return nil, xaiVideoCreateMetadata{}, err
	}
	if value := xaiVideosAspectRatio(gjson.GetBytes(rawJSON, "aspect_ratio").String(), ""); value != "" {
		aspectRatio = value
	}
	if value := xaiVideosResolution(gjson.GetBytes(rawJSON, "resolution").String(), ""); value != "" {
		resolution = value
	}

	imageURL, err := xaiVideosInputImageURL(rawJSON)
	if err != nil {
		return nil, xaiVideoCreateMetadata{}, err
	}
	referenceImages := collectXAIVideoReferenceImages(rawJSON)
	if len(referenceImages) > maxXAIVideoReferences {
		return nil, xaiVideoCreateMetadata{}, fmt.Errorf("reference_images supports at most %d images on xAI", maxXAIVideoReferences)
	}
	if imageURL != "" && len(referenceImages) > 0 {
		return nil, xaiVideoCreateMetadata{}, fmt.Errorf("image and reference_images cannot be combined on xAI")
	}
	if len(referenceImages) > 0 && duration > 10 {
		duration = 10
		seconds = "10"
	}

	req := []byte(`{}`)
	req, _ = sjson.SetBytes(req, "model", canonicalXAIVideosModel(model))
	req, _ = sjson.SetBytes(req, "prompt", prompt)
	req, _ = sjson.SetRawBytes(req, "duration", []byte(strconv.FormatInt(duration, 10)))
	req, _ = sjson.SetBytes(req, "aspect_ratio", aspectRatio)
	req, _ = sjson.SetBytes(req, "resolution", resolution)
	if imageURL != "" {
		req, _ = sjson.SetBytes(req, "image.url", imageURL)
	}
	for _, image := range referenceImages {
		req, _ = sjson.SetBytes(req, "reference_images.-1.url", image)
	}

	meta := xaiVideoCreateMetadata{
		Model:     defaultXAIVideosModel,
		Prompt:    prompt,
		Seconds:   seconds,
		Size:      size,
		CreatedAt: time.Now().Unix(),
	}
	return req, meta, nil
}

// normalizeXAIVideosSeconds 规范化视频时长参数。
// 默认值为 4 秒，有效范围为 1-15 秒。
// 返回规范化后的字符串表示和整数值。
func normalizeXAIVideosSeconds(raw string) (string, int64, error) {
	seconds := strings.TrimSpace(raw)
	if seconds == "" {
		seconds = defaultVideosSeconds
	}
	duration, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return "", 0, fmt.Errorf("seconds must be an integer")
	}
	if duration < 1 {
		duration = 1
	}
	if duration > 15 {
		duration = 15
	}
	return strconv.FormatInt(duration, 10), duration, nil
}

// xaiVideosSizeOptions 解析视频尺寸参数，返回尺寸、宽高比和分辨率。
// 支持的尺寸包括 720x1280、1280x720、1024x1792 和 1792x1024。
func xaiVideosSizeOptions(raw string) (size string, aspectRatio string, resolution string, err error) {
	size = strings.TrimSpace(raw)
	if size == "" {
		size = defaultVideosSize
	}
	switch size {
	case "720x1280", "1024x1792":
		return size, "9:16", defaultVideosResolution, nil
	case "1280x720", "1792x1024":
		return size, "16:9", defaultVideosResolution, nil
	default:
		return "", "", "", fmt.Errorf("size must be one of 720x1280, 1280x720, 1024x1792, or 1792x1024")
	}
}

// xaiVideosAspectRatio 解析宽高比参数，支持多种格式（如 "1:1"、"square"、"landscape" 等）。
// 如果参数无效则返回 fallback 值。
func xaiVideosAspectRatio(raw string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1:1", "square":
		return "1:1"
	case "16:9", "landscape":
		return "16:9"
	case "9:16", "portrait":
		return "9:16"
	case "4:3":
		return "4:3"
	case "3:4":
		return "3:4"
	case "3:2":
		return "3:2"
	case "2:3":
		return "2:3"
	default:
		return fallback
	}
}

// xaiVideosResolution 解析视频分辨率参数。
// 支持 "480p" 和 "720p" 两种分辨率。
// 如果参数无效则返回 fallback 值。
func xaiVideosResolution(raw string, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "480p":
		return "480p"
	case "720p":
		return "720p"
	default:
		return fallback
	}
}

// xaiVideosInputImageURL 从请求体中提取输入图片 URL。
// 支持多种输入格式：input_reference.image_url、input_reference.file_id、
// image.url、image_url 等。file_id 格式不被支持。
func xaiVideosInputImageURL(rawJSON []byte) (string, error) {
	inputRef := gjson.GetBytes(rawJSON, "input_reference")
	if inputRef.Exists() {
		imageURL := strings.TrimSpace(inputRef.Get("image_url").String())
		fileID := strings.TrimSpace(inputRef.Get("file_id").String())
		if imageURL != "" && fileID != "" {
			return "", fmt.Errorf("input_reference must provide exactly one of image_url or file_id")
		}
		if fileID != "" {
			return "", fmt.Errorf("input_reference.file_id is not supported for xAI video generation; use input_reference.image_url")
		}
		if imageURL != "" {
			return imageURL, nil
		}
	}

	image := gjson.GetBytes(rawJSON, "image")
	if image.Exists() {
		if image.Type == gjson.String {
			return strings.TrimSpace(image.String()), nil
		}
		if value := strings.TrimSpace(image.Get("url").String()); value != "" {
			return value, nil
		}
		if value := strings.TrimSpace(image.Get("image_url.url").String()); value != "" {
			return value, nil
		}
	}

	return strings.TrimSpace(gjson.GetBytes(rawJSON, "image_url").String()), nil
}

// collectXAIVideoReferenceImages 从请求体中收集参考图片 URL 列表。
// 支持 reference_images 和 reference_image_urls 两个字段。
// 每个元素可以是字符串或包含 url/image_url.url 属性的对象。
func collectXAIVideoReferenceImages(rawJSON []byte) []string {
	out := make([]string, 0)
	appendRef := func(value string) {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	collectArray := func(result gjson.Result) {
		if !result.IsArray() {
			return
		}
		result.ForEach(func(_, item gjson.Result) bool {
			if item.Type == gjson.String {
				appendRef(item.String())
				return true
			}
			if value := item.Get("url").String(); value != "" {
				appendRef(value)
				return true
			}
			if value := item.Get("image_url.url").String(); value != "" {
				appendRef(value)
			}
			return true
		})
	}
	collectArray(gjson.GetBytes(rawJSON, "reference_images"))
	collectArray(gjson.GetBytes(rawJSON, "reference_image_urls"))
	return out
}

// buildVideosCreateAPIResponseFromXAI 将 xAI 视频创建响应转换为 OpenAI 兼容格式。
// 提取 request_id、status、progress 等字段，构建标准化的视频对象响应。
func buildVideosCreateAPIResponseFromXAI(payload []byte, meta xaiVideoCreateMetadata) ([]byte, error) {
	requestID := strings.TrimSpace(gjson.GetBytes(payload, "request_id").String())
	if requestID == "" {
		requestID = strings.TrimSpace(gjson.GetBytes(payload, "id").String())
	}
	if requestID == "" {
		return nil, fmt.Errorf("xAI video response did not include request_id")
	}

	out := []byte(`{"object":"video","progress":0,"status":"queued"}`)
	out, _ = sjson.SetBytes(out, "id", requestID)
	out, _ = sjson.SetBytes(out, "model", meta.Model)
	out, _ = sjson.SetBytes(out, "prompt", meta.Prompt)
	out, _ = sjson.SetBytes(out, "seconds", meta.Seconds)
	out, _ = sjson.SetBytes(out, "size", meta.Size)
	out, _ = sjson.SetBytes(out, "created_at", meta.CreatedAt)
	if status := openAIVideoStatus(gjson.GetBytes(payload, "status").String()); status != "" {
		out, _ = sjson.SetBytes(out, "status", status)
	}
	if progress := gjson.GetBytes(payload, "progress"); progress.Exists() {
		out, _ = sjson.SetRawBytes(out, "progress", []byte(progress.Raw))
	}
	return out, nil
}

// buildVideosRetrieveAPIResponseFromXAI 将 xAI 视频检索响应转换为 OpenAI 兼容格式。
// 提取 video 对象、status、progress、usage 和 error 等字段。
func buildVideosRetrieveAPIResponseFromXAI(videoID string, payload []byte, fallbackModel string) ([]byte, error) {
	out := []byte(`{"object":"video"}`)
	out, _ = sjson.SetBytes(out, "id", videoID)

	model := strings.TrimSpace(gjson.GetBytes(payload, "model").String())
	if model == "" {
		model = fallbackModel
	}
	out, _ = sjson.SetBytes(out, "model", model)

	if status := openAIVideoStatus(gjson.GetBytes(payload, "status").String()); status != "" {
		out, _ = sjson.SetBytes(out, "status", status)
	}
	if progress := gjson.GetBytes(payload, "progress"); progress.Exists() {
		out, _ = sjson.SetRawBytes(out, "progress", []byte(progress.Raw))
	}
	if duration := gjson.GetBytes(payload, "video.duration"); duration.Exists() {
		out, _ = sjson.SetBytes(out, "seconds", duration.String())
	}
	if video := gjson.GetBytes(payload, "video"); video.Exists() && json.Valid([]byte(video.Raw)) {
		out, _ = sjson.SetRawBytes(out, "video", []byte(video.Raw))
	}
	if usage := gjson.GetBytes(payload, "usage"); usage.Exists() && json.Valid([]byte(usage.Raw)) {
		out, _ = sjson.SetRawBytes(out, "usage", []byte(usage.Raw))
	}
	if errPayload := gjson.GetBytes(payload, "error"); errPayload.Exists() && json.Valid([]byte(errPayload.Raw)) {
		out, _ = sjson.SetRawBytes(out, "error", []byte(errPayload.Raw))
	}
	return out, nil
}

// openAIVideoStatus 将各种视频状态字符串规范化为 OpenAI 标准状态。
// 支持的状态映射：
// - queued/pending -> queued
// - in_progress/processing/running -> in_progress
// - completed/done/succeeded/success -> completed
// - failed/error/expired/cancelled/canceled -> failed
func openAIVideoStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "queued", "pending":
		return "queued"
	case "in_progress", "processing", "running":
		return "in_progress"
	case "completed", "done", "succeeded", "success":
		return "completed"
	case "failed", "error", "expired", "cancelled", "canceled":
		return "failed"
	default:
		return ""
	}
}

// VideosCreate 处理 /v1/videos 路径的视频创建请求。
// 解析请求体，验证模型，构建 xAI 请求并转发到上游服务。
func (h *OpenAIAPIHandler) VideosCreate(c *gin.Context) {
	rawJSON, err := readVideosCreateRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: fmt.Sprintf("Invalid request: %v", err),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	videoModel := strings.TrimSpace(gjson.GetBytes(rawJSON, "model").String())
	if videoModel == "" {
		videoModel = defaultXAIVideosModel
	}
	if rejectUnsupportedVideosModel(c, videoModel) {
		return
	}

	xaiReq, meta, err := buildXAIVideosCreateRequest(rawJSON, videoModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: fmt.Sprintf("Invalid request: %v", err),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	h.collectXAIVideosCreate(c, xaiReq, meta)
}

// XAIVideosGenerations 处理 /v1/videos/generations 路径的 xAI 原生视频生成请求。
func (h *OpenAIAPIHandler) XAIVideosGenerations(c *gin.Context) {
	h.handleXAIVideosNativePost(c)
}

// XAIVideosEdits 处理 /v1/videos/edits 路径的 xAI 原生视频编辑请求。
func (h *OpenAIAPIHandler) XAIVideosEdits(c *gin.Context) {
	h.handleXAIVideosNativePost(c)
}

// XAIVideosExtensions 处理 /v1/videos/extensions 路径的 xAI 原生视频扩展请求。
func (h *OpenAIAPIHandler) XAIVideosExtensions(c *gin.Context) {
	h.handleXAIVideosNativePost(c)
}

// handleXAIVideosNativePost 处理 xAI 原生视频 API 的 POST 请求。
// 解析请求体，验证模型，然后转发到上游服务。
func (h *OpenAIAPIHandler) handleXAIVideosNativePost(c *gin.Context) {
	rawJSON, err := readXAIVideosNativeRequest(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: fmt.Sprintf("Invalid request: %v", err),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	videoModel := strings.TrimSpace(gjson.GetBytes(rawJSON, "model").String())
	if videoModel == "" {
		videoModel = defaultXAIVideosModel
	}
	if rejectUnsupportedNativeVideosModel(c, videoModel) {
		return
	}

	h.collectXAIVideosNative(c, rawJSON, videoModel)
}

// XAIVideosRetrieve 处理 xAI 原生视频检索请求。
// 从路径参数中提取 request_id 并转发到上游服务。
func (h *OpenAIAPIHandler) XAIVideosRetrieve(c *gin.Context) {
	requestID := strings.TrimSpace(c.Param("request_id"))
	if requestID == "" {
		requestID = strings.TrimSpace(c.Param("video_id"))
	}
	if requestID == "" {
		c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: "Invalid request: request_id is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	payload := []byte(`{}`)
	payload, _ = sjson.SetBytes(payload, "request_id", requestID)
	h.collectXAIVideosNative(c, payload, defaultXAIVideosModel)
}

// VideosRetrieve 处理 /v1/videos/{video_id} 路径的视频检索请求。
// 将 OpenAI 格式的 video_id 转换为 xAI 格式的 request_id 并转发。
func (h *OpenAIAPIHandler) VideosRetrieve(c *gin.Context) {
	videoID := strings.TrimSpace(c.Param("video_id"))
	if videoID == "" {
		c.JSON(http.StatusBadRequest, handlers.ErrorResponse{
			Error: handlers.ErrorDetail{
				Message: "Invalid request: video_id is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	payload := []byte(`{}`)
	payload, _ = sjson.SetBytes(payload, "request_id", videoID)

	c.Header("Content-Type", "application/json")
	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	stopKeepAlive := h.StartNonStreamingKeepAlive(c, cliCtx)
	resp, upstreamHeaders, errMsg := h.ExecuteWithAuthManager(cliCtx, xaiVideosHandlerType, defaultXAIVideosModel, payload, "")
	stopKeepAlive()
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		if errMsg.Error != nil {
			cliCancel(errMsg.Error)
		} else {
			cliCancel(nil)
		}
		return
	}

	out, err := buildVideosRetrieveAPIResponseFromXAI(videoID, resp, defaultXAIVideosModel)
	if err != nil {
		errMsg := &interfaces.ErrorMessage{StatusCode: http.StatusBadGateway, Error: err}
		h.WriteErrorResponse(c, errMsg)
		cliCancel(err)
		return
	}

	handlers.WriteUpstreamHeaders(c.Writer.Header(), upstreamHeaders)
	_, _ = c.Writer.Write(out)
	cliCancel(nil)
}

// collectXAIVideosNative 转发 xAI 原生视频请求到上游服务并返回响应。
// 设置 JSON 响应头，创建上下文，启动 keep-alive，执行请求并写入响应。
func (h *OpenAIAPIHandler) collectXAIVideosNative(c *gin.Context, rawJSON []byte, model string) {
	c.Header("Content-Type", "application/json")

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	stopKeepAlive := h.StartNonStreamingKeepAlive(c, cliCtx)
	resp, upstreamHeaders, errMsg := h.ExecuteWithAuthManager(cliCtx, xaiVideosHandlerType, model, rawJSON, "")
	stopKeepAlive()
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		if errMsg.Error != nil {
			cliCancel(errMsg.Error)
		} else {
			cliCancel(nil)
		}
		return
	}

	handlers.WriteUpstreamHeaders(c.Writer.Header(), upstreamHeaders)
	_, _ = c.Writer.Write(resp)
	cliCancel(nil)
}

// collectXAIVideosCreate 转发 xAI 视频创建请求到上游服务并将响应转换为 OpenAI 兼容格式。
// 设置 JSON 响应头，创建上下文，启动 keep-alive，执行请求并构建标准化响应。
func (h *OpenAIAPIHandler) collectXAIVideosCreate(c *gin.Context, xaiReq []byte, meta xaiVideoCreateMetadata) {
	c.Header("Content-Type", "application/json")

	cliCtx, cliCancel := h.GetContextWithCancel(h, c, context.Background())
	stopKeepAlive := h.StartNonStreamingKeepAlive(c, cliCtx)
	resp, upstreamHeaders, errMsg := h.ExecuteWithAuthManager(cliCtx, xaiVideosHandlerType, meta.Model, xaiReq, "")
	stopKeepAlive()
	if errMsg != nil {
		h.WriteErrorResponse(c, errMsg)
		if errMsg.Error != nil {
			cliCancel(errMsg.Error)
		} else {
			cliCancel(nil)
		}
		return
	}

	out, err := buildVideosCreateAPIResponseFromXAI(resp, meta)
	if err != nil {
		errMsg := &interfaces.ErrorMessage{StatusCode: http.StatusBadGateway, Error: err}
		h.WriteErrorResponse(c, errMsg)
		cliCancel(err)
		return
	}

	handlers.WriteUpstreamHeaders(c.Writer.Header(), upstreamHeaders)
	_, _ = c.Writer.Write(out)
	cliCancel(nil)
}
