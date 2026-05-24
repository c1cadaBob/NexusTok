// Package helps - thinking_providers.go
// 导入所有思维/推理提供商的实现，通过空白导入触发提供商的注册。
// 支持的提供商包括 Antigravity、Claude、Codex、Gemini、GeminiCLI、Kimi、OpenAI 和 xAI。
package helps

import (
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/antigravity"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/claude"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/codex"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/gemini"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/geminicli"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/kimi"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/openai"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/thinking/provider/xai"
)
