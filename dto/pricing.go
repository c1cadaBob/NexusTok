// Package dto - pricing.go
// 该文件定义了模型定价相关的数据传输对象
//
// 主要结构体：
// - OpenAIModels：OpenAI 格式的模型信息（用于 /v1/models 响应）
// - AnthropicModel：Anthropic Claude 格式的模型信息
// - GeminiModel：Google Gemini 格式的模型信息
//
// 说明：这些结构体用于统一各厂商的模型列表响应格式
package dto

import "github.com/c1cada/NexusTok/constant"

// OpenAIModels OpenAI 格式的模型信息
// Id：模型唯一标识
// Object：对象类型（"model"）
// Created：创建时间戳
// OwnedBy：模型所有者（如 "openai"、"anthropic" 等）
// SupportedEndpointTypes：支持的端点类型列表
// 注：此结构体原本希望独立出来，但由于依赖较多暂未重构
type OpenAIModels struct {
	Id                     string                  `json:"id"`
	Object                 string                  `json:"object"`
	Created                int                     `json:"created"`
	OwnedBy                string                  `json:"owned_by"`
	SupportedEndpointTypes []constant.EndpointType `json:"supported_endpoint_types"`
}

// AnthropicModel Anthropic Claude 格式的模型信息
// ID：模型唯一标识
// CreatedAt：创建时间
// DisplayName：显示名称
// Type：模型类型
type AnthropicModel struct {
	ID          string `json:"id"`
	CreatedAt   string `json:"created_at"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"`
}

// GeminiModel Google Gemini 格式的模型信息
// Name：模型资源名称（如 "models/gemini-2.5-pro"）
// BaseModelId：基础模型 ID
// Version：模型版本
// DisplayName：显示名称
// Description：模型描述
// InputTokenLimit：输入 token 上限
// OutputTokenLimit：输出 token 上限
// SupportedGenerationMethods：支持的生成方法列表
// Thinking：是否支持思考模式
// Temperature/MaxTemperature：温度参数范围
// TopP/TopK：采样参数
// 注：所有字段使用 interface{} 类型以兼容不同上游返回格式
type GeminiModel struct {
	Name                       interface{}   `json:"name"`
	BaseModelId                interface{}   `json:"baseModelId"`
	Version                    interface{}   `json:"version"`
	DisplayName                interface{}   `json:"displayName"`
	Description                interface{}   `json:"description"`
	InputTokenLimit            interface{}   `json:"inputTokenLimit"`
	OutputTokenLimit           interface{}   `json:"outputTokenLimit"`
	SupportedGenerationMethods []interface{} `json:"supportedGenerationMethods"`
	Thinking                   interface{}   `json:"thinking"`
	Temperature                interface{}   `json:"temperature"`
	MaxTemperature             interface{}   `json:"maxTemperature"`
	TopP                       interface{}   `json:"topP"`
	TopK                       interface{}   `json:"topK"`
}
