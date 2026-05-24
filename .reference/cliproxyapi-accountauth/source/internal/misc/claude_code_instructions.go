// 包 misc - claude_code_instructions.go
// 该文件嵌入了 Claude Code 模型交互的指令文本。
// 使用 go:embed 在编译时将外部文本文件嵌入到二进制文件中。
package misc

import _ "embed"

// ClaudeCodeInstructions 包含在编译时嵌入到应用程序二进制文件中的 claude_code_instructions.txt 文件内容。
// 该变量存储 Claude Code 模型交互和代码生成指导的特定指令。
//
//go:embed claude_code_instructions.txt
var ClaudeCodeInstructions string
