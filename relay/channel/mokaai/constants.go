// Moka AI 通道的常量定义文件。
// 定义支持的模型列表和通道名称。
// Moka AI 提供 m3e 系列文本嵌入模型。
package mokaai

// ModelList Moka AI 支持的模型列表。
// m3e 系列为文本嵌入（Embedding）模型，用于文本向量化。
var ModelList = []string{
	"m3e-large", // m3e 大型嵌入模型
	"m3e-base",  // m3e 基础嵌入模型
	"m3e-small", // m3e 小型嵌入模型
}

// ChannelName 通道名称，用于标识 Moka AI 通道
var ChannelName = "mokaai"
