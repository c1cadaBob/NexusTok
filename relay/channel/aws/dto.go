// aws - dto.go
// AWS Bedrock 渠道的数据传输对象（DTO）定义。
// 本文件定义了与 AWS Bedrock API 交互所需的请求结构体，
// 包括 Claude 模型的 AWS 请求格式（AwsClaudeRequest）、
// Amazon Nova 模型的请求格式（NovaRequest、NovaMessage、NovaContent、NovaInferenceConfig），
// 以及请求格式转换和辅助函数。
package aws

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/dto"
	"github.com/c1cada/NexusTok/logger"
)

// AwsClaudeRequest 定义发往 AWS Bedrock 的 Claude 模型请求体结构。
// 该结构在标准 Claude 请求基础上增加了 AWS Bedrock 特有的字段（如 anthropic_version），
// 用于通过 AWS Bedrock 的 InvokeModel / InvokeModelWithResponseStream API 调用 Claude 模型。
type AwsClaudeRequest struct {
	// AnthropicVersion Anthropic API 版本号，AWS Bedrock 要求固定为 "bedrock-2023-05-31"。
	AnthropicVersion string `json:"anthropic_version"`
	// AnthropicBeta Anthropic Beta 功能标记列表，从请求头 "anthropic-beta" 中解析。
	// 使用 json.RawMessage 存储，避免序列化/反序列化时的数据损失。
	AnthropicBeta json.RawMessage `json:"anthropic_beta,omitempty"`
	// System 系统提示词，支持字符串或结构化内容块等多种格式。
	System any `json:"system,omitempty"`
	// Messages 对话消息列表，包含角色和内容。
	Messages []dto.ClaudeMessage `json:"messages"`
	// MaxTokens 最大生成的 Token 数量。
	MaxTokens uint `json:"max_tokens,omitempty"`
	// Temperature 采样温度，控制生成文本的随机性，范围 0-1。
	// 使用指针类型以区分"未设置"和"设置为 0"。
	Temperature *float64 `json:"temperature,omitempty"`
	// TopP 核采样（nucleus sampling）参数，范围 0-1。
	TopP float64 `json:"top_p,omitempty"`
	// TopK 限制每一步考虑的候选 Token 数量。
	TopK int `json:"top_k,omitempty"`
	// StopSequences 停止生成的序列列表，当生成内容匹配时停止继续生成。
	StopSequences []string `json:"stop_sequences,omitempty"`
	// Tools 工具/函数定义列表，用于函数调用功能。支持多种格式。
	Tools any `json:"tools,omitempty"`
	// ToolChoice 工具选择策略，控制模型如何调用工具。
	ToolChoice any `json:"tool_choice,omitempty"`
	// Thinking 思维链配置，用于启用扩展思考功能。
	Thinking *dto.Thinking `json:"thinking,omitempty"`
	// OutputConfig 输出配置，控制响应格式等。
	OutputConfig json.RawMessage `json:"output_config,omitempty"`
	//Metadata         json.RawMessage     `json:"metadata,omitempty"`
}

// formatRequest 将原始请求体和请求头格式化为 AWS Bedrock 的 Claude 请求格式。
// 处理逻辑:
//   - 从请求体中解码 JSON 为 AwsClaudeRequest 结构
//   - 强制设置 AnthropicVersion 为 "bedrock-2023-05-31"（AWS Bedrock 要求的固定值）
//   - 从请求头中读取 "anthropic-beta" 字段，按逗号分隔解析为字符串数组后设置到请求体中
//   - 记录格式化后的请求体 JSON 日志
//
// 参数:
//   - requestBody: 原始请求体的 io.Reader
//   - requestHeader: HTTP 请求头，包含 anthropic-beta 等自定义头
//
// 返回: 格式化后的 AWS Claude 请求结构指针和错误信息。
func formatRequest(requestBody io.Reader, requestHeader http.Header) (*AwsClaudeRequest, error) {
	var awsClaudeRequest AwsClaudeRequest
	err := common.DecodeJson(requestBody, &awsClaudeRequest)
	if err != nil {
		return nil, err
	}
	awsClaudeRequest.AnthropicVersion = "bedrock-2023-05-31"

	// check header anthropic-beta
	anthropicBetaValues := requestHeader.Get("anthropic-beta")
	if len(anthropicBetaValues) > 0 {
		var tempArray []string
		tempArray = strings.Split(anthropicBetaValues, ",")
		if len(tempArray) > 0 {
			betaJson, err := json.Marshal(tempArray)
			if err != nil {
				return nil, err
			}
			awsClaudeRequest.AnthropicBeta = betaJson
		}
	}
	logger.LogJson(context.Background(), "json", awsClaudeRequest)
	return &awsClaudeRequest, nil
}

// NovaMessage 定义 Amazon Nova 模型使用的 messages-v1 格式的单条消息结构。
// Nova 模型使用与标准 Claude/OpenAI 不同的消息格式，每条消息包含角色和内容列表。
type NovaMessage struct {
	// Role 消息角色，如 "user"、"assistant"。
	Role string `json:"role"`
	// Content 消息内容列表，每条消息可以包含多个内容块。
	Content []NovaContent `json:"content"`
}

