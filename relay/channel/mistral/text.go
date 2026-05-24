// Mistral 请求格式转换文件。
// 负责将 OpenAI 格式的对话请求转换为 Mistral 兼容格式。
// 主要处理以下差异：
// - tool_call_id 格式: Mistral 要求 9 位字母数字 ID，OpenAI 可能使用其他格式
// - 图片 URL 格式: 统一图片 URL 的表示方式
// - 消息结构: 精简消息字段，去除 Mistral 不支持的内容
package mistral

// 标准库导入
import (
	"regexp" // 正则表达式（用于验证 tool_call_id 格式）

	// 项目内部依赖
	"github.com/c1cada/NexusTok/common" // 公共工具函数（随机字符串生成等）
	"github.com/c1cada/NexusTok/dto"    // 数据传输对象定义
)

// mistralToolCallIdRegexp Mistral tool_call_id 的格式验证正则。
// Mistral 要求 tool_call_id 为恰好 9 位的字母数字字符。
var mistralToolCallIdRegexp = regexp.MustCompile("^[a-zA-Z0-9]{9}$")

// requestOpenAI2Mistral 将 OpenAI 格式的对话请求转换为 Mistral 格式。
// 转换逻辑：
// 1. tool_call_id 格式化: 将非 9 位的 tool_call_id 替换为随机 9 位 ID，并维护映射关系确保一致性
// 2. tool_calls.id 格式化: 同样将 assistant 消息中的 tool_calls ID 进行格式化
// 3. 图片 URL 格式化: 将嵌套的 image_url 对象展平为直接 URL 字符串
// 4. 消息精简: 只保留 role、content、tool_calls、tool_call_id 字段
// 5. max_tokens 处理: 统一 max_tokens 和 max_completion_tokens 为 max_tokens
//
// 参数:
//   - request: OpenAI 格式的通用请求对象
// 返回:
//   - *dto.GeneralOpenAIRequest: Mistral 格式的请求对象
func requestOpenAI2Mistral(request *dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	messages := make([]dto.Message, 0, len(request.Messages))
	idMap := make(map[string]string)
	for _, message := range request.Messages {
		// 1. tool_calls.id
		toolCalls := message.ParseToolCalls()
		if toolCalls != nil {
			for i := range toolCalls {
				if !mistralToolCallIdRegexp.MatchString(toolCalls[i].ID) {
					if newId, ok := idMap[toolCalls[i].ID]; ok {
						toolCalls[i].ID = newId
					} else {
						newId, err := common.GenerateRandomCharsKey(9)
						if err == nil {
							idMap[toolCalls[i].ID] = newId
							toolCalls[i].ID = newId
						}
					}
				}
			}
			message.SetToolCalls(toolCalls)
		}

		// 2. tool_call_id
		if message.ToolCallId != "" {
			if newId, ok := idMap[message.ToolCallId]; ok {
				message.ToolCallId = newId
			} else {
				if !mistralToolCallIdRegexp.MatchString(message.ToolCallId) {
					newId, err := common.GenerateRandomCharsKey(9)
					if err == nil {
						idMap[message.ToolCallId] = newId
						message.ToolCallId = newId
					}
				}
			}
		}

		mediaMessages := message.ParseContent()
		if message.Role == "assistant" && message.ToolCalls != nil && message.Content == "" {
			mediaMessages = []dto.MediaContent{}
		}
		for j, mediaMessage := range mediaMessages {
			if mediaMessage.Type == dto.ContentTypeImageURL {
				imageUrl := mediaMessage.GetImageMedia()
				mediaMessage.ImageUrl = imageUrl.Url
				mediaMessages[j] = mediaMessage
			}
		}
		message.SetMediaContent(mediaMessages)
		messages = append(messages, dto.Message{
			Role:       message.Role,
			Content:    message.Content,
			ToolCalls:  message.ToolCalls,
			ToolCallId: message.ToolCallId,
		})
	}
	out := &dto.GeneralOpenAIRequest{
		Model:       request.Model,
		Stream:      request.Stream,
		Messages:    messages,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Tools:       request.Tools,
		ToolChoice:  request.ToolChoice,
	}
	if request.MaxTokens != nil || request.MaxCompletionTokens != nil {
		maxTokens := request.GetMaxTokens()
		out.MaxTokens = &maxTokens
	}
	return out
}
