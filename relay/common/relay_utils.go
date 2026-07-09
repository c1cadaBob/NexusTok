// Package common - relay_utils.go
// 本文件提供了中继层的通用工具函数，包括：
//   - HasPrompt / HasImage: 请求接口，用于抽象不同类型的请求。
//   - GetFullRequestURL: 构建完整的上游请求 URL，支持 Cloudflare AI Gateway 特殊处理。
//   - GetAPIVersion: 从请求中获取 API 版本号（主要用于 Azure 渠道）。
//   - 任务请求验证: ValidateBasicTaskRequest、ValidateMultipartDirect 等，用于验证异步任务提交请求。
package common

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

// HasPrompt 定义了获取提示词的接口。
// 用于抽象不同类型的请求对象（如 TaskSubmitReq）。
type HasPrompt interface {
	GetPrompt() string
}

// HasImage 定义了判断是否包含图片的接口。
type HasImage interface {
	HasImage() bool
}

// GetFullRequestURL 构建完整的上游请求 URL。
// 将基础 URL 和请求路径拼接，并处理 Cloudflare AI Gateway 的特殊路径格式：
//   - OpenAI: 移除 /v1 前缀
//   - Azure: 移除 /openai/deployments 前缀
//
// 参数：
//   - baseURL: 渠道的基础 URL
//   - requestURL: 请求路径
//   - channelType: 渠道类型
//
// 返回值：
//   - string: 完整的请求 URL
func GetFullRequestURL(baseURL string, requestURL string, channelType int) string {
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, requestURL)

	if strings.HasPrefix(baseURL, "https://gateway.ai.cloudflare.com") {
		switch channelType {
		case constant.ChannelTypeOpenAI:
			fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/v1"))
		case constant.ChannelTypeAzure:
			fullRequestURL = fmt.Sprintf("%s%s", baseURL, strings.TrimPrefix(requestURL, "/openai/deployments"))
		}
	}
	return fullRequestURL
}

// GetAPIVersion 从请求中获取 API 版本号。
// 优先从 URL 查询参数 "api-version" 中获取，其次从上下文中获取。
// 主要用于 Azure 渠道的 API 版本控制。
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - string: API 版本号
func GetAPIVersion(c *gin.Context) string {
	query := c.Request.URL.Query()
	apiVersion := query.Get("api-version")
	if apiVersion == "" {
		apiVersion = c.GetString("api_version")
	}
	return apiVersion
}

// createTaskError 创建一个 TaskError 对象。
//
// 参数：
//   - err: 原始错误
//   - code: 错误代码
//   - statusCode: HTTP 状态码
//   - localError: 是否为本地错误（true 表示不需要转发到上游）
//
// 返回值：
//   - *dto.TaskError: 任务错误对象
func createTaskError(err error, code string, statusCode int, localError bool) *dto.TaskError {
	return &dto.TaskError{
		Code:       code,
		Message:    err.Error(),
		StatusCode: statusCode,
		LocalError: localError,
		Error:      err,
	}
}

// storeTaskRequest 将任务请求存储到 Gin 上下文中。
// 设置任务操作类型并将请求对象存入上下文，供后续处理函数使用。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - action: 任务操作类型
//   - requestObj: 任务提交请求对象
func storeTaskRequest(c *gin.Context, info *RelayInfo, action string, requestObj TaskSubmitReq) {
	info.Action = action
	c.Set("task_request", requestObj)
}

// GetTaskRequest 从 Gin 上下文中获取之前存储的任务请求对象。
//
// 参数：
//   - c: Gin 上下文
//
// 返回值：
//   - TaskSubmitReq: 任务提交请求对象
//   - error: 获取失败时的错误（上下文中不存在或类型断言失败）
func GetTaskRequest(c *gin.Context) (TaskSubmitReq, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return TaskSubmitReq{}, fmt.Errorf("request not found in context")
	}
	req, ok := v.(TaskSubmitReq)
	if !ok {
		return TaskSubmitReq{}, fmt.Errorf("invalid task request type")
	}
	return req, nil
}

