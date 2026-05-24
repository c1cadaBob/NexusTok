// vidu - adaptor.go
// Vidu（生数科技）视频生成任务适配器。
// 实现了 taskcommon.TaskAdaptor 接口，负责将统一的视频生成请求转换为 Vidu API 格式。
// 支持文生视频（text2video）、图生视频（img2video）、首尾帧生视频（start-end2video）
// 和参考图生视频（reference2video）四种模式。
// 使用 Token 认证机制，API 密钥直接作为 Token 使用。
// 参考图生视频模式仅支持 viduq2 基础模型（不含 pro/turbo 后缀）。
package vidu

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/gin-gonic/gin"

	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/relay/channel"
	taskcommon "github.com/c1cada/NexusTok/relay/channel/task/taskcommon"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"

	"github.com/pkg/errors"
)

// ============================
// 请求 / 响应数据结构
// ============================

// requestPayload Vidu 视频生成 API 的请求体结构体。
// 包含模型名称、图片列表、提示词、时长、分辨率等参数。
type requestPayload struct {
	Model             string   `json:"model"`                         // 模型名称（如 "viduq2"、"viduq1"、"vidu2.0"、"vidu1.5"）
	Images            []string `json:"images"`                        // 参考图片URL列表（图生视频/首尾帧/参考图模式）
	Prompt            string   `json:"prompt,omitempty"`              // 文本提示词
	Duration          int      `json:"duration,omitempty"`            // 视频时长（秒），默认 5 秒
	Seed              int      `json:"seed,omitempty"`                // 随机数种子
	Resolution        string   `json:"resolution,omitempty"`          // 视频分辨率（如 "1080p"）
	MovementAmplitude string   `json:"movement_amplitude,omitempty"`  // 运动幅度: "auto"（自动）/ "small" / "medium" / "large"
	Bgm               bool     `json:"bgm,omitempty"`                 // 是否添加背景音乐
	Payload           string   `json:"payload,omitempty"`             // 扩展载荷数据
	CallbackUrl       string   `json:"callback_url,omitempty"`        // 任务完成后的回调通知URL
}

// responsePayload Vidu 任务提交响应结构体。
// 包含任务ID、状态、模型、输入参数和创建时间。
type responsePayload struct {
	TaskId            string   `json:"task_id"`            // 任务唯一标识
	State             string   `json:"state"`              // 任务状态
	Model             string   `json:"model"`              // 使用的模型名称
	Images            []string `json:"images"`             // 输入图片列表
	Prompt            string   `json:"prompt"`             // 输入提示词
	Duration          int      `json:"duration"`           // 视频时长（秒）
	Seed              int      `json:"seed"`               // 随机种子
	Resolution        string   `json:"resolution"`         // 视频分辨率
	Bgm               bool     `json:"bgm"`                // 是否添加背景音乐
	MovementAmplitude string   `json:"movement_amplitude"` // 运动幅度
	Payload           string   `json:"payload"`            // 扩展载荷数据
	CreatedAt         string   `json:"created_at"`         // 任务创建时间
}

// taskResultResponse Vidu 任务结果查询响应结构体。
// 包含任务状态、错误码、消耗积分和生成的视频列表。
type taskResultResponse struct {
	State     string     `json:"state"`      // 任务状态: created/queueing/processing/success/failed
	ErrCode   string     `json:"err_code"`   // 错误码（仅在任务失败时有值）
	Credits   int        `json:"credits"`    // 消耗的积分数量
	Payload   string     `json:"payload"`    // 扩展载荷数据
	Creations []creation `json:"creations"`  // 生成的视频/图片列表
}

// creation Vidu 生成的内容结构体。
// 表示一个生成的视频或图片结果。
type creation struct {
	ID       string `json:"id"`        // 内容唯一标识
	URL      string `json:"url"`       // 内容下载URL
	CoverURL string `json:"cover_url"` // 封面图片URL
}

// ============================
// 适配器实现
// ============================

// TaskAdaptor Vidu（生数科技）视频生成任务适配器。
// 实现 taskcommon.TaskAdaptor 接口，负责与 Vidu API 交互。
// 继承 BaseBilling 以获得基础计费功能。
type TaskAdaptor struct {
	taskcommon.BaseBilling           // 嵌入基础计费功能
	ChannelType int                  // 渠道类型常量
	baseURL     string               // Vidu API 基础URL
}

