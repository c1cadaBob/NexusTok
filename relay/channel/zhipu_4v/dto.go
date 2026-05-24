// 智谱 GLM-4V API 的数据传输对象（DTO）定义。
// 定义了 GLM-4V 的流式和非流式响应结构体，以及令牌缓存结构。
// GLM-4V 的响应格式与 OpenAI 兼容，因此直接复用 OpenAI 的响应结构体。
package zhipu_4v

import (
	"time"

	"github.com/c1cada/NexusTok/dto"   // 数据传输对象
	"github.com/c1cada/NexusTok/types"  // 错误类型
)

// ZhipuV4Response 智谱 GLM-4V 非流式响应结构体。
// 响应格式与 OpenAI 兼容，直接使用 OpenAI 的响应结构体。
type ZhipuV4Response struct {
	Id                  string                         `json:"id"`       // 响应唯一标识
	Created             int64                          `json:"created"`  // 创建时间戳
	Model               string                         `json:"model"`    // 模型名称
	TextResponseChoices []dto.OpenAITextResponseChoice `json:"choices"`  // 文本响应选项列表
	Usage               dto.Usage                      `json:"usage"`    // token 使用量
	Error               types.OpenAIError              `json:"error"`    // 错误信息
}

// ZhipuV4StreamResponse 智谱 GLM-4V 流式响应结构体。
// 流式响应格式与 OpenAI 兼容，直接使用 OpenAI 的流式响应结构体。
type ZhipuV4StreamResponse struct {
	Id      string                                    `json:"id"`       // 响应唯一标识
	Created int64                                     `json:"created"`  // 创建时间戳
	Choices []dto.ChatCompletionsStreamResponseChoice `json:"choices"`  // 流式响应选项列表
	Usage   dto.Usage                                 `json:"usage"`    // token 使用量
}

// tokenData JWT 令牌缓存数据结构。
// 用于缓存已生成的 JWT 令牌，避免重复生成。
type tokenData struct {
	Token      string    // JWT 令牌字符串
	ExpiryTime time.Time // 令牌过期时间
}