// validatePrompt 验证提示词是否为空。
//
// 参数：
//   - prompt: 待验证的提示词
//
// 返回值：
//   - *dto.TaskError: 验证失败时返回错误，通过时返回 nil
func validatePrompt(prompt string) *dto.TaskError {
	if strings.TrimSpace(prompt) == "" {
		return createTaskError(fmt.Errorf("prompt is required"), "invalid_request", http.StatusBadRequest, true)
	}
	return nil
}

// MaxTaskDurationSeconds 限制用户可控的视频任务时长。
//
// duration / seconds 会作为 OtherRatios["seconds"] 参与任务预扣费和结算快照。
// 入口层拒绝负数和超大值，可以避免异常请求把计费倍率放大到不可控范围；
// 未传或无法解析的值保持原有默认值语义，由具体 provider 适配器继续处理。
const MaxTaskDurationSeconds = 3600

// validateTaskDurationBounds 校验任务请求中的时长边界。
func validateTaskDurationBounds(req TaskSubmitReq) *dto.TaskError {
	seconds := req.Duration
	if seconds == 0 && req.Seconds != "" {
		seconds, _ = strconv.Atoi(req.Seconds)
	}
	if seconds < 0 || seconds > MaxTaskDurationSeconds {
		return createTaskError(fmt.Errorf("seconds must be between 1 and %d", MaxTaskDurationSeconds), "invalid_seconds", http.StatusBadRequest, true)
	}
	return nil
}

// validateMultipartTaskRequest 验证 multipart/form-data 格式的任务提交请求。
// 从表单数据中提取 prompt、model、mode、image、size、seconds 等字段，
// 以及未识别的字段存入 metadata。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - action: 任务操作类型
//
// 返回值：
//   - TaskSubmitReq: 解析后的任务提交请求
//   - error: 解析过程中的错误
func validateMultipartTaskRequest(c *gin.Context, info *RelayInfo, action string) (TaskSubmitReq, error) {
	var req TaskSubmitReq
	if _, err := c.MultipartForm(); err != nil {
		return req, err
	}

	formData := c.Request.PostForm
	req = TaskSubmitReq{
		Prompt:   formData.Get("prompt"),
		Model:    formData.Get("model"),
		Mode:     formData.Get("mode"),
		Image:    formData.Get("image"),
		Size:     formData.Get("size"),
		Metadata: make(map[string]interface{}),
	}

	if durationStr := formData.Get("seconds"); durationStr != "" {
		if duration, err := strconv.Atoi(durationStr); err == nil {
			req.Duration = duration
		}
	}

	if images := formData["images"]; len(images) > 0 {
		req.Images = images
	}

	for key, values := range formData {
		if len(values) > 0 && !isKnownTaskField(key) {
			if intVal, err := strconv.Atoi(values[0]); err == nil {
				req.Metadata[key] = intVal
			} else if floatVal, err := strconv.ParseFloat(values[0], 64); err == nil {
				req.Metadata[key] = floatVal
			} else {
				req.Metadata[key] = values[0]
			}
		}
	}
	return req, nil
}

