// Mistral AI 通道的常量定义文件。
// 定义支持的模型列表和通道名称。
package mistral

// ModelList Mistral AI 支持的模型列表。
// 包含开源模型（7B、Mixtral）和商业模型（Small、Medium、Large），以及嵌入模型。
var ModelList = []string{
	"open-mistral-7b",        // Mistral 7B 开源模型
	"open-mixtral-8x7b",      // Mixtral 8x7B 开源 MoE 模型
	"mistral-small-latest",   // Mistral Small 轻量商业模型
	"mistral-medium-latest",  // Mistral Medium 中等规模商业模型
	"mistral-large-latest",   // Mistral Large 大型商业模型
	"mistral-embed",          // Mistral 文本嵌入模型
}

// ChannelName 通道名称，用于标识 Mistral AI 通道
var ChannelName = "mistral"
