// Package misc 提供 CLI Proxy API 的杂项工具函数和嵌入数据。
// 该包包含通用辅助函数和嵌入资源，包括 Claude Code 相关操作的嵌入式指令文本。
package misc

import _ "embed"

// ClaudeCodeInstructions 存储 claude_code_instructions.txt 文件的内容，
// 该文件在编译时通过 go:embed 指令嵌入到应用程序二进制文件中。
// 包含 Claude Code 模型交互和代码生成的特定指令。
//
//go:embed claude_code_instructions.txt
var ClaudeCodeInstructions string
