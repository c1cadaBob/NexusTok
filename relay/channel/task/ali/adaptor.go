// ali - adaptor.go
// 阿里云通义万相（Wanx）视频生成任务适配器。
// 实现了 taskcommon.TaskAdaptor 接口，负责将统一的视频生成请求转换为阿里 DashScope API 格式，
// 提交异步视频合成任务、轮询任务状态、解析任务结果，并转换为 OpenAI 兼容的视频响应格式。
// 支持文生视频（t2v）、图生视频（i2v）、首尾帧生视频（kf2v）等多种模式。
// 计费支持按视频时长和分辨率动态计算倍率。
package ali

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/task/taskcommon"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

// ============================
// 请求 / 响应数据结构
// ============================

// AliVideoRequest 阿里云通义万相视频生成 API 的顶层请求结构体。
// 包含模型名称、输入参数和生成参数。
type AliVideoRequest struct {
	Model      string              `json:"model"`                // 模型名称（如 "wan2.5-i2v-preview"）
	Input      AliVideoInput       `json:"input"`                // 输入参数（提示词、图片URL等）
	Parameters *AliVideoParameters `json:"parameters,omitempty"` // 生成参数（分辨率、时长等），可选
}

// AliVideoInput 视频生成的输入参数结构体。
// 支持多种输入模式：纯文本提示词、图生视频（单张图片）、首尾帧生视频（两张图片）、音频驱动等。
type AliVideoInput struct {
	Prompt         string `json:"prompt,omitempty"`          // 文本提示词，描述视频内容
	ImgURL         string `json:"img_url,omitempty"`         // 首帧图像URL或Base64（用于图生视频模式）
	FirstFrameURL  string `json:"first_frame_url,omitempty"` // 首帧图片URL（用于首尾帧生视频模式）
	LastFrameURL   string `json:"last_frame_url,omitempty"`  // 尾帧图片URL（用于首尾帧生视频模式）
	AudioURL       string `json:"audio_url,omitempty"`       // 音频URL（wan2.5版本支持音频驱动视频生成）
	NegativePrompt string `json:"negative_prompt,omitempty"` // 反向提示词（描述不希望在视频中出现的内容）
	Template       string `json:"template,omitempty"`        // 视频特效模板标识
}

// AliVideoParameters 视频生成的控制参数结构体。
// 用于配置视频的分辨率、尺寸、时长、水印等属性。
type AliVideoParameters struct {
	Resolution   string `json:"resolution,omitempty"`    // 分辨率等级: 480P/720P/1080P（用于图生视频、首尾帧生视频模式）
	Size         string `json:"size,omitempty"`          // 精确尺寸: 如 "832*480"、"1920*1080"（用于文生视频模式）
	Duration     int    `json:"duration,omitempty"`      // 视频时长（秒），取值范围 3-10
	PromptExtend bool   `json:"prompt_extend,omitempty"` // 是否开启 prompt 智能改写功能，扩展用户输入的提示词
	Watermark    bool   `json:"watermark,omitempty"`     // 是否在生成的视频中添加水印
	Audio        *bool  `json:"audio,omitempty"`         // 是否生成音频（仅 wan2.5 及以上版本支持）
	Seed         int    `json:"seed,omitempty"`          // 随机数种子，用于控制生成结果的可复现性
}

// AliVideoResponse 阿里云通义万相视频生成 API 的响应结构体。
// 包含任务输出信息、请求ID、错误码和使用统计。
type AliVideoResponse struct {
	Output    AliVideoOutput `json:"output"`            // 任务输出信息（状态、视频URL等）
	RequestID string         `json:"request_id"`        // 阿里云请求唯一标识
	Code      string         `json:"code,omitempty"`    // 错误码（非空表示请求级错误）
	Message   string         `json:"message,omitempty"` // 错误消息（配合 Code 使用）
	Usage     *AliUsage      `json:"usage,omitempty"`   // 使用统计信息（视频时长、数量等）
}

