// codex/claude/init.go
// 本文件负责在包初始化时注册 Claude Code API 到 Codex API 的协议转换器。
// 通过 init() 函数自动完成注册，将请求转换函数和响应转换函数绑定到
// translator 框架中，使系统能够自动将 Claude Code 格式的请求/响应
// 翻译为 Codex 格式。

package claude

import (
	// 点导入：将 constant 包中的常量直接引入当前作用域，简化常量引用
	. "github.com/router-for-me/CLIProxyAPI/v7/internal/constant"
	// 接口定义包：定义了 TranslateResponse 等接口类型
	"github.com/router-for-me/CLIProxyAPI/v7/internal/interfaces"
	// translator 核心包：提供 Register 函数用于注册协议转换器
	"github.com/router-for-me/CLIProxyAPI/v7/internal/translator/translator"
)

// init 包初始化函数，自动在程序启动时执行。
// 注册 Claude Code API (源协议) 到 Codex API (目标协议) 的转换器，
// 包括请求转换函数和三类响应转换函数（流式、非流式、Token 计数）。
func init() {
	translator.Register(
		// 源协议：Claude Code API
		Claude,
		// 目标协议：Codex API
		Codex,
		// 请求转换函数：将 Claude 格式请求转为 Codex 格式
		ConvertClaudeRequestToCodex,
		// 响应转换函数集合
		interfaces.TranslateResponse{
			// 流式响应转换
			Stream: ConvertCodexResponseToClaude,
			// 非流式响应转换
			NonStream: ConvertCodexResponseToClaudeNonStream,
			// Token 数量格式化
			TokenCount: ClaudeTokenCount,
		},
	)
}
