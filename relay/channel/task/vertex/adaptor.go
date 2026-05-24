// Package vertex 实现 Google Vertex AI 视频生成渠道的任务适配器
// 支持 Veo 系列模型的视频生成
package vertex

import (
	"bytes"   // 字节缓冲区
	"fmt"     // 格式化输出
	"io"      // IO 操作
	"net/http" // HTTP 客户端
	"regexp"   // 正则表达式
	"strings"  // 字符串操作
	"time"     // 时间处理

	"github.com/c1cada/NexusTok/common" // 公共工具包
	"github.com/c1cada/NexusTok/model"   // 数据模型
	"github.com/gin-gonic/gin"           // Gin Web 框架

	"github.com/c1cada/NexusTok/constant"                           // 常量定义
	"github.com/c1cada/NexusTok/dto"                                // 数据传输对象
	"github.com/c1cada/NexusTok/relay/channel"                      // 渠道通用工具
	geminitask "github.com/c1cada/NexusTok/relay/channel/task/gemini" // Gemini 任务工具
	taskcommon "github.com/c1cada/NexusTok/relay/channel/task/taskcommon" // 任务通用工具
	vertexcore "github.com/c1cada/NexusTok/relay/channel/vertex"    // Vertex AI 核心工具
	relaycommon "github.com/c1cada/NexusTok/relay/common"          // 中继通用类型
	"github.com/c1cada/NexusTok/service"                            // 服务层
)

// ============================
// 请求/响应结构体
// ============================

// fetchOperationPayload 查询操作请求结构体
type fetchOperationPayload struct {
	OperationName string `json:"operationName"` // 操作名称
}

// submitResponse 提交响应结构体
type submitResponse struct {
	Name string `json:"name"` // 操作名称
}

// operationVideo 操作视频结构体
type operationVideo struct {
	MimeType           string `json:"mimeType"`           // MIME 类型
	BytesBase64Encoded string `json:"bytesBase64Encoded"` // Base64 编码的视频数据
	Encoding           string `json:"encoding"`           // 编码格式
}

// operationResponse 操作响应结构体
type operationResponse struct {
	Name     string `json:"name"` // 操作名称
	Done     bool   `json:"done"` // 是否完成
	Response struct {
		Type                  string           `json:"@type"`                // 响应类型
		RaiMediaFilteredCount int              `json:"raiMediaFilteredCount"` // RAI 过滤的媒体数量
		Videos                []operationVideo `json:"videos"`               // 视频列表
		BytesBase64Encoded    string           `json:"bytesBase64Encoded"`   // Base64 编码的视频数据
		Encoding              string           `json:"encoding"`             // 编码格式
		Video                 string           `json:"video"`                // 视频数据
	} `json:"response"` // 响应数据
	Error struct {
		Message string `json:"message"` // 错误消息
	} `json:"error"` // 错误信息
}

// ============================
// 适配器实现
// ============================

// TaskAdaptor Vertex AI 视频生成任务适配器
type TaskAdaptor struct {
	taskcommon.BaseBilling        // 嵌入基础计费实现
	ChannelType int               // 渠道类型
	apiKey      string            // API 密钥（JSON 格式的服务账号凭证）
	baseURL     string            // 基础 URL
}

// Init 初始化适配器
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl
	a.apiKey = info.ApiKey
}

// ValidateRequestAndSetAction 验证请求并设置操作类型
func (a *TaskAdaptor) ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) (taskErr *dto.TaskError) {
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionTextGenerate)
}

// BuildRequestURL 构建 Vertex AI 视频生成请求 URL
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	adc := &vertexcore.Credentials{}
	if err := common.Unmarshal([]byte(a.apiKey), adc); err != nil {
		return "", fmt.Errorf("failed to decode credentials: %w", err)
	}
	modelName := info.UpstreamModelName
	if modelName == "" {
		modelName = "veo-3.0-generate-001"
	}

	region := vertexcore.GetModelRegion(info.ApiVersion, modelName)
	if strings.TrimSpace(region) == "" {
		region = "global"
	}
	return vertexcore.BuildGoogleModelURL(a.baseURL, vertexcore.DefaultAPIVersion, adc.ProjectID, region, modelName, "predictLongRunning"), nil
}

// BuildRequestHeader 构建请求头
// 设置认证令牌和项目 ID
func (a *TaskAdaptor) BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	adc := &vertexcore.Credentials{}
	if err := common.Unmarshal([]byte(a.apiKey), adc); err != nil {
		return fmt.Errorf("failed to decode credentials: %w", err)
	}

	proxy := ""
	if info != nil {
		proxy = info.ChannelSetting.Proxy
	}
	token, err := vertexcore.AcquireAccessToken(*adc, proxy)
	if err != nil {
		return fmt.Errorf("failed to acquire access token: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-goog-user-project", adc.ProjectID)
	return nil
}

