// jimeng - adaptor.go
// 即梦（Jimeng / 火山引擎）视频生成任务适配器。
// 实现了 taskcommon.TaskAdaptor 接口，负责将统一的视频生成请求转换为即梦 API 格式。
// 支持文生视频、图生视频和首尾帧生视频模式。
// 使用火山引擎的 HMAC-SHA256 签名认证机制，也支持通过 NexusTok 中继的简化认证。
// 即梦视频 3.0 版本支持根据输入图片数量自动选择生成模式（文生/图生/首尾帧）。
package jimeng

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/model"
	"github.com/samber/lo"

	"github.com/gin-gonic/gin"
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

// requestPayload 即梦视频生成 API 的请求体结构体。
// 包含模型标识（req_key）、图片数据、提示词、帧数等参数。
type requestPayload struct {
	ReqKey           string   `json:"req_key"`                        // 模型/任务类型标识（如 "jimeng_vgfm_t2v_l20"、"jimeng_t2v_v30"）
	BinaryDataBase64 []string `json:"binary_data_base64,omitempty"`   // Base64 编码的图片数据列表（二进制图片输入方式）
	ImageUrls        []string `json:"image_urls,omitempty"`           // 图片URL列表（远程图片输入方式）
	Prompt           string   `json:"prompt,omitempty"`               // 文本提示词
	Seed             int64    `json:"seed"`                           // 随机数种子
	AspectRatio      string   `json:"aspect_ratio"`                   // 视频宽高比
	Frames           int      `json:"frames,omitempty"`               // 视频帧数（决定时长：121帧≈5秒，241帧≈10秒）
}

// responsePayload 即梦任务提交响应结构体。
// 包含响应状态码、消息和新创建的任务ID。
type responsePayload struct {
	Code      int    `json:"code"`       // 响应状态码（10000 表示成功）
	Message   string `json:"message"`    // 响应消息
	RequestId string `json:"request_id"` // 请求唯一标识
	Data      struct {
		TaskID string `json:"task_id"` // 新创建的任务唯一标识
	} `json:"data"`                      // 响应数据
}

// responseTask 即梦任务详情响应结构体。
// 包含任务的完整状态信息、生成结果和耗时统计。
type responseTask struct {
	Code int `json:"code"` // 响应状态码（10000 表示成功）
	Data struct {
		BinaryDataBase64 []interface{} `json:"binary_data_base64"` // Base64 编码的结果数据（图片等）
		ImageUrls        interface{}   `json:"image_urls"`         // 结果图片URL
		RespData         string        `json:"resp_data"`          // 响应数据（JSON 字符串）
		Status           string        `json:"status"`             // 任务状态: in_queue/done
		VideoUrl         string        `json:"video_url"`          // 生成的视频下载URL（仅在任务完成时有值）
	} `json:"data"`                                            // 任务数据
	Message     string `json:"message"`      // 响应消息
	RequestId   string `json:"request_id"`   // 请求唯一标识
	Status      int    `json:"status"`       // 状态码
	TimeElapsed string `json:"time_elapsed"` // 任务处理耗时
}

const (
	// MaxFileSize 即梦 API 限制的单个文件最大大小：4.7MB。
	// 参考文档: https://www.volcengine.com/docs/85621/1747301
	MaxFileSize int64 = 4*1024*1024 + 700*1024 // 4.7MB (4MB + 724KB)
)

// ============================
// 适配器实现
// ============================

// TaskAdaptor 即梦（火山引擎）视频生成任务适配器。
// 实现 taskcommon.TaskAdaptor 接口，负责与即梦 API 交互。
// 继承 BaseBilling 以获得基础计费功能。
type TaskAdaptor struct {
	taskcommon.BaseBilling           // 嵌入基础计费功能
	ChannelType int                  // 渠道类型常量
	accessKey   string               // 火山引擎访问密钥 ID（AK）
	secretKey   string               // 火山引擎访问密钥秘密（SK）
	baseURL     string               // 即梦 API 基础URL
}

// Init 初始化适配器，从中继信息中提取渠道类型、API 基础URL 和认证信息。
// API 密钥格式为 "access_key|secret_key"，使用 "|" 分隔。
// 如果密钥以 "sk-" 开头，表示通过 NexusTok 中继访问，使用简化的 Bearer Token 认证。
//
// 参数：
//   - info: 中继上下文信息，包含渠道配置和认证信息
func (a *TaskAdaptor) Init(info *relaycommon.RelayInfo) {
	a.ChannelType = info.ChannelType
	a.baseURL = info.ChannelBaseUrl

	// apiKey format: "access_key|secret_key"
	keyParts := strings.Split(info.ApiKey, "|")
	if len(keyParts) == 2 {
		a.accessKey = strings.TrimSpace(keyParts[0])
		a.secretKey = strings.TrimSpace(keyParts[1])
	}
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
	return relaycommon.ValidateBasicTaskRequest(c, info, constant.TaskActionGenerate)
}

