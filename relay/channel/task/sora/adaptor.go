// sora - adaptor.go
// Sora（OpenAI）视频生成任务适配器。
// 实现了 taskcommon.TaskAdaptor 接口，负责将统一的视频生成请求转换为 Sora API 格式。
// 支持文生视频、图生视频和视频混编（remix）三种模式。
// Sora API 使用 multipart/form-data 或 application/json 两种请求格式。
// 支持代理透传原始请求体，并在转发时替换模型名称。
package sora

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strconv"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/relay/channel"
	taskcommon "github.com/c1cada/NexusTok/relay/channel/task/taskcommon"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"

	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
	"github.com/tidwall/sjson"
)

// ============================
// 请求 / 响应数据结构
// ============================

// ContentItem Sora API 的内容项结构体。
// 用于 JSON 格式请求中的多模态内容（文本或图片）。
type ContentItem struct {
	Type     string    `json:"type"`                // 内容类型: "text" 或 "image_url"
	Text     string    `json:"text,omitempty"`      // 文本内容（当 type 为 "text" 时使用）
	ImageURL *ImageURL `json:"image_url,omitempty"` // 图片URL（当 type 为 "image_url" 时使用）
}

// ImageURL 图片URL包装结构体。
// 用于在 ContentItem 中嵌套引用图片资源。
type ImageURL struct {
	URL string `json:"url"` // 图片资源的URL地址
}

// responseTask Sora 任务响应结构体。
// 包含任务的完整状态信息、进度、生成参数和错误信息。
// 兼容新旧两种接口格式（id 和 task_id 字段）。
type responseTask struct {
	ID                 string `json:"id"`                            // 任务唯一标识（新格式）
	TaskID             string `json:"task_id,omitempty"`             // 任务唯一标识（旧格式兼容）
	Object             string `json:"object"`                        // 对象类型
	Model              string `json:"model"`                         // 使用的模型名称
	Status             string `json:"status"`                        // 任务状态: queued/pending/processing/in_progress/completed/failed/cancelled
	Progress           int    `json:"progress"`                      // 任务进度（0-100）
	CreatedAt          int64  `json:"created_at"`                    // 任务创建时间（Unix 时间戳）
	CompletedAt        int64  `json:"completed_at,omitempty"`        // 任务完成时间（Unix 时间戳）
	ExpiresAt          int64  `json:"expires_at,omitempty"`          // 任务过期时间（Unix 时间戳）
	Seconds            string `json:"seconds,omitempty"`             // 视频时长（秒）
	Size               string `json:"size,omitempty"`                // 视频尺寸
	RemixedFromVideoID string `json:"remixed_from_video_id,omitempty"` // 混编来源视频ID
	Error              *struct {
		Message string `json:"message"` // 错误消息
		Code    string `json:"code"`    // 错误码
	} `json:"error,omitempty"`                                       // 错误信息（仅在任务失败时有值）
}

// ============================
// 适配器实现
// ============================

// TaskAdaptor Sora（OpenAI）视频生成任务适配器。
// 实现 taskcommon.TaskAdaptor 接口，负责与 Sora API 交互。
// 继承 BaseBilling 以获得基础计费功能。
type TaskAdaptor struct {
	taskcommon.BaseBilling           // 嵌入基础计费功能
	ChannelType int                  // 渠道类型常量
	apiKey      string               // Sora API 密钥（Bearer Token）
	baseURL     string               // Sora API 基础URL
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

// validateRemixRequest 验证视频混编（remix）请求。
// 解析请求体并验证 prompt 字段是否非空。
// 将解析后的请求存入 Gin context 供后续使用。
//
// 参数：
//   - c: Gin 上下文
//
// 返回：验证错误信息，nil 表示验证通过
func validateRemixRequest(c *gin.Context) *dto.TaskError {
	var req relaycommon.TaskSubmitReq
	if err := common.UnmarshalBodyReusable(c, &req); err != nil {
		return service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return service.TaskErrorWrapperLocal(fmt.Errorf("field prompt is required"), "invalid_request", http.StatusBadRequest)
	}
	// 存储原始请求到 context，与 ValidateMultipartDirect 路径保持一致
	c.Set("task_request", req)
	return nil
}

// ValidateRequestAndSetAction 验证请求并设置任务动作。
// 根据请求的动作类型选择不同的验证方式：
//   - TaskActionRemix: 使用 validateRemixRequest 验证混编请求
//   - 其他: 使用 ValidateMultipartDirect 验证 multipart 表单请求
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：验证错误信息，nil 表示验证通过
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	if info.Action == constant.TaskActionRemix {
		return validateRemixRequest(c)
	}
	return relaycommon.ValidateMultipartDirect(c, info)
}

