// kling - adaptor.go
// 可灵（Kling）视频生成任务适配器。
// 实现了 taskcommon.TaskAdaptor 接口，负责将统一的视频生成请求转换为可灵 API 格式。
// 支持文生视频（text2video）和图生视频（image2video）两种模式。
// 使用 JWT（HS256）签名认证机制，API 密钥格式为 "access_key|secret_key"。
// 支持运镜控制（CameraControl）、运动遮罩（DynamicMask）等高级视频生成参数。
package kling

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"

	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/pkg/errors"

	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	taskcommon "github.com/c1cada/NexusTok/relay/channel/task/taskcommon"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"
)

// ============================
// 请求 / 响应数据结构
// ============================

// TrajectoryPoint 运动轨迹点坐标。
// 用于 DynamicMask 中描述遮罩区域的运动路径。
type TrajectoryPoint struct {
	X int `json:"x"` // X 坐标
	Y int `json:"y"` // Y 坐标
}

// DynamicMask 动态遮罩配置。
// 用于控制视频中特定区域的运动行为，支持遮罩图片和运动轨迹。
type DynamicMask struct {
	Mask         string            `json:"mask,omitempty"`         // Base64 编码的遮罩图片
	Trajectories []TrajectoryPoint `json:"trajectories,omitempty"` // 运动轨迹点序列
}

// CameraConfig 运镜配置参数。
// 定义摄像机在六个自由度上的运动幅度。
type CameraConfig struct {
	Horizontal float64 `json:"horizontal,omitempty"` // 水平平移（左/右）
	Vertical   float64 `json:"vertical,omitempty"`   // 垂直平移（上/下）
	Pan        float64 `json:"pan,omitempty"`        // 水平旋转（摇镜头）
	Tilt       float64 `json:"tilt,omitempty"`       // 垂直旋转（俯仰）
	Roll       float64 `json:"roll,omitempty"`       // 翻滚旋转
	Zoom       float64 `json:"zoom,omitempty"`       // 缩放
}

// CameraControl 运镜控制结构体。
// 包含运镜类型和具体的运镜参数配置。
type CameraControl struct {
	Type   string        `json:"type,omitempty"`   // 运镜类型
	Config *CameraConfig `json:"config,omitempty"` // 运镜参数配置
}

// requestPayload 可灵视频生成 API 的请求体结构体。
// 包含提示词、图片、生成模式、时长、宽高比、模型名称等参数。
type requestPayload struct {
	Prompt         string         `json:"prompt,omitempty"`          // 文本提示词
	Image          string         `json:"image,omitempty"`           // 首帧图片（Base64 或 URL，用于图生视频）
	ImageTail      string         `json:"image_tail,omitempty"`      // 尾帧图片（用于首尾帧生视频）
	NegativePrompt string         `json:"negative_prompt,omitempty"` // 反向提示词
	Mode           string         `json:"mode,omitempty"`            // 生成模式: "std"（标准）/ "pro"（专业）
	Duration       string         `json:"duration,omitempty"`        // 视频时长（秒），字符串格式
	AspectRatio    string         `json:"aspect_ratio,omitempty"`    // 宽高比: "1:1"、"16:9"、"9:16"
	ModelName      string         `json:"model_name,omitempty"`      // 模型名称（可灵格式）
	Model          string         `json:"model,omitempty"`           // 模型名称（兼容仅识别 "model" 的上游）
	CfgScale       float64        `json:"cfg_scale,omitempty"`       // 引导系数（控制生成与提示词的匹配程度）
	StaticMask     string         `json:"static_mask,omitempty"`     // 静态遮罩（Base64 编码）
	DynamicMasks   []DynamicMask  `json:"dynamic_masks,omitempty"`   // 动态遮罩列表
	CameraControl  *CameraControl `json:"camera_control,omitempty"`  // 运镜控制
	CallbackUrl    string         `json:"callback_url,omitempty"`    // 回调通知URL
	ExternalTaskId string         `json:"external_task_id,omitempty"` // 外部任务ID
}

