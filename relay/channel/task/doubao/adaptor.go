// doubao - adaptor.go
// 豆包（Doubao / Seedance）视频生成任务适配器。
// 实现了 taskcommon.TaskAdaptor 接口，负责将统一的视频生成请求转换为豆包 Seedance API 格式。
// 支持文生视频和图生视频模式，处理视频输入折扣计费逻辑。
// 豆包 API 使用 content 数组来组织多模态输入（文本、图片、视频等）。
package doubao

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/c1cada/NexusTok/common"

	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/relay/channel"
	"github.com/c1cada/NexusTok/relay/channel/task/taskcommon"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/samber/lo"
)

// ============================
// 请求 / 响应数据结构
// ============================

// ContentItem 豆包 API 的多模态内容项结构体。
// 用于构建 content 数组中的单个条目，支持文本、图片、视频、音频等多种类型。
type ContentItem struct {
	Type     string    `json:"type,omitempty"`         // 内容类型: "text"、"image_url"、"video_url"、"audio_url"
	Text     string    `json:"text,omitempty"`         // 文本内容（当 type 为 "text" 时使用）
	ImageURL *MediaURL `json:"image_url,omitempty"`    // 图片URL（当 type 为 "image_url" 时使用）
	VideoURL *MediaURL `json:"video_url,omitempty"`    // 视频URL（当 type 为 "video_url" 时使用，用于图生视频的参考视频）
	AudioURL *MediaURL `json:"audio_url,omitempty"`    // 音频URL（当 type 为 "audio_url" 时使用）
	Role     string    `json:"role,omitempty"`         // 内容角色（用于对话式生成）
}

// MediaURL 媒体资源URL包装结构体。
// 用于在 ContentItem 中嵌套引用媒体资源。
type MediaURL struct {
	URL string `json:"url,omitempty"` // 媒体资源的URL地址
}

// requestPayload 豆包 Seedance 视频生成 API 的请求体结构体。
// 包含模型名称、多模态内容、回调URL、生成参数等。
type requestPayload struct {
	Model                 string         `json:"model"`                            // 模型名称（如 "doubao-seedance-1-0-pro-250528"）
	Content               []ContentItem  `json:"content,omitempty"`                // 多模态内容数组（文本、图片、视频等）
	CallbackURL           string         `json:"callback_url,omitempty"`           // 任务完成后的回调通知URL
	ReturnLastFrame       *dto.BoolValue `json:"return_last_frame,omitempty"`      // 是否返回视频最后一帧图片
	ServiceTier           string         `json:"service_tier,omitempty"`           // 服务等级
	ExecutionExpiresAfter *dto.IntValue  `json:"execution_expires_after,omitempty"` // 任务执行超时时间（秒）
	GenerateAudio         *dto.BoolValue `json:"generate_audio,omitempty"`         // 是否为视频生成配乐
	Draft                 *dto.BoolValue `json:"draft,omitempty"`                  // 是否为草稿模式
	Tools                 []struct {
		Type string `json:"type,omitempty"` // 工具类型（如 "web_search"）
	} `json:"tools,omitempty"`                // 使用的工具列表
	Resolution  string         `json:"resolution,omitempty"`    // 视频分辨率
	Ratio       string         `json:"ratio,omitempty"`         // 视频宽高比
	Duration    *dto.IntValue  `json:"duration,omitempty"`      // 视频时长（秒）
	Frames      *dto.IntValue  `json:"frames,omitempty"`        // 视频帧数
	Seed        *dto.IntValue  `json:"seed,omitempty"`          // 随机种子
	CameraFixed *dto.BoolValue `json:"camera_fixed,omitempty"`  // 是否固定摄像头
	Watermark   *dto.BoolValue `json:"watermark,omitempty"`     // 是否添加水印
}

// responsePayload 豆包任务提交响应结构体。
// 包含新创建的任务ID。
type responsePayload struct {
	ID string `json:"id"` // 任务唯一标识（task_id）
}