// EstimateBilling 根据用户请求的视频时长和尺寸计算计费倍率。
// remix 模式的 OtherRatios 已在 ResolveOriginTask 中设置，此处不再计算。
// 非标准尺寸（1792x1024 或 1024x1792）有额外的 1.666667 倍率。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：其他计费倍率 map（包含 "seconds" 和 "size" 键），nil 表示无额外倍率
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	// remix 路径的 OtherRatios 已在 ResolveOriginTask 中设置
	if info.Action == constant.TaskActionRemix {
		return nil
	}

	req, err := relaycommon.GetTaskRequest(c)
	if err != nil {
		return nil
	}

	seconds, _ := strconv.Atoi(req.Seconds)
	if seconds == 0 {
		seconds = req.Duration
	}
	if seconds <= 0 {
		seconds = 4
	}

	size := req.Size
	if size == "" {
		size = "720x1280"
	}

	ratios := map[string]float64{
		"seconds": float64(seconds),
		"size":    1,
	}
	if size == "1792x1024" || size == "1024x1792" {
		ratios["size"] = 1.666667
	}
	return ratios
}

// BuildRequestURL 构建 Sora 视频生成 API 的请求URL。
// 根据任务动作选择不同的端点路径：
//   - TaskActionRemix: /v1/videos/{originTaskID}/remix
//   - 其他: /v1/videos
//
// 参数：
//   - info: 中继上下文信息
//
// 返回：完整的API请求URL和可能的错误
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if info.Action == constant.TaskActionRemix {
		return fmt.Sprintf("%s/v1/videos/%s/remix", a.baseURL, info.OriginTaskID), nil
	}
	return fmt.Sprintf("%s/v1/videos", a.baseURL), nil
}

// BuildRequestHeader 设置 Sora API 所需的请求头。
// 使用 Bearer Token 认证，Content-Type 透传原始请求的类型。
//
// 参数：
//   - c: Gin 上下文
//   - req: 即将发送的 HTTP 请求对象
//   - info: 中继上下文信息
//
// 返回：设置头信息过程中的错误，nil 表示成功
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Authorization", "Bearer "+a.apiKey)
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	return nil
}

// BuildRequestBody 构建发送给 Sora API 的请求体。
// 支持两种请求格式：
//   - application/json: 解析 JSON 请求体，替换模型名称后重新序列化
//   - multipart/form-data: 解析表单数据，替换模型名称后重新构建 multipart 请求
//     - 自动检测文件的 Content-Type（如果未指定或为 application/octet-stream）
//     - 保留所有原始表单字段和文件
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：请求体 Reader 和可能的错误
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		return nil, errors.Wrap(err, "get_request_body_failed")
	}
	cachedBody, err := storage.Bytes()
	if err != nil {
		return nil, errors.Wrap(err, "read_body_bytes_failed")
	}
	contentType := c.GetHeader("Content-Type")

	if strings.HasPrefix(contentType, "application/json") {
		var bodyMap map[string]interface{}
		if err := common.Unmarshal(cachedBody, &bodyMap); err == nil {
			bodyMap["model"] = info.UpstreamModelName
			if newBody, err := common.Marshal(bodyMap); err == nil {
				return bytes.NewReader(newBody), nil
			}
		}
		return bytes.NewReader(cachedBody), nil
	}

	if strings.Contains(contentType, "multipart/form-data") {
		formData, err := common.ParseMultipartFormReusable(c)
		if err != nil {
			return bytes.NewReader(cachedBody), nil
		}
		var buf bytes.Buffer
		writer := multipart.NewWriter(&buf)
		writer.WriteField("model", info.UpstreamModelName)
		for key, values := range formData.Value {
			if key == "model" {
				continue
			}
			for _, v := range values {
				writer.WriteField(key, v)
			}
		}
		for fieldName, fileHeaders := range formData.File {
			for _, fh := range fileHeaders {
				f, err := fh.Open()
				if err != nil {
					continue
				}
				ct := fh.Header.Get("Content-Type")
				if ct == "" || ct == "application/octet-stream" {
					buf512 := make([]byte, 512)
					n, _ := io.ReadFull(f, buf512)
					ct = http.DetectContentType(buf512[:n])
					// Re-open after sniffing so the full content is copied below
					f.Close()
					f, err = fh.Open()
					if err != nil {
						continue
					}
				}
				h := make(textproto.MIMEHeader)
				h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, fieldName, fh.Filename))
				h.Set("Content-Type", ct)
				part, err := writer.CreatePart(h)
				if err != nil {
					f.Close()
					continue
				}
				io.Copy(part, f)
				f.Close()
			}
		}
		writer.Close()
		c.Request.Header.Set("Content-Type", writer.FormDataContentType())
		return &buf, nil
	}

	return common.ReaderOnly(storage), nil
}

