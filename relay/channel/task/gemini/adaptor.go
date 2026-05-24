// Package gemini 实现 Google Gemini/Veo 视频生成任务的适配器。
// 负责将标准任务请求转换为 Veo predictLongRunning API 格式，
// 处理任务提交、状态轮询、结果解析和计费估算。
// 支持文生视频和图生视频两种模式。
package gemini

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common"                                      // 通用工具函数
	"github.com/c1cada/NexusTok/constant"                                     // 常量定义（任务动作等）
	"github.com/c1cada/NexusTok/dto"                                          // 数据传输对象
	"github.com/c1cada/NexusTok/model"                                        // 数据模型
	"github.com/c1cada/NexusTok/relay/channel"                                // 渠道通用工具
	taskcommon "github.com/c1cada/NexusTok/relay/channel/task/taskcommon"     // 任务通用工具
	relaycommon "github.com/c1cada/NexusTok/relay/common"                     // 中继层通用结构体
	"github.com/c1cada/NexusTok/service"                                       // 服务层工具
	"github.com/c1cada/NexusTok/setting/model_setting"                        // 模型设置

	// 第三方依赖
	"github.com/gin-gonic/gin"      // HTTP 框架
	"github.com/pkg/errors"         // 错误包装库
)

// ============================
// 适配器实现
// ============================

// TaskAdaptor Gemini/Veo 视频生成任务适配器。
// 实现 TaskAdaptor 接口，处理 Veo 模型的视频生成请求。
type TaskAdaptor struct {
	taskcommon.BaseBilling                // 内嵌基础计费实现（默认无额外计费）
	ChannelType int                       // 渠道类型
	apiKey      string                    // API Key
	baseURL     string                    // API 基础 URL
}

// Init 初始化适配器，从中继信息中提取渠道配置。
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction 验证请求参数并设置默认任务动作。
// 默认动作：文本生成（TextGenerate）。
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL 构建 Veo predictLongRunning API 的请求 URL。
//
// URL 格式：{baseURL}/{version}/models/{modelName}:predictLongRunning
// 版本号从模型设置中获取。
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	modelName := info.UpstreamModelName
	version := model_setting.GetGeminiVersionSetting(modelName)

	return fmt.Sprintf(
		"%s/%s/models/%s:predictLongRunning",
		a.baseURL,
		version,
		modelName,
	), nil
}

// BuildRequestHeader 设置请求头，包括 Content-Type 和 API Key 鉴权。
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", a.apiKey)
	return nil
}

// BuildRequestBody 将任务请求转换为 Veo predictLongRunning 格式。
//
// 处理逻辑：
//  1. 从上下文中获取任务请求
//  2. 构建 VeoInstance（提示词 + 可选的参考图片）
//  3. 从 metadata 中解析 VeoParameters（时长、分辨率等）
//  4. 应用默认值和格式转换
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("unexpected task_request type")
	}

	// 构建 Veo 实例（提示词 + 可选图片）
	instance := VeoInstance{Prompt: req.Prompt}
	// 优先从 multipart 表单中提取参考图片
	if img := ExtractMultipartImage(c, info); img != nil {
		instance.Image = img
	} else if len(req.Images) > 0 {
		// 其次从请求的图片列表中解析
		if parsed := ParseImageInput(req.Images[0]); parsed != nil {
			instance.Image = parsed
			info.Action = constant.TaskActionGenerate
		}
	}

	// 从 metadata 中解析参数
	params := &VeoParameters{}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, params); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	// 应用默认值
	if params.DurationSeconds == 0 && req.Duration > 0 {
		params.DurationSeconds = req.Duration
	}
	if params.Resolution == "" && req.Size != "" {
		params.Resolution = SizeToVeoResolution(req.Size)
	}
	if params.AspectRatio == "" && req.Size != "" {
		params.AspectRatio = SizeToVeoAspectRatio(req.Size)
	}
	params.Resolution = strings.ToLower(params.Resolution)
	params.SampleCount = 1 // 目前只生成一个样本

	body := VeoRequestPayload{
		Instances:  []VeoInstance{instance},
		Parameters: params,
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest 发送 HTTP 请求到 Veo API。
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 Veo API 的提交响应，提取任务 ID 并返回给客户端。
//
// Veo 的提交响应包含一个 operation name，需要编码为本地任务 ID 存储。
// 同时向客户端返回一个初始的 OpenAI Video 对象。
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
	}
	_ = resp.Body.Close()

	var s submitResponse
	if err := common.Unmarshal(responseBody, &s); err != nil {
		return "", nil, service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
	}
	if strings.TrimSpace(s.Name) == "" {
		return "", nil, service.TaskErrorWrapper(fmt.Errorf("missing operation name"), "invalid_response", http.StatusInternalServerError)
	}
	// 将上游 operation name 编码为本地任务 ID
	taskID = taskcommon.EncodeLocalTaskID(s.Name)
	// 返回初始的 OpenAI Video 对象给客户端
	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return taskID, responseBody, nil
}