// AliVideoOutput 视频生成任务的输出信息结构体。
// 包含任务ID、状态、时间戳、提示词以及结果视频URL。
type AliVideoOutput struct {
	TaskID        string `json:"task_id"`                  // 阿里云异步任务唯一标识
	TaskStatus    string `json:"task_status"`              // 任务状态: PENDING/RUNNING/SUCCEEDED/FAILED/CANCELED/UNKNOWN
	SubmitTime    string `json:"submit_time,omitempty"`    // 任务提交时间（ISO 8601 格式）
	ScheduledTime string `json:"scheduled_time,omitempty"` // 任务调度开始时间
	EndTime       string `json:"end_time,omitempty"`       // 任务结束时间
	OrigPrompt    string `json:"orig_prompt,omitempty"`    // 用户原始提示词（未经智能改写）
	ActualPrompt  string `json:"actual_prompt,omitempty"`  // 实际使用的提示词（经智能改写后的结果）
	VideoURL      string `json:"video_url,omitempty"`      // 生成的视频下载URL（仅在任务成功时有值）
	Code          string `json:"code,omitempty"`           // 任务级错误码
	Message       string `json:"message,omitempty"`        // 任务级错误消息
}

// AliUsage 阿里云视频生成的使用统计结构体。
// 用于记录生成视频的时长、数量等计费相关信息。
type AliUsage struct {
	Duration   dto.IntValue `json:"duration,omitempty"`    // 视频时长（秒）
	VideoCount dto.IntValue `json:"video_count,omitempty"` // 生成的视频数量
	SR         dto.IntValue `json:"SR,omitempty"`          // 采样率
}

// AliMetadata 阿里云视频生成的元数据结构体。
// 用于从请求 metadata 中提取额外参数，支持覆盖输入和生成参数。
// 所有字段均为指针类型，以区分"未设置"和"设置为零值"。
type AliMetadata struct {
	// Input 相关字段
	AudioURL       string `json:"audio_url,omitempty"`       // 音频URL
	ImgURL         string `json:"img_url,omitempty"`         // 图片URL（用于图生视频）
	FirstFrameURL  string `json:"first_frame_url,omitempty"` // 首帧图片URL（用于首尾帧生视频）
	LastFrameURL   string `json:"last_frame_url,omitempty"`  // 尾帧图片URL（用于首尾帧生视频）
	NegativePrompt string `json:"negative_prompt,omitempty"` // 反向提示词
	Template       string `json:"template,omitempty"`        // 视频特效模板

	// Parameters 相关字段
	Resolution   *string `json:"resolution,omitempty"`    // 分辨率: 480P/720P/1080P
	Size         *string `json:"size,omitempty"`          // 精确尺寸: 如 "832*480"
	Duration     *int    `json:"duration,omitempty"`      // 视频时长（秒）
	PromptExtend *bool   `json:"prompt_extend,omitempty"` // 是否开启 prompt 智能改写
	Watermark    *bool   `json:"watermark,omitempty"`     // 是否添加水印
	Audio        *bool   `json:"audio,omitempty"`         // 是否生成音频
	Seed         *int    `json:"seed,omitempty"`          // 随机数种子
}

// ============================
// 适配器实现
// ============================

// TaskAdaptor 阿里云通义万相视频生成任务适配器。
// 实现 taskcommon.TaskAdaptor 接口，负责与阿里 DashScope API 交互。
// 继承 BaseBilling 以获得基础计费功能。
type TaskAdaptor struct {
	taskcommon.BaseBilling        // 嵌入基础计费功能
	ChannelType            int    // 渠道类型常量（如 constant.ChannelTypeAli）
	apiKey                 string // 阿里云 API 密钥（Bearer Token）
	baseURL                string // 阿里云 DashScope API 基础URL
}

// Init 初始化适配器，从中继信息中提取渠道类型、API 基础URL 和密钥。
//
// 参数：
//   - info: 中继上下文信息，包含渠道配置和认证信息
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction 验证请求并设置任务动作。
// 使用 ValidateMultipartDirect 解析 multipart 表单请求，
// 同时将原始 TaskSubmitReq 存入 Gin context 供后续使用。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：验证错误信息，nil 表示验证通过
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// ValidateMultipartDirect 负责解析并将原始 TaskSubmitReq 存入 context
	return relaycommon.ValidateMultipartDirect(c, info)
}

