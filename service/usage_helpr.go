// 本文件 (usage_helpr.go) 提供 Token 使用量估算的辅助函数。
// 当上游 API 未返回 Token 使用量信息时，通过本地方式估算 Token 数量。
// 主要用于不返回 usage 信息的模型或异常情况下的兜底处理。
package service

import (
	"github.com/c1cada/NexusTok/common"     // 项目公共工具包
	"github.com/c1cada/NexusTok/constant"   // 常量定义（上下文键等）
	"github.com/c1cada/NexusTok/dto"        // 数据传输对象（Usage 结构体）
	"github.com/gin-gonic/gin"              // Gin Web 框架
)

// 注释掉的 GetPromptTokens 函数：根据不同的 Relay 模式计算输入 Token 数。
// 该函数已被其他方式替代，保留代码供参考。

// ResponseText2Usage 从响应文本本地估算 Token 使用量。
// 当上游 API 未返回 usage 信息时，使用此函数作为兜底方案。
// 会在上下文中设置 local_count_tokens 标记，表示使用了本地 Token 计数。
// 参数:
//   - c: Gin 上下文
//   - responseText: 上游 API 返回的响应文本
//   - modeName: 模型名称（用于选择合适的 Token 编码器）
//   - promptTokens: 已知的输入 Token 数量
// 返回值:
//   - *dto.Usage: 估算的 Token 使用量（包含 prompt_tokens、completion_tokens、total_tokens）
func ResponseText2Usage(c *gin.Context, responseText string, modeName string, promptTokens int) *dto.Usage {
	// 在上下文中标记使用了本地 Token 计数
	common.SetContextKey(c, constant.ContextKeyLocalCountTokens, true)
	usage := &dto.Usage{}
	usage.PromptTokens = promptTokens
	// 使用本地编码器估算响应文本的 Token 数量
	usage.CompletionTokens = EstimateTokenByModel(modeName, responseText)
	usage.TotalTokens = usage.PromptTokens + usage.CompletionTokens
	return usage
}

// ValidUsage 检查 usage 是否包含有效的 Token 使用量数据。
// 当输入或输出 Token 数至少有一个不为 0 时，认为 usage 有效。
// 参数:
//   - usage: Token 使用量数据，可为 nil
// 返回值:
//   - bool: true 表示 usage 有效（非 nil 且包含非零的 Token 数据）
func ValidUsage(usage *dto.Usage) bool {
	return usage != nil && (usage.PromptTokens != 0 || usage.CompletionTokens != 0)
}