// responsePayload 可灵 API 的响应结构体。
// 包含任务状态、结果视频列表和使用量信息。
type responsePayload struct {
	Code      int    `json:"code"`       // 响应状态码（0 表示成功）
	Message   string `json:"message"`    // 响应消息
	TaskId    string `json:"task_id"`    // 任务唯一标识（顶层）
	RequestId string `json:"request_id"` // 请求唯一标识
	Data      struct {
		TaskId        string `json:"task_id"`          // 任务唯一标识（data 层）
		TaskStatus    string `json:"task_status"`      // 任务状态: submitted/processing/succeed/failed
		TaskStatusMsg string `json:"task_status_msg"`  // 任务状态描述消息
		TaskInfo      struct {
			ExternalTaskId string `json:"external_task_id"` // 外部任务ID
		} `json:"task_info"`                              // 任务信息
		WatermarkInfo struct {
			Enabled bool `json:"enabled"` // 是否启用水印
		} `json:"watermark_info"`                         // 水印信息
		TaskResult struct {
			Videos []struct {
				Id           string `json:"id"`            // 视频唯一标识
				Url          string `json:"url"`           // 视频下载URL
				WatermarkUrl string `json:"watermark_url"` // 带水印的视频URL
				Duration     string `json:"duration"`      // 视频时长
			} `json:"videos"`                             // 生成的视频列表
			Images []struct {
				Index        int    `json:"index"`         // 图片序号
				Url          string `json:"url"`           // 图片URL
				WatermarkUrl string `json:"watermark_url"` // 带水印的图片URL
			} `json:"images"`                             // 生成的图片列表
		} `json:"task_result"`                            // 任务结果
		CreatedAt          int64  `json:"created_at"`          // 任务创建时间（Unix 时间戳）
		UpdatedAt          int64  `json:"updated_at"`          // 任务最后更新时间（Unix 时间戳）
		FinalUnitDeduction string `json:"final_unit_deduction"` // 最终扣除的积分数量（字符串格式）
	} `json:"data"`                                       // 任务数据
}

// ============================
// 适配器实现
// ============================

// TaskAdaptor 可灵（Kling）视频生成任务适配器。
// 实现 taskcommon.TaskAdaptor 接口，负责与可灵 API 交互。
// 继承 BaseBilling 以获得基础计费功能。
type TaskAdaptor struct {
	taskcommon.BaseBilling           // 嵌入基础计费功能
	ChannelType int                  // 渠道类型常量
	apiKey      string               // 可灵 API 密钥（格式: "access_key|secret_key"）
	baseURL     string               // 可灵 API 基础URL
}

// Init 初始化适配器，从中继信息中提取渠道类型、API 基础URL 和密钥。
// 可灵 API 密钥格式为 "access_key|secret_key"，使用 "|" 分隔。
//
// 参数：
//   - info: 中继上下文信息，包含渠道配置和认证信息
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey

	// apiKey format: "access_key|secret_key"
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
	// Use the standard validation method for TaskSubmitReq
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL 构建可灵视频生成 API 的请求URL。
// 根据任务动作选择不同的端点路径：
//   - TaskActionGenerate（图生视频）: /v1/videos/image2video
//   - 其他（文生视频）: /v1/videos/text2video
//
// 如果通过 NexusTok 中继访问，URL 中会包含 "/kling" 路径前缀。
//
// 参数：
//   - info: 中继上下文信息
//
// 返回：完整的API请求URL和可能的错误
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	path := lo.Ternary(info.Action == constant.TaskActionGenerate, "/v1/videos/image2video", "/v1/videos/text2video")

	if isNexusTokRelay(info.ApiKey) {
		return fmt.Sprintf("%s/kling%s", a.baseURL, path), nil
	}

	return fmt.Sprintf("%s%s", a.baseURL, path), nil
}

// BuildRequestHeader 设置可灵 API 所需的请求头。
// 使用 JWT（HS256）签名生成 Bearer Token 进行认证。
//
// 参数：
//   - c: Gin 上下文
//   - req: 即将发送的 HTTP 请求对象
//   - info: 中继上下文信息
//
// 返回：设置头信息过程中的错误，nil 表示成功
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	token, err := a.createJWTToken()
	if err != nil {
		return fmt.Errorf("failed to create JWT token: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "kling-sdk/1.0")
	return nil
}

// BuildRequestBody 构建发送给可灵 API 的请求体。
// 从 Gin context 中获取已解析的任务请求，转换为可灵特定格式后序列化为 JSON。
// 如果请求中没有图片输入，自动将动作切换为 TaskActionTextGenerate（文生视频）。
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
	if body.Image == "" && body.ImageTail == "" {
		c.Set("action", constant.TaskActionTextGenerate)
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest 发送 HTTP 请求到可灵 API。
// 委托给 channel.DoTaskApiRequest 通用方法处理实际请求发送。
// 如果在 BuildRequestBody 阶段切换了动作类型，会同步更新 info.Action。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//   - requestBody: 请求体 Reader
//
// 返回：HTTP 响应和可能的错误
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	if action := c.GetString("action"); action != "" {
		info.Action = action
	}
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 处理可灵 API 的响应。
// 解析响应体获取任务ID，检查响应状态码（0 表示成功），
// 构建 OpenAI 兼容格式的初始响应返回给客户端。
//
// 参数：
//   - c: Gin 上下文
//   - resp: 可灵 API 的 HTTP 响应
//   - info: 中继上下文信息
//
// 返回：
//   - taskID: 可灵任务ID（用于后续轮询）
//   - taskData: 原始响应数据（用于存储）
//   - taskErr: 任务错误信息（nil 表示成功）
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}

	var kResp responsePayload
	err = common.Unmarshal(responseBody, &kResp)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_failed", http.StatusInternalServerError)
		return
	}
	if kResp.Code != 0 {
		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("%s", kResp.Message), "task_failed", http.StatusBadRequest)
		return
	}
	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return kResp.Data.TaskId, responseBody, nil
}