// BuildRequestURL 构建阿里云 DashScope 视频生成 API 的请求URL。
// 端点格式: {baseURL}/api/v1/services/aigc/video-generation/video-synthesis
//
// 参数：
//   - info: 中继上下文信息
//
// 返回：完整的API请求URL和可能的错误
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v1/services/aigc/video-generation/video-synthesis", a.baseURL), nil
}

// BuildRequestHeader 设置阿里云 DashScope API 所需的请求头。
// 包括 Bearer Token 认证、JSON 内容类型，以及阿里异步任务必需的 X-DashScope-Async 头。
//
// 参数：
//   - c: Gin 上下文
//   - req: 即将发送的 HTTP 请求对象
//   - info: 中继上下文信息
//
// 返回：设置头信息过程中的错误，nil 表示成功
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-DashScope-Async", "enable") // 阿里异步任务必须设置
	return nil
}

// BuildRequestBody 构建发送给阿里云的请求体。
// 从 Gin context 中获取已解析的任务请求，转换为阿里 DashScope 格式后序列化为 JSON。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：请求体 Reader 和可能的错误
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_task_request_failed")
	}

	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		return nil, errors.Wrap(err, "convert_to_ali_request_failed")
	}
	logger.LogJson(c, "ali video request body", aliReq)

	bodyBytes, err := common.Marshal(aliReq)
	if err != nil {
		return nil, errors.Wrap(err, "marshal_ali_request_failed")
	}
	return bytes.NewReader(bodyBytes), nil
}

// 以下变量定义了阿里云通义万相支持的各分辨率对应的精确尺寸列表。
// 用于将用户请求的精确尺寸映射到对应的分辨率等级。
var (
	// size480p 480P 分辨率支持的精确尺寸列表
	size480p = []string{
		"832*480",
		"480*832",
		"624*624",
	}
	// size720p 720P 分辨率支持的精确尺寸列表
	size720p = []string{
		"1280*720",
		"720*1280",
		"960*960",
		"1088*832",
		"832*1088",
	}
	// size1080p 1080P 分辨率支持的精确尺寸列表
	size1080p = []string{
		"1920*1080",
		"1080*1920",
		"1440*1440",
		"1632*1248",
		"1248*1632",
	}
)

// sizeToResolution 将精确尺寸字符串转换为阿里云分辨率等级。
// 根据尺寸字符串匹配 size480p/size720p/size1080p 列表，返回对应的分辨率标签。
//
// 参数：
//   - size: 精确尺寸字符串，如 "1920*1080"
//
// 返回：分辨率等级字符串（"480P"/"720P"/"1080P"）和可能的错误
func sizeToResolution(size string) (string, error) {
	if lo.Contains(size480p, size) {
		return "480P", nil
	} else if lo.Contains(size720p, size) {
		return "720P", nil
	} else if lo.Contains(size1080p, size) {
		return "1080P", nil
	}
	return "", fmt.Errorf("invalid size: %s", size)
}

