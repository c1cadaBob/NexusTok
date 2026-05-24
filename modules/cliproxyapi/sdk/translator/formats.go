// translator - formats.go
// 该文件定义了 SDK 用户可使用的所有通用格式标识符常量。
// 包括 OpenAI、Claude、Gemini、Codex、Antigravity 等 AI 提供商的格式。

package translator

// Common format identifiers exposed for SDK users.
// 以下是暴露给 SDK 用户的通用格式标识符常量。
const (
	// FormatOpenAI 标识 OpenAI Chat Completions 格式
	FormatOpenAI Format = "openai"
	// FormatOpenAIResponse 标识 OpenAI Responses API 格式
	FormatOpenAIResponse Format = "openai-response"
	// FormatClaude 标识 Anthropic Claude 格式
	FormatClaude Format = "claude"
	// FormatGemini 标识 Google Gemini 格式
	FormatGemini Format = "gemini"
	// FormatGeminiCLI 标识 Gemini CLI 格式
	FormatGeminiCLI Format = "gemini-cli"
	// FormatCodex 标识 OpenAI Codex 格式
	FormatCodex Format = "codex"
	// FormatAntigravity 标识 Antigravity 格式
	FormatAntigravity Format = "antigravity"
)
