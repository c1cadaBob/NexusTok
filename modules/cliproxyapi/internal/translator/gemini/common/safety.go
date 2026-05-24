// gemini/common - safety.go
// 本文件提供 Gemini API 请求的安全设置功能。
// 包含默认安全设置的定义以及将默认安全设置附加到请求 JSON 中的工具函数。
// 安全设置用于控制 Gemini API 对各类有害内容的过滤行为。
package common

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// DefaultSafetySettings 返回附加到请求中的默认 Gemini 安全配置。
// 所有有害内容类别均设置为关闭（OFF）或不过滤（BLOCK_NONE），
// 以确保请求不会因安全过滤而被拒绝。
func DefaultSafetySettings() []map[string]string {
	return []map[string]string{
		{
			"category":  "HARM_CATEGORY_HARASSMENT",      // 骚扰类别
			"threshold": "OFF",                            // 关闭过滤
		},
		{
			"category":  "HARM_CATEGORY_HATE_SPEECH",     // 仇恨言论类别
			"threshold": "OFF",                            // 关闭过滤
		},
		{
			"category":  "HARM_CATEGORY_SEXUALLY_EXPLICIT", // 色情内容类别
			"threshold": "OFF",                              // 关闭过滤
		},
		{
			"category":  "HARM_CATEGORY_DANGEROUS_CONTENT", // 危险内容类别
			"threshold": "OFF",                              // 关闭过滤
		},
		{
			"category":  "HARM_CATEGORY_CIVIC_INTEGRITY",   // 公民完整性类别
			"threshold": "BLOCK_NONE",                       // 不过滤
		},
	}
}

// AttachDefaultSafetySettings 确保在请求中存在默认安全设置。
// 如果指定路径上已存在安全设置，则不做修改直接返回原始 JSON。
// 调用方必须提供目标 JSON 路径（例如 "safetySettings" 或 "request.safetySettings"）。
//
// 参数：
//   - rawJSON: 原始请求 JSON 字节数据
//   - path: 安全设置在 JSON 中的目标路径
//
// 返回：
//   - []byte: 附加安全设置后的 JSON 字节数据
func AttachDefaultSafetySettings(rawJSON []byte, path string) []byte {
	if gjson.GetBytes(rawJSON, path).Exists() {
		return rawJSON
	}

	out, err := sjson.SetBytes(rawJSON, path, DefaultSafetySettings())
	if err != nil {
		return rawJSON
	}

	return out
}