// BuildRequestURL 构建即梦视频生成任务提交 API 的请求URL。
// 如果通过 NexusTok 中继访问，URL 中会包含 "/jimeng/" 路径前缀。
// 端点: {baseURL}/[jimeng/]？Action=CVSync2AsyncSubmitTask&Version=2022-08-31
//
// 参数：
//   - info: 中继上下文信息
//
// 返回：完整的API请求URL和可能的错误
func (a *TaskAdaptor) BuildRequestURL(info *relaycommon.RelayInfo) (string, error) {
	if isNexusTokRelay(info.ApiKey) {
		return fmt.Sprintf("%s/jimeng/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", a.baseURL), nil
	}
	return fmt.Sprintf("%s/?Action=CVSync2AsyncSubmitTask&Version=2022-08-31", a.baseURL), nil
}

// BuildRequestHeader 设置即梦 API 所需的请求头。
// 如果通过 NexusTok 中继访问，使用简化的 Bearer Token 认证；
// 否则使用火山引擎的 HMAC-SHA256 签名认证。
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
	if isNexusTokRelay(info.ApiKey) {
		req.Header.Set("Authorization", "Bearer "+info.ApiKey)
	} else {
		return a.signRequest(req, a.accessKey, a.secretKey)
	}
	return nil
}

// BuildRequestBody 构建发送给即梦 API 的请求体。
// 支持 OpenAI SDK 的 multipart 文件上传方式，将上传的图片文件转换为 Base64 格式。
// 根据上传的图片数量自动设置动作类型：单张图片为图生视频，多张图片为首尾帧生视频。
// 文件大小限制为 4.7MB。
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
	req, ok := v.(relaycommon.TaskSubmitReq)
	if !ok {
		return nil, fmt.Errorf("invalid request type in context")
	}
	// 支持openai sdk的图片上传方式
	if mf, err := c.MultipartForm(); err == nil {
		if files, exists := mf.File["input_reference"]; exists && len(files) > 0 {
			if len(files) == 1 {
				info.Action = constant.TaskActionGenerate
			} else if len(files) > 1 {
				info.Action = constant.TaskActionFirstTailGenerate
			}

			// 将上传的文件转换为base64格式
			var images []string

			for _, fileHeader := range files {
				// 检查文件大小
				if fileHeader.Size > MaxFileSize {
					return nil, fmt.Errorf("文件 %s 大小超过限制，最大允许 %d MB", fileHeader.Filename, MaxFileSize/(1024*1024))
				}

				file, err := fileHeader.Open()
				if err != nil {
					continue
				}
				fileBytes, err := io.ReadAll(file)
				file.Close()
				if err != nil {
					continue
				}
				// 将文件内容转换为base64
				base64Str := base64.StdEncoding.EncodeToString(fileBytes)
				images = append(images, base64Str)
			}
			req.Images = images
		}
	}

	body, err := a.convertToRequestPayload(&req, info)
	if err != nil {
		return nil, errors.Wrap(err, "convert request payload failed")
	}
	data, err := common.Marshal(body)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(data), nil
}

// DoRequest 发送 HTTP 请求到即梦 API。
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

// DoResponse 处理即梦 API 的响应。
// 解析响应体获取任务ID，检查响应状态码（10000 表示成功），
// 构建 OpenAI 兼容格式的初始响应返回给客户端。
//
// 参数：
//   - c: Gin 上下文
//   - resp: 即梦 API 的 HTTP 响应
//   - info: 中继上下文信息
//
// 返回：
//   - taskID: 即梦任务ID（用于后续轮询）
//   - taskData: 原始响应数据（用于存储）
//   - taskErr: 任务错误信息（nil 表示成功）
func (a *TaskAdaptor) DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, taskErr *dto.TaskError) {
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		taskErr = service.TaskErrorWrapper(err, "read_response_body_failed", http.StatusInternalServerError)
		return
	}
	_ = resp.Body.Close()

	// Parse Jimeng response
	var jResp responsePayload
	if err := common.Unmarshal(responseBody, &jResp); err != nil {
		taskErr = service.TaskErrorWrapper(errors.Wrapf(err, "body: %s", responseBody), "unmarshal_response_body_failed", http.StatusInternalServerError)
		return
	}

	if jResp.Code != 10000 {
		taskErr = service.TaskErrorWrapper(fmt.Errorf("%s", jResp.Message), fmt.Sprintf("%d", jResp.Code), http.StatusInternalServerError)
		return
	}

	ov := dto.NewOpenAIVideo()
	ov.ID = info.PublicTaskID
	ov.TaskID = info.PublicTaskID
	ov.CreatedAt = time.Now().Unix()
	ov.Model = info.OriginModelName
	c.JSON(http.StatusOK, ov)
	return jResp.Data.TaskID, responseBody, nil
}

