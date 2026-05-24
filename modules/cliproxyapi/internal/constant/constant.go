// Package constant 定义了 CLI Proxy API 中使用的服务商名称常量。
// 这些常量标识不同的 AI 服务商及其变体，确保整个应用程序中命名的一致性。
//
// Package constant defines provider name constants used throughout the CLI Proxy API.
package constant

const (
	// Gemini 代表 Google Gemini 服务商标识符。
	// Gemini represents the Google Gemini provider identifier.
	Gemini = "gemini"

	// GeminiCLI 代表 Google Gemini CLI 服务商标识符。
	// GeminiCLI represents the Google Gemini CLI provider identifier.
	GeminiCLI = "gemini-cli"

	// Codex 代表 OpenAI Codex 服务商标识符。
	// Codex represents the OpenAI Codex provider identifier.
	Codex = "codex"

	// Claude 代表 Anthropic Claude 服务商标识符。
	// Claude represents the Anthropic Claude provider identifier.
	Claude = "claude"

	// OpenAI 代表 OpenAI 服务商标识符。
	// OpenAI represents the OpenAI provider identifier.
	OpenAI = "openai"

	// OpenaiResponse 代表 OpenAI 响应格式标识符。
	// OpenaiResponse represents the OpenAI response format identifier.
	OpenaiResponse = "openai-response"

	// Antigravity 代表 Antigravity 响应格式标识符。
	// Antigravity represents the Antigravity response format identifier.
	Antigravity = "antigravity"
)