// Init 初始化适配器，从中继信息中提取渠道类型和 API 基础URL。
// Vidu 使用 Token 认证，API 密钥在各方法中通过 info 参数传递。
//
// 参数：
//   - info: 中继上下文信息，包含渠道配置
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
}

// ValidateRequestAndSetAction 验证请求并设置任务动作。
// 首先使用 ValidateBasicTaskRequest 解析请求体，然后根据以下规则确定动作类型：
//   - metadata 中包含 "action" 字段：使用指定的动作
//   - 包含图片输入：
//     - 2 张图片：首尾帧生视频（TaskActionFirstTailGenerate）
//     - 3 张及以上图片：参考图生视频（TaskActionReferenceGenerate）
//     - 1 张图片：图生视频（TaskActionGenerate）
//   - 无图片：文生视频（TaskActionTextGenerate）
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：验证错误信息，nil 表示验证通过
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	if err := relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate); err != nil {
		return err
	}
	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_task_request_failed", http.StatusBadRequest)
	}
	action := constant.TaskActionTextGenerate
	if meatAction, ok := req.Metadata["action"]; ok {
		action, _ = meatAction.(string)
	} else if req.HasImage() {
		action = constant.TaskActionGenerate
		if info.ChannelType == constant.ChannelTypeVidu {
			// vidu 增加 首尾帧生视频和参考图生视频
			if len(req.Images) == 2 {
				action = constant.TaskActionFirstTailGenerate
			} else if len(req.Images) > 2 {
				action = constant.TaskActionReferenceGenerate
			}
		}
	}
	info.Action = action
	return nil
}

// BuildRequestBody 构建发送给 Vidu API 的请求体。
// 从 Gin context 中获取已解析的任务请求，转换为 Vidu 特定格式后序列化为 JSON。
// 参考图生视频模式（reference2video）强制使用 viduq2 基础模型。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：请求体 Reader 和可能的错误
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, exists := c.Get("task_request")
	if !exists {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, err
	}

	if info.Action == constant.TaskActionReferenceGenerate {
		if strings.Contains(body.Model, "viduq2") {
			// 参考图生视频只能用 viduq2 模型, 不能带有pro或turbo后缀 https://platform.vidu.cn/docs/reference-to-video
			body.Model = "viduq2"
		}
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// BuildRequestURL 构建 Vidu 视频生成 API 的请求URL。
// 根据任务动作选择不同的端点路径：
//   - TaskActionGenerate（图生视频）: /ent/v2/img2video
//   - TaskActionFirstTailGenerate（首尾帧生视频）: /ent/v2/start-end2video
//   - TaskActionReferenceGenerate（参考图生视频）: /ent/v2/reference2video
//   - 其他（文生视频）: /ent/v2/text2video
//
// 参数：
//   - info: 中继上下文信息
//
// 返回：完整的API请求URL和可能的错误
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	var path string
	switch info.Action {
	case constant.TaskActionGenerate:
		path = "/img2video"
	case constant.TaskActionFirstTailGenerate:
		path = "/start-end2video"
	case constant.TaskActionReferenceGenerate:
		path = "/reference2video"
	default:
		path = "/text2video"
	}
	return fmt.Sprintf("%s/ent/v2%s", a.baseURL, path), nil
}

// BuildRequestHeader 设置 Vidu API 所需的请求头。
// 使用 Token 认证机制（非 Bearer Token），直接将 API 密钥作为 Token 使用。
//
// 参数：
//   - c: Gin 上下文
//   - req: 即将发送的 HTTP 请求对象
//   - info: 中继上下文信息
//
// 返回：设置头信息过程中的错误，nil 表示成功
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+info.ApiKey)
	return nil
}

// DoRequest 发送 HTTP 请求到 Vidu API。
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