// ProcessAliOtherRatios 根据阿里云视频请求中的模型和分辨率，计算计费倍率。
// 不同模型在不同分辨率下有不同的价格倍率，此函数将模型+分辨率组合映射为计费系数。
// 返回的 otherRatios map 可直接用于计费计算。
//
// 参数：
//   - aliReq: 阿里云视频生成请求
//
// 返回：计费倍率 map（键为 "resolution-{等级}"）和可能的错误
func ProcessAliOtherRatios(aliReq *AliVideoRequest) (map[string]float64, error) {
	otherRatios := make(map[string]float64)
	aliRatios := map[string]map[string]float64{
		"wan2.6-i2v": {
			"720P":  1,
			"1080P": 1 / 0.6,
		},
		"wan2.5-t2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-t2v-plus": {
			"480P":  1,
			"1080P": 0.7 / 0.14,
		},
		"wan2.5-i2v-preview": {
			"480P":  1,
			"720P":  2,
			"1080P": 1 / 0.3,
		},
		"wan2.2-i2v-plus": {
			"480P":  1,
			"1080P": 0.7 / 0.14,
		},
		"wan2.2-kf2v-flash": {
			"480P":  1,
			"720P":  2,
			"1080P": 4.8,
		},
		"wan2.2-i2v-flash": {
			"480P": 1,
			"720P": 2,
		},
		"wan2.2-s2v": {
			"480P": 1,
			"720P": 0.9 / 0.5,
		},
	}
	var resolution string

	// size match
	if aliReq.Parameters.Size != "" {
		toResolution, err := sizeToResolution(aliReq.Parameters.Size)
		if err != nil {
			return nil, err
		}
		resolution = toResolution
	} else {
		resolution = strings.ToUpper(aliReq.Parameters.Resolution)
		if !strings.HasSuffix(resolution, "P") {
			resolution = resolution + "P"
		}
	}
	if otherRatio, ok := aliRatios[aliReq.Model]; ok {
		if ratio, ok := otherRatio[resolution]; ok {
			otherRatios[fmt.Sprintf("resolution-%s", resolution)] = ratio
		}
	}
	return otherRatios, nil
}

// convertToAliRequest 将统一的 TaskSubmitReq 转换为阿里云 DashScope API 的请求格式。
// 处理模型名称映射、分辨率/尺寸转换、时长解析，以及从 metadata 中提取额外参数。
// 如果设置了模型映射（IsModelMapped），使用上游模型名称替代原始模型名。
//
// 参数：
//   - info: 中继上下文信息
//   - req: 统一的任务提交请求
//
// 返回：阿里云格式的请求和可能的错误
func (a *TaskAdaptor) convertToAliRequest(info *relaycommon.RelayInfo, req relaycommon.TaskSubmitReq) (*AliVideoRequest, error) {
	upstreamModel := req.Model
	if info.IsModelMapped {
		upstreamModel = info.UpstreamModelName
	}
	aliReq := &AliVideoRequest{
		Model: upstreamModel,
		Input: AliVideoInput{
			Prompt: req.Prompt,
			ImgURL: req.InputReference,
		},
		Parameters: &AliVideoParameters{
			PromptExtend: true, // 默认开启智能改写
			Watermark:    false,
		},
	}

	// 处理分辨率映射
	if req.Size != "" {
		// text to video size must be contained *
		if strings.Contains(req.Model, "t2v") && !strings.Contains(req.Size, "*") {
			return nil, fmt.Errorf("invalid size: %s, example: %s", req.Size, "1920*1080")
		}
		if strings.Contains(req.Size, "*") {
			aliReq.Parameters.Size = req.Size
		} else {
			resolution := strings.ToUpper(req.Size)
			// 支持 480p, 720p, 1080p 或 480P, 720P, 1080P
			if !strings.HasSuffix(resolution, "P") {
				resolution = resolution + "P"
			}
			aliReq.Parameters.Resolution = resolution
		}
	} else {
		// 根据模型设置默认分辨率
		if strings.Contains(req.Model, "t2v") { // image to video
			if strings.HasPrefix(req.Model, "wan2.5") {
				aliReq.Parameters.Size = "1920*1080"
			} else if strings.HasPrefix(req.Model, "wan2.2") {
				aliReq.Parameters.Size = "1920*1080"
			} else {
				aliReq.Parameters.Size = "1280*720"
			}
		} else {
			if strings.HasPrefix(req.Model, "wan2.6") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.5") {
				aliReq.Parameters.Resolution = "1080P"
			} else if strings.HasPrefix(req.Model, "wan2.2-i2v-flash") {
				aliReq.Parameters.Resolution = "720P"
			} else if strings.HasPrefix(req.Model, "wan2.2-i2v-plus") {
				aliReq.Parameters.Resolution = "1080P"
			} else {
				aliReq.Parameters.Resolution = "720P"
			}
		}
	}

	// 处理时长
	if req.Duration > 0 {
		aliReq.Parameters.Duration = req.Duration
	} else if req.Seconds != "" {
		seconds, err := strconv.Atoi(req.Seconds)
		if err != nil {
			return nil, errors.Wrap(err, "convert seconds to int failed")
		} else {
			aliReq.Parameters.Duration = seconds
		}
	} else {
		aliReq.Parameters.Duration = 5 // 默认5秒
	}

	// 从 metadata 中提取额外参数
	if req.Metadata != nil {
		if metadataBytes, err := common.Marshal(req.Metadata); err == nil {
			err = common.Unmarshal(metadataBytes, aliReq)
			if err != nil {
				return nil, errors.Wrap(err, "unmarshal metadata failed")
			}
		} else {
			return nil, errors.Wrap(err, "marshal metadata failed")
		}
	}

	if aliReq.Model != upstreamModel {
		return nil, errors.New("can't change model with metadata")
	}

	return aliReq, nil
}