// NovaContent 定义 Nova 消息中的单个内容块结构。
// 当前仅支持文本内容。
type NovaContent struct {
	// Text 文本内容。
	Text string `json:"text"`
}

// NovaRequest 定义发往 AWS Bedrock 的 Amazon Nova 模型请求体结构。
// Nova 模型使用 schemaVersion 为 "messages-v1" 的请求格式。
type NovaRequest struct {
	// SchemaVersion 请求协议版本号，Nova 模型固定为 "messages-v1"。
	SchemaVersion string `json:"schemaVersion"` // 请求版本，例如 "1.0"
	// Messages 对话消息列表，包含角色和内容。
	Messages []NovaMessage `json:"messages"` // 对话消息列表
	// InferenceConfig 推理配置参数，控制生成行为。可选。
	InferenceConfig *NovaInferenceConfig `json:"inferenceConfig,omitempty"` // 推理配置，可选
}

// NovaInferenceConfig 定义 Nova 模型的推理配置参数。
// 控制模型生成文本时的各种采样和限制参数。
type NovaInferenceConfig struct {
	// MaxTokens 最大生成的 Token 数量。
	MaxTokens int `json:"maxTokens,omitempty"` // 最大生成的 token 数
	// Temperature 采样温度，控制随机性。默认 0.7，范围 0-1。
	Temperature float64 `json:"temperature,omitempty"` // 随机性 (默认 0.7, 范围 0-1)
	// TopP 核采样参数。默认 0.9，范围 0-1。
	TopP float64 `json:"topP,omitempty"` // nucleus sampling (默认 0.9, 范围 0-1)
	// TopK 限制候选 Token 数量。默认 50，范围 0-128。
	TopK int `json:"topK,omitempty"` // 限制候选 token 数 (默认 50, 范围 0-128)
	// StopSequences 停止生成的序列列表，当生成内容匹配时停止继续生成。
	StopSequences []string `json:"stopSequences,omitempty"` // 停止生成的序列
}

// convertToNovaRequest 将 OpenAI 格式的请求转换为 Amazon Nova 模型的请求格式。
// 转换逻辑:
//   - 将 OpenAI 消息列表中的每条消息转换为 NovaMessage 格式
//   - 保留原始角色（role），将消息内容转换为 NovaContent 文本块
//   - 如果 OpenAI 请求中设置了 MaxTokens、Temperature、TopP、TopK 或 Stop 参数，
//     则构造对应的 NovaInferenceConfig
//   - Stop 参数支持字符串或字符串数组格式，通过 parseStopSequences 进行解析
//
// 参数:
//   - req: OpenAI 格式的通用请求体
//
// 返回: Nova 格式的请求体指针。
func convertToNovaRequest(req *dto.GeneralOpenAIRequest) *NovaRequest {
	novaMessages := make([]NovaMessage, len(req.Messages))
	for i, msg := range req.Messages {
		novaMessages[i] = NovaMessage{
			Role:    msg.Role,
			Content: []NovaContent{{Text: msg.StringContent()}},
		}
	}

	novaReq := &NovaRequest{
		SchemaVersion: "messages-v1",
		Messages:      novaMessages,
	}

	// 设置推理配置
	if (req.MaxTokens != nil && *req.MaxTokens != 0) || (req.Temperature != nil && *req.Temperature != 0) || (req.TopP != nil && *req.TopP != 0) || (req.TopK != nil && *req.TopK != 0) || req.Stop != nil {
		novaReq.InferenceConfig = &NovaInferenceConfig{}
		if req.MaxTokens != nil && *req.MaxTokens != 0 {
			novaReq.InferenceConfig.MaxTokens = int(*req.MaxTokens)
		}
		if req.Temperature != nil && *req.Temperature != 0 {
			novaReq.InferenceConfig.Temperature = *req.Temperature
		}
		if req.TopP != nil && *req.TopP != 0 {
			novaReq.InferenceConfig.TopP = *req.TopP
		}
		if req.TopK != nil && *req.TopK != 0 {
			novaReq.InferenceConfig.TopK = *req.TopK
		}
		if req.Stop != nil {
			if stopSequences := parseStopSequences(req.Stop); len(stopSequences) > 0 {
				novaReq.InferenceConfig.StopSequences = stopSequences
			}
		}
	}

	return novaReq
}

// parseStopSequences 解析停止序列参数，支持多种输入格式。
// 由于 OpenAI 请求中的 Stop 字段类型为 any，需要兼容以下格式:
//   - string: 单个停止序列字符串，转换为 []string
//   - []string: 字符串数组，直接返回
//   - []interface{}: JSON 反序列化后的通用数组，逐项提取字符串
//
// 参数:
//   - stop: 原始停止序列参数，支持任意类型
//
// 返回: 解析后的字符串切片，如果输入为 nil 或不包含有效内容则返回 nil。
func parseStopSequences(stop any) []string {
	if stop == nil {
		return nil
	}

	switch v := stop.(type) {
	case string:
		if v != "" {
			return []string{v}
		}
	case []string:
		return v
	case []interface{}:
		var sequences []string
		for _, item := range v {
			if str, ok := item.(string); ok && str != "" {
				sequences = append(sequences, str)
			}
		}
		return sequences
	}
	return nil
}
