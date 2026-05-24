// suno - adaptor.go
// Suno AI 音乐生成任务适配器。
// 实现了 taskcommon.TaskAdaptor 接口，负责将统一的音乐生成请求转换为 Suno API 格式。
// 支持两种动作：MUSIC（音乐生成）和 LYRICS（歌词生成）。
// Suno 使用批量轮询机制（/suno/fetch）而非逐任务轮询，因此 ParseTaskResult 不适用。
// 任务结果的获取通过 service.UpdateSunoTasks 专用路径处理。
package suno

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/constant"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/relay/channel"
	taskcommon "github.com/c1cada/NexusTok/relay/channel/task/taskcommon"
	relaycommon "github.com/c1cada/NexusTok/relay/common"
	"github.com/c1cada/NexusTok/service"

	"github.com/gin-gonic/gin"
)

// TaskAdaptor Suno AI 音乐生成任务适配器。
// 实现 taskcommon.TaskAdaptor 接口，负责与 Suno API 交互。
// 继承 BaseBilling 以获得基础计费功能。
type TaskAdaptor struct {
	taskcommon.BaseBilling           // 嵌入基础计费功能
	ChannelType int                  // 渠道类型常量
}

// ParseTaskResult 解析任务结果（Suno 不适用）。
// Suno 使用批量轮询机制（通过 service.UpdateSunoTasks）而非逐任务轮询。
// 批量轮询接收 dto.TaskResponse[[]dto.SunoDataResponse] 格式的响应，
// 与视频适配器使用的逐任务轮询机制不同。
//
// 参数：
//   - 未使用
//
// 返回：始终返回错误，提示 Suno 不使用此方法
func (a *TaskAdaptor) ParseTaskResult([]byte) (*relaycommon.TaskInfo, error) {
	return nil, fmt.Errorf("suno uses batch polling via UpdateSunoTasks, ParseTaskResult is not applicable")
}

// Init 初始化适配器，从中继信息中提取渠道类型。
// Suno 适配器不需要存储 API 密钥和基础URL，因为它们在各方法中通过 info 参数传递。
//
// 参数：
//   - info: 中继上下文信息，包含渠道配置
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
}

// ValidateRequestAndSetAction 验证请求并设置任务动作。
// 从 URL 路径参数中提取动作类型（MUSIC 或 LYRICS），
// 解析请求体为 SunoSubmitReq 格式，验证动作合法性。
// 将解析后的请求存入 Gin context 供后续使用。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：验证错误信息，nil 表示验证通过
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	action := strings.ToUpper(c.Param("action"))

	var sunoRequest *dto.SunoSubmitReq
	err := common.UnmarshalBodyReusable(c, &sunoRequest)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}
	err = actionValidate(c, sunoRequest, action)
	if err != nil {
		taskErr = service.TaskErrorWrapperLocal(err, "invalid_request", http.StatusBadRequest)
		return
	}

	//if sunoRequest.ContinueClipId != "" {
	//	if sunoRequest.TaskID == "" {
	//		taskErr = service.TaskErrorWrapperLocal(fmt.Errorf("task id is empty"), "invalid_request", http.StatusBadRequest)
	//		return
	//	}
	//	info.OriginTaskID = sunoRequest.TaskID
	//}

	info.Action = action
	c.Set("task_request", sunoRequest)
	return nil
}

// BuildRequestURL 构建 Suno 音乐生成 API 的请求URL。
// 端点格式: {baseURL}/suno/submit/{action}
// action 为动作类型（如 "MUSIC"、"LYRICS"）。
//
// 参数：
//   - info: 中继上下文信息
//
// 返回：完整的API请求URL和可能的错误
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	baseURL := info.ChannelBaseUrl
	fullRequestURL := fmt.Sprintf("%s%s", baseURL, "/suno/submit/"+info.Action)
	return fullRequestURL, nil
}

// BuildRequestHeader 设置 Suno API 所需的请求头。
// Content-Type 和 Accept 透传原始请求的类型，使用 Bearer Token 认证。
//
// 参数：
//   - c: Gin 上下文
//   - req: 即将发送的 HTTP 请求对象
//   - info: 中继上下文信息
//
// 返回：设置头信息过程中的错误，nil 表示成功
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", c.Request.Header.Get("Content-Type"))
	req.Header.Set("Accept", c.Request.Header.Get("Accept"))
	req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	return nil
}