// DoResponse 处理 Vidu API 的响应。
// 解析响应体获取任务ID，检查任务状态（failed 状态返回错误），
// 构建 OpenAI 兼容格式的初始响应返回给客户端。
//
// 参数：
//   - c: Gin 上下文
//   - resp: Vidu API 的 HTTP 响应
//   - info: 中继上下文信息
//
// 返回：
//   - taskID: Vidu 任务ID（用于后续轮询）
//   - taskData: 原始响应数据（用于存储）
//   - taskErr: 任务错误信息（nil 表示成功）
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var vResp responsePayload
	err = common.Unmarshal(responseBody, &vResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrap(err, fmt.Sprintf("%s", responseBody)), "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}

	if vResp.State == "failed" {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task failed"), "task_failed", http.StatusBadRequest)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return vResp.TaskId, responseBody, nil
}

// FetchTask 向 Vidu 查询异步任务的当前状态。
// 通过 GET /ent/v2/tasks/{taskID}/creations 接口获取任务最新状态和生成结果。
//
// 参数：
//   - baseUrl: Vidu API 基础URL
//   - key: API 密钥（Token）
//   - body: 包含 task_id 的请求参数 map
//   - proxy: 代理地址（可为空）
//
// 返回：HTTP 响应和可能的错误
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	url := fmt.Sprintf("%s/ent/v2/tasks/%s/creations", baseUrl, taskID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Token "+key)

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// GetModelList 返回 Vidu 支持的视频生成模型列表。
//
// 返回：支持的模型名称切片（"viduq2"、"viduq1"、"vidu2.0"、"vidu1.5"）
func (a *TaskAdaptor) GetModelList() []string {
	return []string{"viduq2", "viduq1", "vidu2.0", "vidu1.5"}
}

// GetChannelName 返回 Vidu 视频生成渠道的唯一标识名称。
//
// 返回：渠道名称字符串（"vidu"）
func (a *TaskAdaptor) GetChannelName() string {
	return "vidu"
}

// ============================
// 辅助函数
// ============================

// convertToRequestPayload 将统一的 TaskSubmitReq 转换为 Vidu API 的请求格式。
// 设置默认值：模型为 "viduq1"，时长为 5 秒，分辨率为 "1080p"，运动幅度为 "auto"。
// 从 metadata 中解析额外参数。
//
// 参数：
//   - req: 统一的任务提交请求
//   - info: 中继上下文信息
//
// 返回：Vidu 格式的请求和可能的错误
func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	r := requestPayload{
		Model:             taskcommon.DefaultString(info.UpstreamModelName, "viduq1"),
		Images:            req.Images,
		Prompt:            req.Prompt,
		Duration:          taskcommon.DefaultInt(req.Duration, 5),
		Resolution:        taskcommon.DefaultString(req.Size, "1080p"),
		MovementAmplitude: "auto",
		Bgm:               false,
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	return &r, nil
}

// ParseTaskResult 解析 Vidu 任务轮询的响应数据。
// 将 Vidu 的任务状态映射为内部统一状态：
//   - created/queueing -> TaskStatusSubmitted（已提交）
//   - processing -> TaskStatusInProgress（处理中）
//   - success -> TaskStatusSuccess（成功），提取第一个生成内容的URL
//   - failed -> TaskStatusFailure（失败），提取错误码作为原因
//
// 参数：
//   - respBody: Vidu 任务查询接口的响应体字节数据
//
// 返回：统一的任务信息结构体和可能的解析错误
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}

	var taskResp taskResultResponse
	err := common.Unmarshal(respBody, &taskResp)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}

	state := taskResp.State
	switch state {
	case "created", "queueing":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "success":
		taskInfo.Status = model.TaskStatusSuccess
		if len(taskResp.Creations) > 0 {
			taskInfo.Url = taskResp.Creations[0].URL
		}
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
		if taskResp.ErrCode != "" {
			taskInfo.Reason = taskResp.ErrCode
		}
	default:
		return nil, fmt.Errorf("unknown task state: %s", state)
	}

	return taskInfo, nil
}

// ConvertToOpenAIVideo 将存储的 Vidu 任务数据转换为 OpenAI 兼容的视频响应格式。
// 从数据库中的原始任务数据反序列化 Vidu 响应，构建 OpenAI 格式的视频对象，
// 包含任务ID、状态、进度、视频URL和错误信息。
//
// 参数：
//   - originTask: 数据库中的任务记录
//
// 返回：OpenAI 格式视频响应的 JSON 字节数据和可能的错误
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var viduResp taskResultResponse
	if err := common.Unmarshal(originTask.Data, &viduResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal vidu task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if len(viduResp.Creations) > 0 && viduResp.Creations[0].URL != "" {
		openAIVideo.SetMetadata("url", viduResp.Creations[0].URL)
	}

	if viduResp.State == "failed" && viduResp.ErrCode != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: viduResp.ErrCode,
			Code:    viduResp.ErrCode,
		}
	}

	return common.Marshal(openAIVideo)
}
