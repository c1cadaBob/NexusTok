// Package perplexity 实现 Perplexity AI 渠道的中继逻辑
package perplexity

import "github.com/c1cada/NexusTok/dto" // 数据传输对象

// requestOpenAI2Perplexity 将 OpenAI 格式请求转换为 Perplexity 格式
// Perplexity API 兼容 OpenAI 格式，但支持额外的搜索参数
//
// 参数：
//   - request: OpenAI 格式的通用请求
//
// 返回值：
//   - *dto.GeneralOpenAIRequest: Perplexity 格式的请求
func requestOpenAI2Perplexity(request dto.GeneralOpenAIRequest) *dto.GeneralOpenAIRequest {
	// 简化消息结构，只保留 role 和 content
	messages := make([]dto.Message, 0, len(request.Messages))
	for _, message := range request.Messages {
		messages = append(messages, dto.Message{
			Role:    message.Role,
			Content: message.Content,
		})
	}
	// 构建 Perplexity 请求，包含搜索相关参数
	req := &dto.GeneralOpenAIRequest{
		Model:                  request.Model,
		Stream:                 request.Stream,
		Messages:               messages,
		Temperature:            request.Temperature,
		TopP:                   request.TopP,
		FrequencyPenalty:       request.FrequencyPenalty,
		PresencePenalty:        request.PresencePenalty,
		SearchDomainFilter:     request.SearchDomainFilter,     // 搜索域名过滤
		SearchRecencyFilter:    request.SearchRecencyFilter,    // 搜索时间过滤
		ReturnImages:           request.ReturnImages,           // 是否返回图片
		ReturnRelatedQuestions: request.ReturnRelatedQuestions, // 是否返回相关问题
		SearchMode:             request.SearchMode,             // 搜索模式
	}
	// 处理 max_tokens 参数
	if request.MaxTokens != nil || request.MaxCompletionTokens != nil {
		maxTokens := request.GetMaxTokens()
		req.MaxTokens = &maxTokens
	}
	return req
}