// BuildRequestBody 构建发送给 Suno API 的请求体。
// 从 Gin context 中获取已解析的 SunoSubmitReq 请求，序列化为 JSON。
//
// 参数：
//   - c: Gin 上下文
//   - info: 中继上下文信息
//
// 返回：请求体 Reader 和可能的错误
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	sunoRequest, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("task_request not found in context")
	}
	data, err := common.Marshal(sunoRequest)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest 发送 HTTP 请求到 Suno API。
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

// DoResponse 处理 Suno API 的响应。
// 解析响应体获取上游任务ID，检查响应是否成功。
// 使用公开的 task_xxxx ID 替换上游ID后返回给客户端。
// 返回的 taskData 为 nil，因为 Suno 使用批量轮询机制。
//
// 参数：
//   - c: Gin 上下文
//   - resp: Suno API 的 HTTP 响应
//   - info: 中继上下文信息
//
// 返回：
//   - taskID: Suno 任务ID（用于后续批量轮询）
//   - taskData: nil（Suno 不存储单任务响应数据）
//   - taskErr: 任务错误信息（nil 表示成功）
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	var sunoResponse dto.TaskResponse[string]
	err = common.Unmarshal(responseBody, &sunoResponse)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}
	if !sunoResponse.IsSuccess() {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", sunoResponse.Message), sunoResponse.Code, http.StatusInternalServerError)
		return
	}

	// 使用公开 task_xxxx ID 替换上游 ID 返回给客户端
	publicResponse := dto.TaskResponse[string]{
		Code:    sunoResponse.Code,
		Message: sunoResponse.Message,
		Data:    info.PublicTaskID,
	}
	c.JSON(http.StatusOK, publicResponse)

	return sunoResponse.Data, nil, nil
}

// GetModelList 返回 Suno 支持的音乐生成模型列表。
//
// 返回：支持的模型名称切片
func (a *TaskAdaptor) GetModelList() []string {
	return ModelList
}

// GetChannelName 返回 Suno 音乐生成渠道的唯一标识名称。
//
// 返回：渠道名称字符串
func (a *TaskAdaptor) GetChannelName() string {
	return ChannelName
}

// FetchTask 向 Suno 查询任务状态（批量模式）。
// 通过 POST /suno/fetch 接口批量查询任务状态。
// Suno 使用批量查询而非逐任务查询，body 中包含所有需要查询的任务ID。
//
// 参数：
//   - baseUrl: Suno API 基础URL
//   - key: API 密钥
//   - body: 包含任务ID列表的请求参数 map
//   - proxy: 代理地址（可为空）
//
// 返回：HTTP 响应和可能的错误
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	requestUrl := fmt.Sprintf("%s/suno/fetch", baseUrl)
	byteBody, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest("POST", requestUrl, bytes.NewBuffer(byteBody))
	if err != nil {
		common.SysLog(fmt.Sprintf("Get Task error: %v", err))
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// actionValidate 验证 Suno 请求的动作类型和必要字段。
//
// 验证规则：
//   - MUSIC 动作：如果未指定 mv（模型版本），默认设置为 "chirp-v3-0"
//   - LYRICS 动作：prompt 字段必须非空
//   - 其他动作：返回 "invalid_action" 错误
//
// 参数：
//   - c: Gin 上下文
//   - sunoRequest: Suno 提交请求
//   - action: 动作类型字符串（"MUSIC" 或 "LYRICS"）
//
// 返回：验证错误，nil 表示验证通过
func actionValidate(c *gin.Context, sunoRequest *dto.SunoSubmitReq, action string) (err error) {
	switch action {
	case constant.SunoActionMusic:
		if sunoRequest.Mv == "" {
			sunoRequest.Mv = "chirp-v3-0"
		}
	case constant.SunoActionLyrics:
		if sunoRequest.Prompt == "" {
			err = fmt.Errorf("prompt_empty")
			return
		}
	default:
		err = fmt.Errorf("invalid_action")
	}
	return
}
