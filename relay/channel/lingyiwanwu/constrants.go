// Package lingyiwanwu 实现零一万物（01.AI）通道的常量定义。
// 零一万物是由李开复创立的 AI 公司，提供 Yi 系列大语言模型。
// 官方文档: https://platform.lingyiwanwu.com/docs
package lingyiwanwu

// https://platform.lingyiwanwu.com/docs

// ModelList 零一万物支持的模型列表。
// 包含 Yi 系列的多种模型，涵盖通用、视觉、长文本和 RAG 等场景。
var ModelList = []string{
	"yi-large",            // Yi 大型模型
	"yi-medium",           // Yi 中型模型
	"yi-vision",           // Yi 视觉多模态模型
	"yi-medium-200k",      // Yi 中型长上下文模型（200K tokens）
	"yi-spark",            // Yi 轻量快速模型
	"yi-large-rag",        // Yi 大型 RAG 增强模型
	"yi-large-turbo",      // Yi 大型加速模型
	"yi-large-preview",    // Yi 大型预览版模型
	"yi-large-rag-preview", // Yi 大型 RAG 增强预览版模型
}

// ChannelName 通道名称，用于标识零一万物通道
var ChannelName = "lingyiwanwu"
