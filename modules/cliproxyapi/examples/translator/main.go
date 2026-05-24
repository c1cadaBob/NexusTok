// Package main - translator 示例
// 演示如何使用 SDK 翻译器（translator）在不同 AI 提供者格式之间进行请求和响应的转换。
// 本示例展示了：
// - 检查是否存在特定格式间的响应转换器
// - 将 OpenAI 格式的请求转换为 Gemini 格式
// - 将 Gemini 格式的响应转换为 OpenAI 格式
package main

import (
	"context"
	"fmt"

	"github.com/router-for-me/CLIProxyAPI/v7/sdk/translator"
	// 导入内置翻译器包以触发 init() 注册。
	_ "github.com/router-for-me/CLIProxyAPI/v7/sdk/translator/builtin"
)

// main 是示例程序的入口函数。
// 它演示了 OpenAI 与 Gemini 格式之间的请求和响应转换。
func main() {
	// 原始的 OpenAI 格式请求。
	rawRequest := []byte(`{"messages":[{"content":[{"text":"Hello! Gemini","type":"text"}],"role":"user"}],"model":"gemini-2.5-pro","stream":false}`)

	// 检查是否存在 Gemini -> OpenAI 的响应转换器。
	fmt.Println("Has gemini->openai response translator:", translator.HasResponseTransformerByFormatName(
		translator.FormatGemini,
		translator.FormatOpenAI,
	))

	// 将 OpenAI 格式的请求转换为 Gemini 格式。
	translatedRequest := translator.TranslateRequestByFormatName(
		translator.FormatOpenAI,
		translator.FormatGemini,
		"gemini-2.5-pro",
		rawRequest,
		false,
	)

	fmt.Printf("Translated request to Gemini format:\n%s\n\n", translatedRequest)

	// 模拟的 Gemini 格式响应（包含思考过程和函数调用）。
	claudeResponse := []byte(`{"candidates":[{"content":{"role":"model","parts":[{"thought":true,"text":"Okay, here's what's going through my mind. I need to schedule a meeting"},{"thoughtSignature":"","functionCall":{"name":"schedule_meeting","args":{"topic":"Q3 planning","attendees":["Bob","Alice"],"time":"10:00","date":"2025-03-27"}}}]},"finishReason":"STOP","avgLogprobs":-0.50018133435930523}],"usageMetadata":{"promptTokenCount":117,"candidatesTokenCount":28,"totalTokenCount":474,"trafficType":"PROVISIONED_THROUGHPUT","promptTokensDetails":[{"modality":"TEXT","tokenCount":117}],"candidatesTokensDetails":[{"modality":"TEXT","tokenCount":28}],"thoughtsTokenCount":329},"modelVersion":"gemini-2.5-pro","createTime":"2025-08-15T04:12:55.249090Z","responseId":"x7OeaIKaD6CU48APvNXDyA4"}`)

	// 将 Gemini 格式的响应转换为 OpenAI 格式。
	convertedResponse := translator.TranslateNonStreamByFormatName(
		context.Background(),
		translator.FormatGemini,
		translator.FormatOpenAI,
		"gemini-2.5-pro",
		rawRequest,
		translatedRequest,
		claudeResponse,
		nil,
	)

	fmt.Printf("Converted response for OpenAI clients:\n%s\n", convertedResponse)
}
