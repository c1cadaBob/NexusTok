// Package common - model.go
// 该文件定义了模型名称匹配相关的工具函数和常量
//
// 包含的功能：
// - OpenAI Response Only 模型判断（仅支持 Responses API 的模型）
// - 图像生成模型判断（DALL-E、Imagen、Flux 等）
// - OpenAI 文本模型判断（GPT、o1、o3 等）
//
// 匹配规则：
// - 使用 strings.Contains 进行子串匹配
// - 使用 strings.HasPrefix 进行前缀匹配（带 "prefix:" 前缀的模式）
package common

import "strings"

var (
	// OpenAIResponseOnlyModels 仅支持 OpenAI Responses API 的模型列表
	// 这些模型不支持传统的 Chat Completions API
	OpenAIResponseOnlyModels = []string{
		"o3-pro",
		"o3-deep-research",
		"o4-mini-deep-research",
	}

	// ImageGenerationModels 图像生成模型列表
	// 支持子串匹配和前缀匹配（带 "prefix:" 前缀的模式）
	ImageGenerationModels = []string{
		"dall-e-3",         // DALL-E 3
		"dall-e-2",         // DALL-E 2
		"gpt-image-1",      // GPT Image 1
		"prefix:imagen-",   // Google Imagen 系列（前缀匹配）
		"flux-",            // Flux 系列
		"flux.1-",          // Flux.1 系列
	}

	// OpenAITextModels OpenAI 文本模型列表
	// 用于判断是否为 OpenAI 的文本生成模型
	OpenAITextModels = []string{
		"gpt-",   // GPT 系列
		"o1",     // o1 系列
		"o3",     // o3 系列
		"o4",     // o4 系列
		"chatgpt", // ChatGPT
	}
)

// IsOpenAIResponseOnlyModel 判断模型是否仅支持 OpenAI Responses API
//
// 参数：
//   - modelName: 模型名称
//
// 返回值：
//   - bool: 是否为 Response Only 模型
func IsOpenAIResponseOnlyModel(modelName string) bool {
	for _, m := range OpenAIResponseOnlyModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}

// IsImageGenerationModel 判断模型是否为图像生成模型
//
// 匹配规则：
// - 普通字符串：使用 strings.Contains 进行子串匹配
// - "prefix:" 前缀：使用 strings.HasPrefix 进行前缀匹配
//
// 参数：
//   - modelName: 模型名称
//
// 返回值：
//   - bool: 是否为图像生成模型
func IsImageGenerationModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range ImageGenerationModels {
		if strings.Contains(modelName, m) {
			return true
		}
		if strings.HasPrefix(m, "prefix:") && strings.HasPrefix(modelName, strings.TrimPrefix(m, "prefix:")) {
			return true
		}
	}
	return false
}

// IsOpenAITextModel 判断模型是否为 OpenAI 文本模型
//
// 参数：
//   - modelName: 模型名称
//
// 返回值：
//   - bool: 是否为 OpenAI 文本模型
func IsOpenAITextModel(modelName string) bool {
	modelName = strings.ToLower(modelName)
	for _, m := range OpenAITextModels {
		if strings.Contains(modelName, m) {
			return true
		}
	}
	return false
}