// FetchTask 向可灵查询异步任务的当前状态。
// 根据任务动作选择不同的端点路径，通过 GET 请求获取任务最新状态。
// 使用 JWT 签名认证。
//
// 参数：
//   - baseUrl: 可灵 API 基础URL
//   - key: API 密钥（格式为 "access_key|secret_key" 或 NexusTok 中继密钥）
//   - body: 包含 task_id 和 action 的请求参数 map
//   - proxy: 代理地址（可为空）
//
// 返回：HTTP 响应和可能的错误
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	action, ok := body["action"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid action")
	}
	path := lo.Ternary(action == constant.TaskActionGenerate, "/v1/videos/image2video", "/v1/videos/text2video")
	url := fmt.Sprintf("%s%s/%s", baseUrl, path, taskID)
	if isNexusTokRelay(key) {
		url = fmt.Sprintf("%s/kling%s/%s", baseUrl, path, taskID)
	}

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	token, err := a.createJWTTokenWithKey(key)
	if err != nil {
		token = key
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("User-Agent", "kling-sdk/1.0")

	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// GetModelList 返回可灵支持的视频生成模型列表。
//
// 返回：支持的模型名称切片（"kling-v1"、"kling-v1-6"、"kling-v2-master"）
func (a *TaskAdaptor) GetModelList() []string {
	return []string{"kling-v1", "kling-v1-6", "kling-v2-master"}
}

// GetChannelName 返回可灵视频生成渠道的唯一标识名称。
//
// 返回：渠道名称字符串（"kling"）
func (a *TaskAdaptor) GetChannelName() string {
	return "kling"
}

// ============================
// 辅助函数
// ============================

// convertToRequestPayload 将统一的 TaskSubmitReq 转换为可灵 API 的请求格式。
// 设置默认值：模式为 "std"（标准），时长为 5 秒，宽高比根据图片尺寸自动计算。
// 从 metadata 中解析额外参数（运镜控制、遮罩等）。
//
// 参数：
//   - req: 统一的任务提交请求
//   - info: 中继上下文信息
//
// 返回：可灵格式的请求和可能的错误
func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	r := requestPayload{
		Prompt:         req.Prompt,
		Image:          req.Image,
		Mode:           taskcommon.DefaultString(req.Mode, "std"),
		Duration:       fmt.Sprintf("%d", taskcommon.DefaultInt(req.Duration, 5)),
		AspectRatio:    a.getAspectRatio(req.Size),
		ModelName:      info.UpstreamModelName,
		Model:          info.UpstreamModelName,
		CfgScale:       0.5,
		StaticMask:     "",
		DynamicMasks:   []DynamicMask{},
		CameraControl:  nil,
		CallbackUrl:    "",
		ExternalTaskId: "",
	}
	if r.ModelName == "" {
		r.ModelName = "kling-v1"
		r.Model = "kling-v1"
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}
	return &r, nil
}

// getAspectRatio 根据图片尺寸字符串计算可灵 API 的宽高比标签。
//
// 转换规则：
//   - 正方形（1024x1024、512x512）-> "1:1"
//   - 横屏（1280x720、1920x1080）-> "16:9"
//   - 竖屏（720x1280、1080x1920）-> "9:16"
//   - 其他 -> "1:1"（默认正方形）
//
// 参数：
//   - size: 尺寸字符串（如 "1920x1080"）
//
// 返回：宽高比字符串
func (a *TaskAdaptor) getAspectRatio(size string) string {
	switch size {
	case "1024x1024", "512x512":
		return "1:1"
	case "1280x720", "1920x1080":
		return "16:9"
	case "720x1280", "1080x1920":
		return "9:16"
	default:
		return "1:1"
	}
}

// ============================
// JWT 辅助函数
// ============================

// createJWTToken 使用当前适配器的 API 密钥创建 JWT Token。
// 委托给 createJWTTokenWithKey 方法。
//
// 返回：JWT Token 字符串和可能的错误
func (a *TaskAdaptor) createJWTToken() (string, error) {
	return a.createJWTTokenWithKey(a.apiKey)
}

