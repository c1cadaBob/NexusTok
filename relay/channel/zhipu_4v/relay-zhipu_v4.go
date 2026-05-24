// 智谱 GLM-4V 的请求格式转换逻辑。
// 负责将 OpenAI 格式的请求转换为智谱 GLM-4V 兼容的格式，
// 主要处理多模态消息中的图片 URL 格式转换（如 base64 前缀去除），
// 以及 stop 参数的类型标准化。
package zhipu_4v

import (
	"strings"

	"github.com/c1cada/NexusTok/dto"
)

// requestOpenAI2Zhipu 将 OpenAI 格式的请求转换为智谱 GLM-4V 兼容格式。
// 特殊处理：
//   - 对于多模态消息中的图片 URL，如果是 base64 编码格式（"data:image/..."），
//     去除 URL 中的数据前缀，只保留纯 base64 数据
//   - stop 参数统一转换为字符串切片格式（智谱要求 []string 类型）
//   - MaxTokens/MaxCompletionTokens 统一取较大值
// 参数:
//   - request: OpenAI 格式的通用请求
// 返回:
//   - *dto.GeneralOpenAIRequest: 转换后的请求体
func requestOpenAI2Zhipu(request dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	messages := make([]dto.Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		if !message.IsStringContent() {
			mediaMessages := message.ParseContent()
			for j, mediaMessage := range mediaMessages {
				if mediaMessage.Type == dto.ContentTypeImageURL {
					imageUrl := mediaMessage.GetImageMedia()
					// check if base64
					if strings.HasPrefix(imageUrl.Url, "data:image/") {
						// 去除base64数据的URL前缀（如果有）
						if idx := strings.Index(imageUrl.Url, ","); idx != -1 {
							imageUrl.Url = imageUrl.Url[idx+1:]
						}
					}
					mediaMessage.ImageUrl = imageUrl
					mediaMessages[j] = mediaMessage
				}
			}
			message.SetMediaContent(mediaMessages)
		}
		messages = append(messages, dto.Message{
			Role:       message.Role,
			Content:    message.Content,
			ToolCalls:  message.ToolCalls,
			ToolCallId: message.ToolCallId,
		})
	}
	str, ok := request.Stop.(string)
	var Stop []string
	if ok {
		Stop = []string{str}
	} else {
		Stop, _ = request.Stop.([]string)
	}
	out := &dto.GeneralOpenAIRequest{
		Model:       request.Model,
		Stream:      request.Stream,
		Messages:    messages,
		Temperature: request.Temperature,
		TopP:        request.TopP,
		Stop:        Stop,
		Tools:       request.Tools,
		ToolChoice:  request.ToolChoice,
		THINKING:    request.THINKING,
	}
	if request.MaxTokens != nil || request.MaxCompletionTokens != nil {
		maxTokens := request.GetMaxTokens()
		out.MaxTokens = &maxTokens
	}
	return out
}
