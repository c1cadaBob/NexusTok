// Package channel - adapter.go
// 该文件定义了渠道适配器接口（Adaptor）和任务适配器接口（TaskAdaptor）
//
// 适配器模式概述：
// 每个上游 AI 服务提供商都有自己的 API 格式，适配器负责：
// 1. 将统一的请求格式转换为上游特定的格式
// 2. 将上游特定的响应格式转换为统一的格式
// 3. 处理上游特定的认证方式和请求头
//
// 接口层次：
// - Adaptor: 标准 API 适配器（Chat、Completion、Embedding、Image、Audio 等）
// - TaskAdaptor: 任务型 API 适配器（Midjourney、Suno、视频生成等）
// - OpenAIVideoConverter: OpenAI 视频格式转换器
package channel

import (
	"io"
	"net/http"

	"github.com/c1cada/NexusTok/dto"                          // 数据传输对象
	"github.com/c1cada/NexusTok/model"                        // 数据模型
	relaycommon "github.com/c1cada/NexusTok/relay/common"     // 中继公共
	"github.com/c1cada/NexusTok/types"                        // 类型定义

	"github.com/gin-gonic/gin" // Gin 框架
)

// Adaptor 标准 API 适配器接口
// 所有上游 AI 服务提供商的适配器都必须实现此接口
//
// 生命周期：
// 1. Init: 初始化适配器
// 2. GetRequestURL: 构建请求 URL
// 3. SetupRequestHeader: 设置请求头
// 4. ConvertXxxRequest: 将统一请求转换为上游格式
// 5. DoRequest: 发送请求到上游
// 6. DoResponse: 处理上游响应
type Adaptor interface {
	// Init 初始化适配器
	// 参数 info 包含中继信息（渠道类型、Key、URL 等）
	Init(info *relaycommon.RelayInfo)

	// GetRequestURL 构建上游 API 请求 URL
	// 返回完整的请求 URL（包括路径和查询参数）
	GetRequestURL(info *relaycommon.RelayInfo) (string, error)

	// SetupRequestHeader 设置请求头
	// 包括认证头、Content-Type、自定义头等
	SetupRequestHeader(c *gin.Context, req *http.Header, info *relaycommon.RelayInfo) error

	// ConvertOpenAIRequest 将 OpenAI 格式请求转换为上游格式
	// 用于 Chat Completion、Completion 等接口
	ConvertOpenAIRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeneralOpenAIRequest) (any, error)

	// ConvertRerankRequest 将 Rerank 格式请求转换为上游格式
	ConvertRerankRequest(c *gin.Context, relayMode int, request dto.RerankRequest) (any, error)

	// ConvertEmbeddingRequest 将 Embedding 格式请求转换为上游格式
	ConvertEmbeddingRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.EmbeddingRequest) (any, error)

	// ConvertAudioRequest 将音频格式请求转换为上游格式
	// 用于 TTS、STT 等接口
	ConvertAudioRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.AudioRequest) (io.Reader, error)

	// ConvertImageRequest 将图像格式请求转换为上游格式
	// 用于 DALL-E、Midjourney 等图像生成接口
	ConvertImageRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.ImageRequest) (any, error)

	// ConvertOpenAIResponsesRequest 将 OpenAI Responses 格式请求转换为上游格式
	ConvertOpenAIResponsesRequest(c *gin.Context, info *relaycommon.RelayInfo, request dto.OpenAIResponsesRequest) (any, error)

	// DoRequest 发送请求到上游 API
	// requestBody 是转换后的请求体
	DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (any, error)

	// DoResponse 处理上游 API 响应
	// 返回使用量信息和可能的错误
	DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (usage any, err *types.NexusTokError)

	// GetModelList 获取适配器支持的模型列表
	GetModelList() []string

	// GetChannelName 获取渠道名称
	GetChannelName() string

	// ConvertClaudeRequest 将 Claude 格式请求转换为上游格式
	ConvertClaudeRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.ClaudeRequest) (any, error)

	// ConvertGeminiRequest 将 Gemini 格式请求转换为上游格式
	ConvertGeminiRequest(c *gin.Context, info *relaycommon.RelayInfo, request *dto.GeminiChatRequest) (any, error)
}

