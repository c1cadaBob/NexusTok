// translator - init.go
// 该文件通过空白导入注册所有内置的请求格式转换器，覆盖 Claude、Codex、Gemini、
// Gemini CLI、OpenAI 和 Antigravity 六种 API 格式之间的交叉转换。
// 每个导入对应一个源格式到目标格式的转换实现，在包初始化时自动注册到全局转换器注册表。
package translator

import (
	// Claude -> Gemini/Gemini-CLI/OpenAI 转换器
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/claude/gemini"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/claude/gemini-cli"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/claude/openai/chat-completions"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/claude/openai/responses"

	// Codex -> Claude/Gemini/Gemini-CLI/OpenAI 转换器
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/codex/claude"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/codex/gemini"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/codex/gemini-cli"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/codex/openai/chat-completions"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/codex/openai/responses"

	// Gemini-CLI -> Claude/Gemini/OpenAI 转换器
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini-cli/claude"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini-cli/gemini"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini-cli/openai/chat-completions"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini-cli/openai/responses"

	// Gemini -> Claude/Gemini-CLI/OpenAI 转换器
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/claude"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/gemini"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/gemini-cli"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/openai/chat-completions"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/gemini/openai/responses"

	// OpenAI -> Claude/Gemini/Gemini-CLI 转换器
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/openai/claude"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/openai/gemini"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/openai/gemini-cli"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/openai/openai/chat-completions"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/openai/openai/responses"

	// Antigravity -> Claude/Gemini/OpenAI 转换器
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/antigravity/claude"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/antigravity/gemini"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/antigravity/openai/chat-completions"
	_ "github.com/router-for-me/CLIProxyAPI/v7/internal/translator/antigravity/openai/responses"
)