// ValidateMultipartDirect 验证直接的 JSON 格式任务提交请求。
// 支持视频生成任务的特殊验证逻辑：
//   - sora-2 模型：验证 size 和 seconds 参数
//   - sora-2-pro 模型：支持更多分辨率选项
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//
// 返回值：
//   - *dto.TaskError: 验证失败时返回错误，通过时返回 nil
func ValidateMultipartDirect(c *gin.Context, info *RelayInfo) *dto.TaskError {
	var prompt string
	var model string
	var seconds int
	var size string
	var hasInputReference bool

	var req TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return createTaskError(err, "invalid_json", http.StatusBadRequest, true)
	}

	prompt = req.Prompt
	model = req.Model
	size = req.Size
	seconds, _ = strconv.Atoi(req.Seconds)
	if seconds == 0 {
		seconds = req.Duration
	}
	if req.InputReference != "" {
		req.Images = []string{req.InputReference}
	} else if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		// 直连 JSON 路径也兼容单图 image 字段，确保图生视频动作判断与 Basic 路径一致。
		req.Images = []string{strings.TrimSpace(req.Image)}
	}

	if strings.TrimSpace(req.Model) == "" {
		return createTaskError(fmt.Errorf("model field is required"), "missing_model", http.StatusBadRequest, true)
	}

	if req.HasImage() {
		hasInputReference = true
	}

	if taskErr := validatePrompt(prompt); taskErr != nil {
		return taskErr
	}

	if taskErr := validateTaskDurationBounds(req); taskErr != nil {
		return taskErr
	}

	action := constant.TaskActionTextGenerate
	if hasInputReference {
		action = constant.TaskActionGenerate
	}
	if strings.HasPrefix(model, "sora-2") {

		if size == "" {
			size = "720x1280"
		}

		if seconds <= 0 {
			seconds = 4
		}

		if model == "sora-2" && !lo.Contains([]string{"720x1280", "1280x720"}, size) {
			return createTaskError(fmt.Errorf("sora-2 size is invalid"), "invalid_size", http.StatusBadRequest, true)
		}
		if model == "sora-2-pro" && !lo.Contains([]string{"720x1280", "1280x720", "1792x1024", "1024x1792"}, size) {
			return createTaskError(fmt.Errorf("sora-2 size is invalid"), "invalid_size", http.StatusBadRequest, true)
		}
		// OtherRatios 已移到 Sora adaptor 的 EstimateBilling 中设置
	}

	storeTaskRequest(c, info, action, req)

	return nil
}

// isKnownTaskField 判断表单字段是否为已知的任务字段。
// 用于在 multipart 请求中区分核心字段和需要存入 metadata 的额外字段。
//
// 参数：
//   - field: 字段名称
//
// 返回值：
//   - bool: 是否为已知字段
func isKnownTaskField(field string) bool {
	knownFields := map[string]bool{
		"prompt":          true,
		"model":           true,
		"mode":            true,
		"image":           true,
		"images":          true,
		"size":            true,
		"duration":        true,
		"input_reference": true, // Sora 特有字段
	}
	return knownFields[field]
}

// ValidateBasicTaskRequest 验证基本的任务提交请求。
// 支持两种内容类型：
//   - multipart/form-data: 调用 validateMultipartTaskRequest 解析
//   - application/json: 使用 UnmarshalBodyReusable 统一解析
//
// 验证完成后将请求对象存储到上下文中。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继信息
//   - action: 任务操作类型
//
// 返回值：
//   - *dto.TaskError: 验证失败时返回错误，通过时返回 nil
func ValidateBasicTaskRequest(c *gin.Context, info *RelayInfo, action string) *dto.TaskError {
	var err error
	contentType := c.GetHeader("Content-Type")
	var req TaskSubmitReq
	if strings.HasPrefix(contentType, "multipart/form-data") {
		req, err = validateMultipartTaskRequest(c, info, action)
		if err != nil {
			return createTaskError(err, "invalid_multipart_form", http.StatusBadRequest, true)
		}
	}
	// 为了metadata字段的兼容性，统一UnmarshalBodyReusable
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return createTaskError(err, "invalid_request", http.StatusBadRequest, true)
	}

	if taskErr := validatePrompt(req.Prompt); taskErr != nil {
		return taskErr
	}

	if taskErr := validateTaskDurationBounds(req); taskErr != nil {
		return taskErr
	}

	if len(req.Images) == 0 && strings.TrimSpace(req.Image) != "" {
		// 兼容单图上传
		req.Images = []string{req.Image}
	}

	storeTaskRequest(c, info, action, req)
	return nil
}