// EstimateBilling 根据用户请求参数计算 OtherRatios（时长、分辨率等）。
// 在 ValidateRequestAndSetAction 之后、价格计算之前调用。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	taskReq, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	aliReq, err := a.convertToAliRequest(info, taskReq)
	if err != nil {
		return nil
	}

	// metadata 可覆盖 duration 并绕过通用入口校验；作为计费倍率前统一钳制。
	otherRatios := map[string]float64{
		"seconds": float64(min(aliReq.Parameters.Duration, relaycommon.MaxTaskDurationSeconds)),
	}
	ratios, err := ProcessAliOtherRatios(aliReq)
	if err != nil {
		return otherRatios
	}
	for k, v := range ratios {
		otherRatios[k] = v
	}
	return otherRatios
}

// DoRequest 发送 HTTP 请求到阿里云 DashScope API。
// 委托给 channel.DoTaskApiRequest 通用方法处理实际请求发送。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//   - requestBody: 请求体 Reader
//
// 返回：HTTP 响应和可能的错误
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 处理阿里云 DashScope API 的响应。
// 解析响应体，检查错误，提取任务ID，并将结果转换为 OpenAI 兼容格式返回给客户端。
//
// 参数：
//   - c: Gin 上下文
//   - resp: 阿里云 API 的 HTTP 响应
//   - info: 中继上下文信息
//
// 返回：
//   - taskID: 阿里云任务ID（用于后续轮询）
//   - taskData: 原始响应数据（用于存储）
//   - taskErr: 任务错误信息（nil 表示成功）
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// 解析阿里响应
	var aliResp AliVideoResponse
	if err := common.Unmarshal(responseBody, &aliResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	// 检查错误
	if aliResp.Code != "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s: %s", aliResp.Code, aliResp.Message), "ali_api_error", resp.StatusCode)
		return
	}

	if aliResp.Output.TaskID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 转换为 OpenAI 格式响应
	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = info.PublicTaskID
	openAIResp.TaskID = info.PublicTaskID
	openAIResp.Model = c.GetString("model")
	if openAIResp.Model == "" && info != nil {
		openAIResp.Model = info.OriginModelName
	}
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.CreatedAt = common.GetTimestamp()

	// 返回 OpenAI 格式
	c.JSON(http.StatusOK, openAIResp)

	return aliResp.Output.TaskID, responseBody, nil
}