// DoRequest 发送 HTTP 请求到 Sora API。
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

// DoResponse 处理 Sora API 的响应。
// 解析响应体获取任务ID（兼容 id 和 task_id 两种字段），
// 使用公开的 task_xxxx ID 替换上游ID后返回给客户端。
//
// 参数：
//   - c: Gin 上下文
//   - resp: Sora API 的 HTTP 响应
//   - info: 中继上下文信息
//
// 返回：
//   - taskID: Sora 任务ID（用于后续轮询）
//   - taskData: 原始响应数据（用于存储）
//   - taskErr: 任务错误信息（nil 表示成功）
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Sora response
	var dResp responseTask
	if err := common.Unmarshal(responseBody, &dResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	upstreamID := dResp.ID
	if upstreamID == "" {
		upstreamID = dResp.TaskID
	}
	if upstreamID == "" {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("task_id is empty"), "invalid_response", http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 返回给客户端
	dResp.ID = info.PublicTaskID
	dResp.TaskID = info.PublicTaskID
	c.JSON(http.StatusOK, dResp)
	return upstreamID, responseBody, nil
}

// FetchTask 向 Sora 查询异步任务的当前状态。
// 通过 GET /v1/videos/{taskID} 接口获取任务最新状态。
//
// 参数：
//   - baseUrl: Sora API 基础URL
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

	uri := fmt.Sprintf("%s/v1/videos/%s", baseUrl, taskID)

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

// GetModelList 返回 Sora 支持的视频生成模型列表。
//
// 返回：支持的模型名称切片
func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回 Sora 视频生成渠道的唯一标识名称。
//
// 返回：渠道名称字符串
func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// ParseTaskResult 解析 Sora 任务轮询的响应数据。
// 将 Sora 的任务状态映射为内部统一状态：
//   - queued/pending -> TaskStatusQueued（排队中）
//   - processing/in_progress -> TaskStatusInProgress（处理中）
//   - completed -> TaskStatusSuccess（成功）
//   - failed/cancelled -> TaskStatusFailure（失败）
//
// URL 字段故意留空，由调用方使用公开任务ID构建代理URL。
//
// 参数：
//   - respBody: Sora 任务查询接口的响应体字节数据
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

	switch resTask.Status {
	case "queued", "pending":
		taskResult.Status = model.TaskStatusQueued
	case "processing", "in_progress":
		taskResult.Status = model.TaskStatusInProgress
	case "completed":
		taskResult.Status = model.TaskStatusSuccess
		// Url intentionally left empty — the caller constructs the proxy URL using the public task ID
	case "failed", "cancelled":
		taskResult.Status = model.TaskStatusFailure
		if resTask.Error != nil {
			taskResult.Reason = resTask.Error.Message
		} else {
			taskResult.Reason = "task failed"
		}
	default:
	}
	if resTask.Progress > 0 && resTask.Progress < 100 {
		taskResult.Progress = fmt.Sprintf("%d%%", resTask.Progress)
	}

	return &taskResult, nil
}

// ConvertToOpenAIVideo 将存储的 Sora 任务数据转换为 OpenAI 兼容的视频响应格式。
// 使用 sjson 库直接修改原始 JSON 数据中的 id 字段为公开任务ID。
// 由于 Sora 响应格式本身与 OpenAI 格式高度兼容，只需替换 ID 即可。
//
// 参数：
//   - task: 数据库中的任务记录
//
// 返回：OpenAI 格式视频响应的 JSON 字节数据和可能的错误
func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	data := task.Data
	var err error
	if data, err = sjson.SetBytes(data, "id", task.TaskID); err != nil {
		return nil, errors.Wrap(err, "set id failed")
	}
	return data, nil
}
