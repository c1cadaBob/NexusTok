// 智谱 ChatGLM API 的数据传输对象（DTO）定义。
// 定义了智谱 API 的请求和响应结构体，包括消息格式、
// 请求参数、响应数据和 JWT 令牌缓存结构。
package zhipu

import (
	"time"

	"github.com/c1cada/NexusTok/dto"
)

// ZhipuMessage 智谱消息格式，包含角色和内容。
type ZhipuMessage struct {
	Role    string `json:"role"`    // 消息角色："user"、"assistant"、"system"
	Content string `json:"content"` // 消息文本内容
}

// ZhipuRequest 智谱聊天请求结构体。
type ZhipuRequest struct {
	Prompt      []ZhipuMessage `json:"prompt"`                 // 消息列表（智谱使用 prompt 而非 messages）
	Temperature *float64       `json:"temperature,omitempty"`  // 生成温度
	TopP        float64        `json:"top_p,omitempty"`        // Top-P 采样参数
	RequestId   string         `json:"request_id,omitempty"`   // 请求 ID，用于追踪
	Incremental bool           `json:"incremental,omitempty"`  // 是否增量返回（流式模式）
}

// ZhipuResponseData 智谱响应数据结构。
type ZhipuResponseData struct {
	TaskId     string         `json:"task_id"`     // 任务 ID
	RequestId  string         `json:"request_id"`  // 请求 ID
	TaskStatus string         `json:"task_status"` // 任务状态
	Choices    []ZhipuMessage `json:"choices"`     // 响应选项列表
	dto.Usage  `json:"usage"`                     // token 使用量（内嵌）
}

// ZhipuResponse 智谱非流式响应结构体。
type ZhipuResponse struct {
	Code    int               `json:"code"`    // 状态码
	Msg     string            `json:"msg"`     // 状态消息
	Success bool              `json:"success"` // 请求是否成功
	Data    ZhipuResponseData `json:"data"`    // 响应数据
}

// ZhipuStreamMetaResponse 智谱流式响应的元数据结构。
// 在流式传输结束时发送，包含 token 使用量等汇总信息。
type ZhipuStreamMetaResponse struct {
	RequestId  string `json:"request_id"`  // 请求 ID
	TaskId     string `json:"task_id"`     // 任务 ID
	TaskStatus string `json:"task_status"` // 任务状态
	dto.Usage  `json:"usage"`             // token 使用量（内嵌）
}

// zhipuTokenData JWT 令牌缓存数据结构。
// 用于缓存已生成的 JWT 令牌，避免重复生成。
type zhipuTokenData struct {
	Token      string    // JWT 令牌字符串
	ExpiryTime time.Time // 令牌过期时间
}