// GetModelList 返回 Gemini/Veo 支持的视频生成模型列表。
func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"veo-3.0-generate-001",        // Veo 3.0 标准版
		"veo-3.0-fast-generate-001",   // Veo 3.0 快速版
		"veo-3.1-generate-preview",    // Veo 3.1 预览版
		"veo-3.1-fast-generate-preview", // Veo 3.1 快速预览版
	}
}

// GetChannelName 返回渠道名称 "gemini"。
func (a *TaskAdaptor) GetChannelName() string {
	return "gemini"
}

// EstimateBilling 根据视频时长和分辨率估算计费比例。
// 返回的 map 包含 "seconds"（时长）和 "resolution"（分辨率系数）。
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	v, ok := c.Get("task_request")
	if !ok {
		return nil
	}
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil
	}

	seconds := ResolveVeoDuration(req.Metadata, req.Duration, req.Seconds)
	resolution := ResolveVeoResolution(req.Metadata, req.Size)
	resRatio := VeoResolutionRatio(info.UpstreamModelName, resolution)

	return map[string]float64{
		"seconds":    float64(seconds),
		"resolution": resRatio,
	}
}

// FetchTask 轮询 Veo 任务状态。
// 通过 Gemini operations GET 端点查询任务进度。
//
// 参数：
//   - baseUrl: API 基础 URL
//   - key: API Key
//   - body: 包含 task_id 的请求体
//   - proxy: 代理地址
//
// 返回：任务状态的 HTTP 响应。
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	// 解码本地任务 ID 为上游 operation name
	upstreamName, err := taskcommon.DecodeLocalTaskID(taskID)
	if err != nil {
		return nil, fmt.Errorf("decode task_id failed: %w", err)
	}

	version := model_setting.GetGeminiVersionSetting("default")
	url := fmt.Sprintf("%s/%s/%s", baseUrl, version, upstreamName)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-goog-api-key", key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// ParseTaskResult 解析 Veo 任务的轮询响应，提取任务状态和结果。
//
// 返回的 TaskInfo 包含：
//   - Status: 任务状态（进行中/成功/失败）
//   - Progress: 进度百分比
//   - RemoteUrl: 视频下载地址（成功时）
//   - Reason: 失败原因（失败时）
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op operationResponse
	if err := common.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}

	ti := &relaycommon.TaskInfo{}

	// 检查是否有错误
	if op.Error.Message != "" {
		ti.Status = model.TaskStatusFailure
		ti.Reason = op.Error.Message
		ti.Progress = "100%"
		return ti, nil
	}

	// 检查是否仍在处理中
	if !op.Done {
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
		return ti, nil
	}

	// 任务成功完成
	ti.Status = model.TaskStatusSuccess
	ti.Progress = "100%"

	ti.TaskID = taskcommon.EncodeLocalTaskID(op.Name)

	// 提取视频下载地址
	if len(op.Response.GenerateVideoResponse.GeneratedVideos) > 0 {
		if uri := op.Response.GenerateVideoResponse.GeneratedVideos[0].Video.URI; uri != "" {
			ti.RemoteUrl = uri
		}
	}

	return ti, nil
}

// ConvertToOpenAIVideo 将内部任务模型转换为 OpenAI Video 格式的 JSON。
// 用于任务状态查询接口的响应。
func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	upstreamTaskID := task.GetUpstreamTaskID()
	upstreamName, err := taskcommon.DecodeLocalTaskID(upstreamTaskID)
	if err != nil {
		upstreamName = ""
	}
	// 从 operation name 中提取模型名称
	modelName := extractModelFromOperationName(upstreamName)
	if strings.TrimSpace(modelName) == "" {
		modelName = "veo-3.0-generate-001" // 默认模型
	}

	video := dto.NewOpenAIVideo()
	video.ID = task.TaskID
	video.Model = modelName
	video.Status = task.Status.ToVideoStatus()
	video.SetProgressStr(task.Progress)
	video.CreatedAt = task.CreatedAt
	if task.FinishTime > 0 {
		video.CompletedAt = task.FinishTime
	} else if task.UpdatedAt > 0 {
		video.CompletedAt = task.UpdatedAt
	}

	return common.Marshal(video)
}

// ============================
// 辅助函数
// ============================

// modelRe 用于从 operation name 中提取模型名称的正则表达式。
// 匹配格式：models/{modelName}/operations/
var modelRe = regexp.MustCompile(`models/([^/]+)/operations/`)

// extractModelFromOperationName 从 Veo operation name 中提取模型名称。
// 支持格式：...models/{modelName}/operations/{operationId}
func extractModelFromOperationName(name string) string {
	if name == "" {
		return ""
	}
	if m := modelRe.FindStringSubmatch(name); len(m) == 2 {
		return m[1]
	}
	if idx := strings.Index(name, "models/"); idx >= 0 {
		s := name[idx+len("models/"):]
		if p := strings.Index(s, "/operations/"); p > 0 {
			return s[:p]
		}
	}
	return ""
}