// FetchTask 向阿里云查询异步任务的当前状态。
// 通过 GET /api/v1/tasks/{taskID} 接口获取任务最新状态。
// 此方法在后台轮询任务时被调用。
//
// 参数：
//   - baseUrl: 阿里云 API 基础URL
//   - key: API 密钥
//   - body: 包含 task_id 的请求参数 map
//   - proxy: 代理地址（可为空）
//
// 返回：HTTP 响应和可能的错误
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/api/v1/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// GetModelList 返回阿里云通义万相支持的视频生成模型列表。
// 返回的列表用于路由匹配和模型验证。
//
// 返回：支持的模型名称切片
func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回阿里云通义万相渠道的唯一标识名称。
//
// 返回：渠道名称字符串（"ali"）
func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ParseTaskResult 解析阿里云任务轮询的响应数据。
// 将阿里云的任务状态（PENDING/RUNNING/SUCCEEDED/FAILED/CANCELED）映射为内部统一状态。
// 任务成功时提取视频URL，失败时提取错误原因。
//
// 参数：
//   - respBody: 阿里云任务查询接口的响应体字节数据
//
// 返回：统一的任务信息结构体和可能的解析错误
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(respBody, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// 状态映射
	switch aliResp.Output.TaskStatus {
	case "PENDING":
		taskResult.Status = model.TaskStatusQueued
	case "RUNNING":
		taskResult.Status = model.TaskStatusInProgress
	case "SUCCEEDED":
		taskResult.Status = model.TaskStatusSuccess
		// 阿里直接返回视频URL，不需要额外的代理端点
		taskResult.Url = aliResp.Output.VideoURL
	case "FAILED", "CANCELED", "UNKNOWN":
		taskResult.Status = model.TaskStatusFailure
		if aliResp.Message != "" {
			taskResult.Reason = aliResp.Message
		} else if aliResp.Output.Message != "" {
			taskResult.Reason = fmt.Sprintf("task failed, code: %s , message: %s", aliResp.Output.Code, aliResp.Output.Message)
		} else {
			taskResult.Reason = "task failed"
		}
	default:
		taskResult.Status = model.TaskStatusQueued
	}

	return &taskResult, nil
}

// ConvertToOpenAIVideo 将存储的阿里云任务数据转换为 OpenAI 兼容的视频响应格式。
// 从数据库中的原始任务数据反序列化阿里云响应，构建 OpenAI 格式的视频对象，
// 包含任务ID、状态、进度、视频URL和错误信息。
//
// 参数：
//   - task: 数据库中的任务记录
//
// 返回：OpenAI 格式视频响应的 JSON 字节数据和可能的错误
func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	var aliResp AliVideoResponse
	if err := common.Unmarshal(task.Data, &aliResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal ali response failed")
	}

	openAIResp := dto.NewOpenAIVideo()
	openAIResp.ID = task.TaskID
	openAIResp.Status = convertAliStatus(aliResp.Output.TaskStatus)
	openAIResp.Model = task.Properties.OriginModelName
	openAIResp.SetProgressStr(task.Progress)
	openAIResp.CreatedAt = task.CreatedAt
	openAIResp.CompletedAt = task.UpdatedAt

	// 设置视频URL（核心字段）
	openAIResp.SetMetadata("url", aliResp.Output.VideoURL)

	// 错误处理
	if aliResp.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Code,
			Message: aliResp.Message,
		}
	} else if aliResp.Output.Code != "" {
		openAIResp.Error = &dto.OpenAIVideoError{
			Code:    aliResp.Output.Code,
			Message: aliResp.Output.Message,
		}
	}

	return common.Marshal(openAIResp)
}

// convertAliStatus 将阿里云的任务状态字符串转换为 OpenAI 兼容的视频状态字符串。
//
// 状态映射关系：
//   - PENDING -> queued（排队中）
//   - RUNNING -> in_progress（处理中）
//   - SUCCEEDED -> completed（已完成）
//   - FAILED/CANCELED/UNKNOWN -> failed（失败）
//   - 其他 -> unknown（未知状态）
//
// 参数：
//   - aliStatus: 阿里云任务状态字符串
//
// 返回：OpenAI 兼容的状态字符串
func convertAliStatus(aliStatus string) string {
	switch aliStatus {
	case "PENDING":
		return dto.VideoStatusQueued
	case "RUNNING":
		return dto.VideoStatusInProgress
	case "SUCCEEDED":
		return dto.VideoStatusCompleted
	case "FAILED", "CANCELED", "UNKNOWN":
		return dto.VideoStatusFailed
	default:
		return dto.VideoStatusUnknown
	}
}