// responseTask 豆包任务详情响应结构体。
// 包含任务的完整状态信息、生成结果、使用量统计和错误信息。
type responseTask struct {
	ID      string `json:"id"`       // 任务唯一标识
	Model   string `json:"model"`    // 使用的模型名称
	Status  string `json:"status"`   // 任务状态: pending/queued/processing/running/succeeded/failed
	Content struct {
		VideoURL string `json:"video_url"` // 生成的视频下载URL（仅在任务成功时有值）
	} `json:"content"`                   // 任务生成的内容
	Seed            int    `json:"seed"`              // 随机种子
	Resolution      string `json:"resolution"`        // 视频分辨率
	Duration        int    `json:"duration"`          // 视频时长（秒）
	Ratio           string `json:"ratio"`             // 视频宽高比
	FramesPerSecond int    `json:"framespersecond"`   // 帧率
	ServiceTier     string `json:"service_tier"`      // 服务等级
	Tools           []struct {
		Type string `json:"type"` // 使用的工具类型
	} `json:"tools"`                               // 使用的工具列表
	Usage struct {
		CompletionTokens int `json:"completion_tokens"` // 完成 token 数量
		TotalTokens      int `json:"total_tokens"`      // 总 token 数量
		ToolUsage        struct {
			WebSearch int `json:"web_search"` // 网络搜索工具使用次数
		} `json:"tool_usage"`                        // 工具使用统计
	} `json:"usage"`                               // 使用量统计
	Error struct {
		Code    string `json:"code"`    // 错误码
		Message string `json:"message"` // 错误消息
	} `json:"error"`                             // 错误信息（仅在任务失败时有值）
	CreatedAt int64 `json:"created_at"`          // 任务创建时间（Unix 时间戳）
	UpdatedAt int64 `json:"updated_at"`          // 任务最后更新时间（Unix 时间戳）
}

// ============================
// 适配器实现
// ============================

// TaskAdaptor 豆包 Seedance 视频生成任务适配器。
// 实现 taskcommon.TaskAdaptor 接口，负责与豆包 Seedance API 交互。
// 继承 BaseBilling 以获得基础计费功能。
type TaskAdaptor struct {
	taskcommon.BaseBilling           // 嵌入基础计费功能
	ChannelType int                  // 渠道类型常量
	apiKey      string               // 豆包 API 密钥（Bearer Token）
	baseURL     string               // 豆包 API 基础URL
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
// 使用 ValidateBasicTaskRequest 解析请求体，验证必要字段，
// 默认设置动作为 TaskActionGenerate（视频生成）。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：验证错误信息，nil 表示验证通过
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	// Accept only POST /v1/video/generations as "generate" action.
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL 构建豆包 Seedance 视频生成 API 的请求URL。
// 端点格式: {baseURL}/api/v3/contents/generations/tasks
//
// 参数：
//   - _: 中继上下文信息（未使用）
//
// 返回：完整的API请求URL和可能的错误
func (a *TaskAdaptor) BuildRequestURL(_ *relaycommon.RelayInfo) (string, error) {
	return fmt.Sprintf("%s/api/v3/contents/generations/tasks", a.baseURL), nil
}

// BuildRequestHeader 设置豆包 Seedance API 所需的请求头。
// 包括 JSON 内容类型、Accept 头和 Bearer Token 认证。
//
// 参数：
//   - _: Gin 上下文（未使用）
//   - req: 即将发送的 HTTP 请求对象
//   - _: 中继上下文信息（未使用）
//
// 返回：设置头信息过程中的错误，nil 表示成功
func (a *TaskAdaptor) BuildRequestHeader(_ *gin.Context, req *http.Request, _ *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	return nil
}

// EstimateBilling 检测请求 metadata 中是否包含视频输入，计算视频折扣倍率。
// 如果请求中包含 video_url 类型的内容项，且该模型支持视频输入折扣，
// 则返回折扣系数用于降低计费费率。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：其他计费倍率 map（键为 "video_input"），nil 表示无额外倍率
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}
	if hasVideoInMetadata(req.Metadata) {
		if ratio, ok := GetVideoInputRatio(info.OriginModelName); ok {
			return map[string]float64{"video_input": ratio}
		}
	}
	return nil
}

// hasVideoInMetadata 检查 metadata 的 content 数组中是否包含视频输入。
// 直接检查 content 数组中是否存在 type 为 "video_url" 或包含 video_url 字段的条目。
// 避免构建完整的上游 requestPayload 结构体，提高检查效率。
//
// 参数：
//   - metadata: 请求的元数据 map
//
// 返回：true 表示包含视频输入，false 表示不包含
func hasVideoInMetadata(metadata map[string]interface{}) bool {
	if metadata == nil {
		return false
	}
	contentRaw, ok := metadata["content"]
	if !ok {
		return false
	}
	contentSlice, ok := contentRaw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range contentSlice {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if itemMap["type"] == "video_url" {
			return true
		}
		if _, has := itemMap["video_url"]; has {
			return true
		}
	}
	return false
}

