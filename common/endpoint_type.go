// Package common - endpoint_type.go
// 该文件实现了渠道类型到端点类型的映射逻辑
//
// 端点类型（EndpointType）标识不同的 API 接口类型
// 每个渠道类型支持一组特定的端点类型
// 端点类型的优先级决定了请求的路由顺序
//
// 映射规则：
// - Jina 渠道 → JinaRerank 端点
// - Anthropic/AWS 渠道 → Anthropic + OpenAI 端点
// - Gemini/Vertex 渠道 → Gemini + OpenAI 端点
// - xAI 渠道 → OpenAI + OpenAIResponse 端点
// - Sora 渠道 → OpenAIVideo 端点
// - 其他渠道 → OpenAI 端点（默认）
package common

import "github.com/c1cada/NexusTok/constant"

// GetEndpointTypesByChannelType 获取渠道支持的端点类型列表
//
// 返回的端点类型按优先级排序，第一个是最优先的端点类型
// 所有渠道都支持 OpenAI 端点作为兜底
//
// 特殊处理：
// - 如果模型名称匹配 IsOpenAIResponseOnlyModel，则只返回 OpenAIResponse 端点
// - 如果模型名称匹配 IsImageGenerationModel，则在列表开头添加 ImageGeneration 端点
//
// 参数：
//   - channelType: 渠道类型常量
//   - modelName: 模型名称（用于特殊模型的端点判断）
//
// 返回值：
//   - []constant.EndpointType: 端点类型列表（按优先级排序）
func GetEndpointTypesByChannelType(channelType int, modelName string) []constant.EndpointType {
	var endpointTypes []constant.EndpointType
	switch channelType {
	case constant.ChannelTypeJina:
		// Jina 渠道只支持 Rerank 端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeJinaRerank}
	// 以下渠道类型已被注释掉（可能是暂时禁用或已移除）
	//case constant.ChannelTypeMidjourney, constant.ChannelTypeMidjourneyPlus:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeMidjourney}
	//case constant.ChannelTypeSunoAPI:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeSuno}
	//case constant.ChannelTypeKling:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeKling}
	//case constant.ChannelTypeJimeng:
	//	endpointTypes = []constant.EndpointType{constant.EndpointTypeJimeng}
	case constant.ChannelTypeAws:
		fallthrough // AWS Bedrock 使用与 Anthropic 相同的端点
	case constant.ChannelTypeAnthropic:
		// Anthropic/AWS 渠道优先使用 Anthropic 端点，也支持 OpenAI 端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeAnthropic, constant.EndpointTypeOpenAI}
	case constant.ChannelTypeVertexAi:
		fallthrough // Vertex AI 使用与 Gemini 相同的端点
	case constant.ChannelTypeGemini:
		// Gemini/Vertex 渠道优先使用 Gemini 端点，也支持 OpenAI 端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeGemini, constant.EndpointTypeOpenAI}
	case constant.ChannelTypeOpenRouter:
		// OpenRouter 只支持 OpenAI 端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
	case constant.ChannelTypeXai:
		// xAI 渠道支持 OpenAI 和 OpenAIResponse 端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI, constant.EndpointTypeOpenAIResponse}
	case constant.ChannelTypeSora:
		// Sora 渠道只支持视频生成端点
		endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIVideo}
	default:
		// 默认情况：根据模型名称判断使用 OpenAI 或 OpenAIResponse 端点
		if IsOpenAIResponseOnlyModel(modelName) {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAIResponse}
		} else {
			endpointTypes = []constant.EndpointType{constant.EndpointTypeOpenAI}
		}
	}
	// 如果是图像生成模型，在端点列表开头添加 ImageGeneration 端点
	if IsImageGenerationModel(modelName) {
		endpointTypes = append([]constant.EndpointType{constant.EndpointTypeImageGeneration}, endpointTypes...)
	}
	return endpointTypes
}