// createJWTTokenWithKey 使用指定的 API 密钥创建 JWT Token。
// 如果是 NexusTok 中继密钥，直接返回密钥本身（不需要 JWT 签名）。
// 否则解析 "access_key|secret_key" 格式，使用 HS256 算法生成 JWT Token。
// Token 有效期为 30 分钟。
//
// 参数：
//   - apiKey: API 密钥字符串
//
// 返回：JWT Token 字符串和可能的错误
func (a *TaskAdaptor) createJWTTokenWithKey(apiKey string) (string, error) {
	if isNexusTokRelay(apiKey) {
		return apiKey, nil // NexusTok relay
	}
	keyParts := strings.Split(apiKey, "|")
	if len(keyParts) != 2 {
		return "", errors.New("invalid api_key, required format is accessKey|secretKey")
	}
	accessKey := strings.TrimSpace(keyParts[0])
	if len(keyParts) == 1 {
		return accessKey, nil
	}
	secretKey := strings.TrimSpace(keyParts[1])
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"iss": accessKey,
		"exp": now + 1800, // 30 minutes
		"nbf": now - 5,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["typ"] = "JWT"
	return token.SignedString([]byte(secretKey))
}

// ParseTaskResult 解析可灵任务轮询的响应数据。
// 将可灵的任务状态（submitted/processing/succeed/failed）映射为内部统一状态。
// 任务成功时提取第一个视频的URL和扣除的积分数量。
//
// 参数：
//   - respBody: 可灵任务查询接口的响应体字节数据
//
// 返回：统一的任务信息结构体和可能的解析错误
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	taskInfo := &relaycommon.TaskInfo{}
	resPayload := responsePayload{}
	err := common.Unmarshal(respBody, &resPayload)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal response body")
	}
	taskInfo.Code = resPayload.Code
	taskInfo.TaskID = resPayload.Data.TaskId
	taskInfo.Reason = resPayload.Data.TaskStatusMsg
	//任务状态，枚举值：submitted（已提交）、processing（处理中）、succeed（成功）、failed（失败）
	status := resPayload.Data.TaskStatus
	switch status {
	case "submitted":
		taskInfo.Status = model.TaskStatusSubmitted
	case "processing":
		taskInfo.Status = model.TaskStatusInProgress
	case "succeed":
		taskInfo.Status = model.TaskStatusSuccess
		if videos := resPayload.Data.TaskResult.Videos; len(videos) > 0 {
			video := videos[0]
			taskInfo.Url = video.Url
		}
		if tokens, err := strconv.ParseFloat(resPayload.Data.FinalUnitDeduction, 64); err == nil {
			// 上游扣费积分会进入异步任务重算链路。这里先保持可灵“向上取整”的
			// 历史语义，再用统一 quota 饱和转换防止异常大值在 int 转换时回绕。
			rounded := common.QuotaFromFloat(math.Ceil(tokens))
			if rounded > 0 {
				taskInfo.CompletionTokens = rounded
				taskInfo.TotalTokens = rounded
			}
		}
	case "failed":
		taskInfo.Status = model.TaskStatusFailure
	default:
		return nil, fmt.Errorf("unknown task status: %s", status)
	}
	return taskInfo, nil
}

// isNexusTokRelay 判断 API 密钥是否为 NexusTok 中继格式。
// NexusTok 中继密钥以 "sk-" 开头，使用简化的 Bearer Token 认证。
//
// 参数：
//   - apiKey: API 密钥字符串
//
// 返回：true 表示是 NexusTok 中继密钥
func isNexusTokRelay(apiKey string) bool {
	return strings.HasPrefix(apiKey, "sk-")
}

// ConvertToOpenAIVideo 将存储的可灵任务数据转换为 OpenAI 兼容的视频响应格式。
// 从数据库中的原始任务数据反序列化可灵响应，构建 OpenAI 格式的视频对象，
// 包含任务ID、状态、进度、视频URL、时长和错误信息。
//
// 参数：
//   - originTask: 数据库中的任务记录
//
// 返回：OpenAI 格式视频响应的 JSON 字节数据和可能的错误
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var klingResp responsePayload
	if err := common.Unmarshal(originTask.Data, &klingResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal kling task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.CreatedAt = klingResp.Data.CreatedAt
	openAIVideo.CompletedAt = klingResp.Data.UpdatedAt

	if len(klingResp.Data.TaskResult.Videos) > 0 {
		video := klingResp.Data.TaskResult.Videos[0]
		if video.Url != "" {
			openAIVideo.SetMetadata("url", video.Url)
		}
		if video.Duration != "" {
			openAIVideo.Seconds = video.Duration
		}
	}

	if klingResp.Code != 0 && klingResp.Message != "" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: klingResp.Message,
			Code:    fmt.Sprintf("%d", klingResp.Code),
		}
	}

	// https://app.klingai.com/cn/dev/document-api/apiReference/model/textToVideo
	if data := klingResp.Data; data.TaskStatus == "failed" {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: data.TaskStatusMsg,
		}
	}
	return common.Marshal(openAIVideo)
}
