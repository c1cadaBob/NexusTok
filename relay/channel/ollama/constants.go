// Package ollama 的常量定义文件。
// 定义 Ollama 渠道支持的模型列表和渠道名称。
package ollama

// ModelList 是 Ollama 支持的模型名称列表。
// 实际使用时，用户可以自行配置本地安装的任何模型。
// 此列表仅作为默认的模型选项。
var ModelList = []string{
	"llama3-7b",
}

// ChannelName 是 Ollama 渠道的标识名称。
var ChannelName = "ollama"