// TaskAdaptor 任务型 API 适配器接口
// 用于异步任务型 API（如 Midjourney、Suno、视频生成等）
//
// 生命周期：
// 1. Init: 初始化适配器
// 2. ValidateRequestAndSetAction: 验证请求并设置任务动作
// 3. EstimateBilling: 估算计费（预扣费）
// 4. BuildRequestBody: 构建请求体
// 5. DoRequest: 提交任务到上游
// 6. DoResponse: 处理提交响应，获取任务 ID
// 7. FetchTask: 轮询任务状态
// 8. ParseTaskResult: 解析任务结果
// 9. AdjustBillingOnComplete: 最终结算
type TaskAdaptor interface {
	// Init 初始化适配器
	Init(info *relaycommon.RelayInfo)

	// ValidateRequestAndSetAction 验证请求参数并设置任务动作
	// 返回 TaskError 表示验证失败
	ValidateRequestAndSetAction(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError

	// ── 计费相关 ──────────────────────────────────────────────────────

	// EstimateBilling 返回预扣费的 OtherRatios
	// 在 ValidateRequestAndSetAction 之后、价格计算之前调用
	// 适配器应从请求中提取时长、分辨率等参数，作为倍率乘数返回
	// 返回 nil 表示使用基础模型价格，无额外倍率
	EstimateBilling(c *gin.Context, info *relaycommon.RelayInfo) map[string]float64

	// AdjustBillingOnSubmit 返回上游提交响应后的调整 OtherRatios
	// 在成功的 DoResponse 之后调用
	// 如果上游返回的实际参数与估算不同（如实际时长），返回更新后的倍率
	// 返回 nil 表示无需调整
	AdjustBillingOnSubmit(info *relaycommon.RelayInfo, taskData []byte) map[string]float64

	// AdjustBillingOnComplete 返回任务完成时的实际配额
	// 在轮询循环中，ParseTaskResult 之后调用
	// 返回正值触发差额结算（补扣/退还）
	// 返回 0 表示保持预扣金额不变
	AdjustBillingOnComplete(task *model.Task, taskResult *relaycommon.TaskInfo) int

	// ── 请求/响应相关 ───────────────────────────────────────────────────

	// BuildRequestURL 构建上游 API 请求 URL
	BuildRequestURL(info *relaycommon.RelayInfo) (string, error)

	// BuildRequestHeader 构建请求头
	BuildRequestHeader(c *gin.Context, req *http.Request, info *relaycommon.RelayInfo) error

	// BuildRequestBody 构建请求体
	BuildRequestBody(c *gin.Context, info *relaycommon.RelayInfo) (io.Reader, error)

	// DoRequest 发送请求到上游 API
	DoRequest(c *gin.Context, info *relaycommon.RelayInfo, requestBody io.Reader) (*http.Response, error)

	// DoResponse 处理上游 API 响应
	// 返回任务 ID、任务数据和可能的错误
	DoResponse(c *gin.Context, resp *http.Response, info *relaycommon.RelayInfo) (taskID string, taskData []byte, err *dto.TaskError)

	// GetModelList 获取适配器支持的模型列表
	GetModelList() []string

	// GetChannelName 获取渠道名称
	GetChannelName() string

	// ── 轮询相关 ──────────────────────────────────────────────────────

	// FetchTask 获取任务状态
	// 通过 HTTP 请求上游 API 查询任务状态
	FetchTask(baseUrl, key string, body map[string]any, proxy string) (*http.Response, error)

	// ParseTaskResult 解析任务结果
	// 从上游响应中提取任务状态、进度、结果等信息
	ParseTaskResult(respBody []byte) (*relaycommon.TaskInfo, error)
}

// OpenAIVideoConverter OpenAI 视频格式转换器接口
// 将内部任务格式转换为 OpenAI 视频 API 响应格式
type OpenAIVideoConverter interface {
	// ConvertToOpenAIVideo 将内部任务转换为 OpenAI 视频格式
	ConvertToOpenAIVideo(originTask *model.Task) ([]byte, error)
}