// BuildRequestBody 构建发送给豆包 Seedance API 的请求体。
// 从 Gin context 中获取已解析的任务请求，转换为豆包特定格式后序列化为 JSON。
// 如果设置了模型映射，使用上游模型名称替代原始模型名。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：请求体 Reader 和可能的错误
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil, err
	}

	body, err := a.convertToRequestPayload(&req)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	if info.IsModelMapped {
		body.Model = info.UpstreamModelName
	} else {
		info.UpstreamModelName = body.Model
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest 发送 HTTP 请求到豆包 Seedance API。
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

// DoResponse 处理豆包 Seedance API 的响应。
// 解析响应体获取任务ID，构建 OpenAI 兼容格式的初始响应返回给客户端。
//
// 参数：
//   - c: Gin 上下文
//   - resp: 豆包 API 的 HTTP 响应
//   - info: 中继上下文信息
//
// 返回：
//   - taskID: 豆包任务ID（用于后续轮询）
//   - taskData: 原始响应数据（用于存储）
//   - taskErr: 任务错误信息（nil 表示成功）
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Doubao response
	var dResp responsePayload
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if dResp.ID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName

	c.JSON(http.StatusOK, ov)
	return dResp.ID, responseBody, nil
}

// FetchTask 向豆包查询异步任务的当前状态。
// 通过 GET /api/v3/contents/generations/tasks/{taskID} 接口获取任务最新状态。
// 此方法在后台轮询任务时被调用。
//
// 参数：
//   - baseUrl: 豆包 API 基础URL
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

	uri := fmt.Sprintf("%s/api/v3/contents/generations/tasks/%s", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, uri, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// GetModelList 返回豆包 Seedance 支持的视频生成模型列表。
// 返回的列表用于路由匹配和模型验证。
//
// 返回：支持的模型名称切片
func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回豆包视频生成渠道的唯一标识名称。
//
// 返回：渠道名称字符串（"doubao-video"）
func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// convertToRequestPayload 将统一的 TaskSubmitReq 转换为豆包 Seedance API 的请求格式。
// 处理图片输入、metadata 参数解析和时长转换。
// 将图片URL添加为 content 数组中的 image_url 类型条目，
// 文本提示词作为 content 数组中的最后一个 text 类型条目。
//
// 参数：
//   - req: 统一的任务提交请求
//
// 返回：豆包格式的请求和可能的错误
func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq) (*requestPayload, error) {
	r := requestPayload{
		Model:   req.Model,
		Content: []ContentItem{},
	}

	// Add images if present
	if req.HasImage() {
		for _, imgURL := range req.Images {
			r.Content = append(r.Content, ContentItem{
				Type: "image_url",
				ImageURL: &MediaURL{
					URL: imgURL,
				},
			})
		}
	}

	metadata := req.Metadata
	if err := taskcommon.UnmarshalMetadata(metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	if sec, _ := strconv.Atoi(req.Seconds); sec > 0 {
		r.Duration = lo.ToPtr(dto.IntValue(sec))
	}

	r.Content = lo.Reject(r.Content, func(c ContentItem, _ int) bool { return c.Type == "text" })
	r.Content = append(r.Content, ContentItem{
		Type: "text",
		Text: req.Prompt,
	})

	return &r, nil
}

// ParseTaskResult 解析豆包任务轮询的响应数据。
// 将豆包的任务状态（pending/queued/processing/running/succeeded/failed）
// 映射为内部统一状态，并设置进度百分比。
// 任务成功时提取视频URL和 usage 信息用于按倍率计费。
//
// 参数：
//   - respBody: 豆包任务查询接口的响应体字节数据
//
// 返回：统一的任务信息结构体和可能的解析错误
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}

	taskResult := relaycommon.TaskInfo{
		Code: 0,
	}

	// Map Doubao status to internal status
	switch resTask.Status {
	case "pending", "queued":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "processing", "running":
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "50%"
	case "succeeded":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
		taskResult.Url = resTask.Content.VideoURL
		// 解析 usage 信息用于按倍率计费
		taskResult.CompletionTokens = resTask.Usage.CompletionTokens
		taskResult.TotalTokens = resTask.Usage.TotalTokens
	case "failed":
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
		taskResult.Reason = resTask.Error.Message
	default:
		// Unknown status, treat as processing
		taskResult.Status = model.TaskStatusInProgress
		taskResult.Progress = "30%"
	}

	return &taskResult, nil
}

// ConvertToOpenAIVideo 将存储的豆包任务数据转换为 OpenAI 兼容的视频响应格式。
// 从数据库中的原始任务数据反序列化豆包响应，构建 OpenAI 格式的视频对象，
// 包含任务ID、状态、进度、视频URL和错误信息。
//
// 参数：
//   - originTask: 数据库中的任务记录
//
// 返回：OpenAI 格式视频响应的 JSON 字节数据和可能的错误
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var dResp responseTask
	if err := common.Unmarshal(originTask.Data, &dResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal doubao task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.TaskID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", dResp.Content.VideoURL)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt
	openAIVideo.Model = originTask.Properties.OriginModelName

	if dResp.Status == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: dResp.Error.Message,
			Code:    dResp.Error.Code,
		}
	}

	return common.Marshal(openAIVideo)
}