// FetchTask 向即梦查询异步任务的当前状态。
// 通过 POST CVSync2AsyncGetResult 接口获取任务最新状态。
// 使用固定的 req_key 值 "jimeng_vgfm_t2v_l20"。
// 如果通过 NexusTok 中继访问，使用简化认证；否则使用 HMAC-SHA256 签名认证。
//
// 参数：
//   - baseUrl: 即梦 API 基础URL
//   - key: API 密钥（格式为 "access_key|secret_key" 或 NexusTok 中继密钥）
//   - body: 包含 task_id 的请求参数 map
//   - proxy: 代理地址（可为空）
//
// 返回：HTTP 响应和可能的错误
func (a *TaskAdaptor) FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error) {
	taskID, ok := body["task_id"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid task_id")
	}

	uri := fmt.Sprintf("%s/?Action=CVSync2AsyncGetResult&Version=2022-08-31", baseUrl)
	if isNexusTokRelay(key) {
		uri = fmt.Sprintf("%s/jimeng/?Action=CVSync2AsyncGetResult&Version=2022-08-31", a.baseURL)
	}
	payload := map[string]string{
		"req_key": "jimeng_vgfm_t2v_l20", // This is fixed value from doc: https://www.volcengine.com/docs/85621/1544774
		"task_id": taskID,
	}
	payloadBytes, err := common.Marshal(payload)
	if err != nil {
		return nil, errors.Wrap(err, "marshal fetch task payload failed")
	}

	req, err := http.NewRequest(http.MethodPost, uri, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	if isNexusTokRelay(key) {
		req.Header.Set("Authorization", "Bearer "+key)
	} else {
		keyParts := strings.Split(key, "|")
		if len(keyParts) != 2 {
			return nil, fmt.Errorf("invalid api key format for jimeng: expected 'ak|sk'")
		}
		accessKey := strings.TrimSpace(keyParts[0])
		secretKey := strings.TrimSpace(keyParts[1])

		if err := a.signRequest(req, accessKey, secretKey); err != nil {
			return nil, errors.Wrap(err, "sign request failed")
		}
	}
	client, err := service.GetHttpClientWithProxy(proxy)
	if err != nil {
		return nil, fmt.Errorf("new proxy http client failed: %w", err)
	}
	return client.Do(req)
}

// GetModelList 返回即梦支持的视频生成模型列表。
// 当前仅支持 "jimeng_vgfm_t2v_l20" 模型。
//
// 返回：支持的模型名称切片
func (a *TaskAdaptor) GetModelList() []string {
	return []string{"jimeng_vgfm_t2v_l20"}
}

// GetChannelName 返回即梦视频生成渠道的唯一标识名称。
//
// 返回：渠道名称字符串（"jimeng"）
func (a *TaskAdaptor) GetChannelName() string {
	return "jimeng"
}

// signRequest 使用火山引擎的 HMAC-SHA256 签名机制对 HTTP 请求进行签名。
// 签名流程：
//  1. 计算请求体的 SHA256 哈希
//  2. 构建规范化的请求字符串（包含方法、路径、查询参数、头部等）
//  3. 使用派生密钥链（secretKey -> date -> region -> service -> signing）计算签名
//  4. 将签名结果添加到 Authorization 头部
//
// 参数：
//   - req: 待签名的 HTTP 请求
//   - accessKey: 火山引擎访问密钥 ID
//   - secretKey: 火山引擎访问密钥秘密
//
// 返回：签名过程中的错误，nil 表示签名成功
func (a *TaskAdaptor) signRequest(req *http.Request, accessKey, secretKey string) error {
	var bodyBytes []byte
	var err error

	if req.Body != nil {
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return errors.Wrap(err, "read request body failed")
		}
		_ = req.Body.Close()
		req.Body = io.NopCloser(bytes.NewBuffer(bodyBytes)) // Rewind
	} else {
		bodyBytes = []byte{}
	}

	payloadHash := sha256.Sum256(bodyBytes)
	hexPayloadHash := hex.EncodeToString(payloadHash[:])

	t := time.Now().UTC()
	xDate := t.Format("20060102T150405Z")
	shortDate := t.Format("20060102")

	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Date", xDate)
	req.Header.Set("X-Content-Sha256", hexPayloadHash)

	// Sort and encode query parameters to create canonical query string
	queryParams := req.URL.Query()
	sortedKeys := make([]string, 0, len(queryParams))
	for k := range queryParams {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)
	var queryParts []string
	for _, k := range sortedKeys {
		values := queryParams[k]
		sort.Strings(values)
		for _, v := range values {
			queryParts = append(queryParts, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
		}
	}
	canonicalQueryString := strings.Join(queryParts, "&")

	headersToSign := map[string]string{
		"host":             req.URL.Host,
		"x-date":           xDate,
		"x-content-sha256": hexPayloadHash,
	}
	if req.Header.Get("Content-Type") != "" {
		headersToSign["content-type"] = req.Header.Get("Content-Type")
	}

	var signedHeaderKeys []string
	for k := range headersToSign {
		signedHeaderKeys = append(signedHeaderKeys, k)
	}
	sort.Strings(signedHeaderKeys)

	var canonicalHeaders strings.Builder
	for _, k := range signedHeaderKeys {
		canonicalHeaders.WriteString(k)
		canonicalHeaders.WriteString(":")
		canonicalHeaders.WriteString(strings.TrimSpace(headersToSign[k]))
		canonicalHeaders.WriteString("\n")
	}
	signedHeaders := strings.Join(signedHeaderKeys, ";")

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		req.URL.Path,
		canonicalQueryString,
		canonicalHeaders.String(),
		signedHeaders,
		hexPayloadHash,
	)

	hashedCanonicalRequest := sha256.Sum256([]byte(canonicalRequest))
	hexHashedCanonicalRequest := hex.EncodeToString(hashedCanonicalRequest[:])

	region := "cn-north-1"
	serviceName := "cv"
	credentialScope := fmt.Sprintf("%s/%s/%s/request", shortDate, region, serviceName)
	stringToSign := fmt.Sprintf("HMAC-SHA256\n%s\n%s\n%s",
		xDate,
		credentialScope,
		hexHashedCanonicalRequest,
	)

	kDate := hmacSHA256([]byte(secretKey), []byte(shortDate))
	kRegion := hmacSHA256(kDate, []byte(region))
	kService := hmacSHA256(kRegion, []byte(serviceName))
	kSigning := hmacSHA256(kService, []byte("request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	authorization := fmt.Sprintf("HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		accessKey,
		credentialScope,
		signedHeaders,
		signature,
	)
	req.Header.Set("Authorization", authorization)
	return nil
}

// hmacSHA256 使用 HMAC-SHA256 算法计算消息认证码。
// 用于火山引擎签名认证中的密钥派生链。
//
// 参数：
//   - key: HMAC 密钥
//   - data: 待计算的消息数据
//
// 返回：HMAC-SHA256 计算结果的字节切片
func hmacSHA256(key []byte, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

// convertToRequestPayload 将统一的 TaskSubmitReq 转换为即梦 API 的请求格式。
// 处理图片输入（URL 或 Base64）、帧数计算和 metadata 参数解析。
// 帧数与视频时长的关系：5秒=121帧（24*5+1），10秒=241帧（24*10+1）。
//
// 即梦视频 3.0 版本的 req_key 转换逻辑：
//   - jimeng_v30_pro -> jimeng_ti2v_v30_pro（固定）
//   - 多张图片 -> jimeng_i2v_first_tail_v30（首尾帧生视频）
//   - 单张图片 -> jimeng_i2v_first_v30（图生视频）
//   - 无图片 -> jimeng_t2v_v30（文生视频）
//
// 参数：
//   - req: 统一的任务提交请求
//   - info: 中继上下文信息
//
// 返回：即梦格式的请求和可能的错误
func (a *TaskAdaptor) convertToRequestPayload(req *relaycommon.TaskSubmitReq, info *relaycommon.RelayInfo) (*requestPayload, error) {
	r := requestPayload{
		ReqKey: info.UpstreamModelName,
		Prompt: req.Prompt,
	}

	switch req.Duration {
	case 10:
		r.Frames = 241 // 24*10+1 = 241
	default:
		r.Frames = 121 // 24*5+1 = 121
	}

	// Handle one-of image_urls or binary_data_base64
	if req.HasImage() {
		if strings.HasPrefix(req.Images[0], "http") {
			r.ImageUrls = req.Images
		} else {
			r.BinaryDataBase64 = req.Images
		}
	}
	if err := taskcommon.UnmarshalMetadata(req.Metadata, &r); err != nil {
		return nil, errors.Wrap(err, "unmarshal metadata failed")
	}

	// 即梦视频3.0 ReqKey转换
	// https://www.volcengine.com/docs/85621/1792707
	imageLen := lo.Max([]int{len(req.Images), len(r.BinaryDataBase64), len(r.ImageUrls)})
	if strings.Contains(r.ReqKey, "jimeng_v30") {
		if r.ReqKey == "jimeng_v30_pro" {
			// 3.0 pro只有固定的jimeng_ti2v_v30_pro
			r.ReqKey = "jimeng_ti2v_v30_pro"
		} else if imageLen > 1 {
			// 多张图片：首尾帧生成
			r.ReqKey = strings.TrimSuffix(strings.Replace(r.ReqKey, "jimeng_v30", "jimeng_i2v_first_tail_v30", 1), "p")
		} else if imageLen == 1 {
			// 单张图片：图生视频
			r.ReqKey = strings.TrimSuffix(strings.Replace(r.ReqKey, "jimeng_v30", "jimeng_i2v_first_v30", 1), "p")
		} else {
			// 无图片：文生视频
			r.ReqKey = strings.Replace(r.ReqKey, "jimeng_v30", "jimeng_t2v_v30", 1)
		}
	}

	return &r, nil
}

// ParseTaskResult 解析即梦任务轮询的响应数据。
// 将即梦的任务状态映射为内部统一状态：
//   - code=10000 且 status=in_queue -> TaskStatusQueued（排队中）
//   - code=10000 且 status=done -> TaskStatusSuccess（成功）
//   - 其他 code -> TaskStatusFailure（失败）
//
// 参数：
//   - respBody: 即梦任务查询接口的响应体字节数据
//
// 返回：统一的任务信息结构体和可能的解析错误
func (a *TaskAdaptor) ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error) {
	resTask := responseTask{}
	if err := common.Unmarshal(respBody, &resTask); err != nil {
		return nil, errors.Wrap(err, "unmarshal task result failed")
	}
	taskResult := relaycommon.TaskInfo{}
	if resTask.Code == 10000 {
		taskResult.Code = 0
	} else {
		taskResult.Code = resTask.Code // todo uni code
		taskResult.Reason = resTask.Message
		taskResult.Status = model.TaskStatusFailure
		taskResult.Progress = "100%"
	}
	switch resTask.Data.Status {
	case "in_queue":
		taskResult.Status = model.TaskStatusQueued
		taskResult.Progress = "10%"
	case "done":
		taskResult.Status = model.TaskStatusSuccess
		taskResult.Progress = "100%"
	}
	taskResult.Url = resTask.Data.VideoUrl
	return &taskResult, nil
}

// ConvertToOpenAIVideo 将存储的即梦任务数据转换为 OpenAI 兼容的视频响应格式。
// 从数据库中的原始任务数据反序列化即梦响应，构建 OpenAI 格式的视频对象，
// 包含任务ID、状态、进度、视频URL和错误信息。
//
// 参数：
//   - originTask: 数据库中的任务记录
//
// 返回：OpenAI 格式视频响应的 JSON 字节数据和可能的错误
func (a *TaskAdaptor) ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error) {
	var jimengResp responseTask
	if err := common.Unmarshal(originTask.Data, &jimengResp); err != nil {
		return nil, errors.Wrap(err, "unmarshal jimeng task data failed")
	}

	openAIVideo := dto.NewOpenAIVideo()
	openAIVideo.ID = originTask.TaskID
	openAIVideo.Status = originTask.Status.ToVideoStatus()
	openAIVideo.SetProgressStr(originTask.Progress)
	openAIVideo.SetMetadata("url", jimengResp.Data.VideoUrl)
	openAIVideo.CreatedAt = originTask.CreatedAt
	openAIVideo.CompletedAt = originTask.UpdatedAt

	if jimengResp.Code != 10000 {
		openAIVideo.Error = &dto.OpenAIVideoError{
			Message: jimengResp.Message,
			Code:    fmt.Sprintf("%d", jimengResp.Code),
		}
	}

	return common.Marshal(openAIVideo)
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