// EstimateBilling 估算计费
// 根据视频时长和分辨率计算计费比率
func (a *TaskAdaptor) EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64 {
	v, ok := c.Get("task_request")
	if !ok {
		return nil
	}
	req := v.(relaycommon.TaskSubmitReq)

	seconds := geminitask.ResolveVeoDuration(req.Metadata, req.Duration, req.Seconds)
	resolution := geminitask.ResolveVeoResolution(req.Metadata, req.Size)
	resRatio := geminitask.VeoResolutionRatio(info.UpstreamModelName, resolution)

	return map[string]float64{
		"seconds":    float64(seconds),
		"resolution": resRatio,
	}
}

// BuildRequestBody 构建请求体
// 将通用请求转换为 Vertex AI 视频生成格式
func (a *TaskAdaptor) BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error) {
	v, ok := c.Get("task_request")
	if !ok {
		return nil, fmt.Errorf("request not found in context")
	}
	req := v.(relaycommon.TaskSubmitReq)

	// 构建实例
	instance := geminitask.VeoInstance{Prompt: req.Prompt}
	if img := geminitask.ExtractMultipartImage(c, info); img != nil {
		instance.Image = img
	} else if len(req.Images) > 0 {
		if parsed := geminitask.ParseImageInput(req.Images[0]); parsed != nil {
			instance.Image = parsed
			info.Action = constant.TaskActionGenerate
		}
	}

	// 构建参数
	params := &geminitask.VeoParameters{}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, params); err != nil {
		return nil, fmt.Errorf("unmarshal metadata failed: %w", err)
	}
	// 从请求中提取参数
	if params.DurationSeconds == 0 && req.Duration > 0 {
		params.DurationSeconds = req.Duration
	}
	if params.Resolution == "" && req.Size != "" {
		params.Resolution = geminitask.SizeToVeoResolution(req.Size)
	}
	if params.AspectRatio == "" && req.Size != "" {
		params.AspectRatio = geminitask.SizeToVeoAspectRatio(req.Size)
	}
	params.Resolution = strings.ToLower(params.Resolution)
	params.SampleCount = 1

	body := geminitask.VeoRequestPayload{
		Instances:  []geminitask.VeoInstance{instance},
		Parameters: params,
	}

	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest 执行 API 请求
func (a *TaskAdaptor) DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error) {
	return channel.DoTaskApiRequest(a, c, info, requestBody)
}

// DoResponse 处理 API 响应
// 解析提交响应，返回操作名称作为任务 ID
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
	// 将操作名称编码为本地任务 ID
	localID := taskcommon.EncodeLocalTaskID(s.Name)
	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return localID, responseBody, nil
}

// GetModelList 获取支持的模型列表
func (a *TaskAdaptor) GetModelList() []string {
	return []string{
		"veo-3.0-generate-001",
		"veo-3.0-fast-generate-001",
		"veo-3.1-generate-preview",
		"veo-3.1-fast-generate-preview",
	}
}

// GetChannelName 获取渠道名称
func (a *TaskAdaptor) GetChannelName() string { return "vertex" }

// buildFetchOperationURL 构建查询操作的 URL
func buildFetchOperationURL(baseURL, upstreamName string) (string, error) {
	region := extractRegionFromOperationName(upstreamName)
	if region == "" {
		region = "us-central1"
	}
	project := extractProjectFromOperationName(upstreamName)
	modelName := extractModelFromOperationName(upstreamName)
	if strings.TrimSpace(modelName) == "" {
		return "", fmt.Errorf("cannot extract model from operation name")
	}
	if strings.TrimSpace(project) == "" {
		return "", fmt.Errorf("cannot extract project from operation name")
	}
	return vertexcore.BuildGoogleModelURL(baseURL, vertexcore.DefaultAPIVersion, project, region, modelName, "fetchPredictOperation"), nil
}

// FetchTask 查询任务状态
// 向 Vertex AI 发送查询请求，获取视频生成操作的状态
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}
	upstreamName, err := taskcommon.DecodeLocalTaskID(taskID)
	if err != nil {
		return nil, fmt.Errorf("decode task_id failed: %w", err)
	}
	url, err := buildFetchOperationURL(baseUrl, upstreamName)
	if err != nil {
		return nil, err
	}
	payload := fetchOperationPayload{OperationName: upstreamName}
	data, err := common.Marshal(payload)
	if err != nil {
		return nil, err
	}
	adc := &vertexcore.Credentials{}
	if err := common.Unmarshal([]byte(key), adc); err != nil {
		return nil, fmt.Errorf("failed to decode credentials: %w", err)
	}
	token, err := vertexcore.AcquireAccessToken(*adc, proxy)
	if err != nil {
		return nil, fmt.Errorf("failed to acquire access token: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-goog-user-project", adc.ProjectID)
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// ParseTaskResult 解析任务查询结果
// 将 Vertex AI 的操作响应转换为通用任务状态格式
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	var op operationResponse
	if err := common.Unmarshal(respBody, &op); err != nil {
		return nil, fmt.Errorf("unmarshal operation response failed: %w", err)
	}
	ti := &relaycommon.TaskInfo{}
	// 检查错误
	if op.Error.Message != "" {
		ti.Status = model.TaskStatusFailure
		ti.Reason = op.Error.Message
		ti.Progress = "100%"
		return ti, nil
	}
	// 检查是否完成
	if !op.Done {
		ti.Status = model.TaskStatusInProgress
		ti.Progress = "50%"
		return ti, nil
	}
	ti.Status = model.TaskStatusSuccess
	ti.Progress = "100%"
	// 从响应中提取视频数据
	if len(op.Response.Videos) > 0 {
		v0 := op.Response.Videos[0]
		if v0.BytesBase64Encoded != "" {
			mime := strings.TrimSpace(v0.MimeType)
			if mime == "" {
				enc := strings.TrimSpace(v0.Encoding)
				if enc == "" {
					enc = "mp4"
				}
				if strings.Contains(enc, "/") {
					mime = enc
				} else {
					mime = "video/" + enc
				}
			}
			ti.Url = "data:" + mime + ";base64," + v0.BytesBase64Encoded
			return ti, nil
		}
	}
	if op.Response.BytesBase64Encoded != "" {
		enc := strings.TrimSpace(op.Response.Encoding)
		if enc == "" {
			enc = "mp4"
		}
		mime := enc
		if !strings.Contains(enc, "/") {
			mime = "video/" + enc
		}
		ti.Url = "data:" + mime + ";base64," + op.Response.BytesBase64Encoded
		return ti, nil
	}
	if op.Response.Video != "" { // 某些变体使用 `video` 字段存储 base64 数据
		enc := strings.TrimSpace(op.Response.Encoding)
		if enc == "" {
			enc = "mp4"
		}
		mime := enc
		if !strings.Contains(enc, "/") {
			mime = "video/" + enc
		}
		ti.Url = "data:" + mime + ";base64," + op.Response.Video
		return ti, nil
	}
	return ti, nil
}

// ConvertToOpenAIVideo 将任务数据转换为 OpenAI 视频格式
func (a *TaskAdaptor) ConvertToOpenAIVideo(task *model.Task) ([]byte, error) {
	// 使用 GetUpstreamTaskID() 获取真实的上游操作名称用于模型提取
	// task.TaskID 现在是公共任务 ID（task_xxxx），不再是 base64 编码的上游名称
	upstreamTaskID := task.GetUpstreamTaskID()
	upstreamName, err := taskcommon.DecodeLocalTaskID(upstreamTaskID)
	if err != nil {
		upstreamName = ""
	}
	modelName := extractModelFromOperationName(upstreamName)
	if strings.TrimSpace(modelName) == "" {
		modelName = "veo-3.0-generate-001"
	}
	v := dto.NewOpenAIVideo()
	v.ID = task.TaskID
	v.Model = modelName
	v.Status = task.Status.ToVideoStatus()
	v.SetProgressStr(task.Progress)
	v.CreatedAt = task.CreatedAt
	v.CompletedAt = task.UpdatedAt
	if resultURL := task.GetResultURL(); strings.HasPrefix(resultURL, "data:") && len(resultURL) > 0 {
		v.SetMetadata("url", resultURL)
	}

	return common.Marshal(v)
}

// ============================
// helpers
// ============================

var regionRe = regexp.MustCompile(`locations/([a-z0-9-]+)/`)

func extractRegionFromOperationName(name string) string {
	m := regionRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

var modelRe = regexp.MustCompile(`models/([^/]+)/operations/`)

func extractModelFromOperationName(name string) string {
	m := modelRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	idx := strings.Index(name, "models/")
	if idx >= 0 {
		s := name[idx+len("models/"):]
		if p := strings.Index(s, "/operations/"); p > 0 {
			return s[:p]
		}
	}
	return ""
}

var projectRe = regexp.MustCompile(`projects/([^/]+)/locations/`)

func extractProjectFromOperationName(name string) string {
	m := projectRe.FindStringSubmatch(name)
	if len(m) == 2 {
		return m[1]
	}
	return ""
}
