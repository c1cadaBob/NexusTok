// MiniMax 通道的常量定义文件。
// 定义支持的模型列表和通道名称。
// MiniMax 提供对话模型（abab 系列）、语音合成模型和图片生成模型。
// 参考文档: https://www.minimaxi.com/document/guides/chat-model/V2?id=65e0736ab2845de20908e2dd
package minimax

// https://www.minimaxi.com/document/guides/chat-model/V2?id=65e0736ab2845de20908e2dd

// ModelList MiniMax 支持的模型列表。
// 包含对话模型、语音合成模型和图片生成模型。
var ModelList = []string{
	"abab6.5-chat",                // ABAB 6.5 对话模型
	"abab6.5s-chat",              // ABAB 6.5s 轻量对话模型
	"abab6-chat",                 // ABAB 6 对话模型
	"abab5.5-chat",               // ABAB 5.5 对话模型
	"abab5.5s-chat",              // ABAB 5.5s 轻量对话模型
	"MiniMax-M2.7",               // MiniMax M2.7 对话模型
	"MiniMax-M2.7-highspeed",     // MiniMax M2.7 高速版
	"speech-2.5-hd-preview",      // 语音合成 2.5 高清预览版
	"speech-2.5-turbo-preview",   // 语音合成 2.5 快速预览版
	"speech-02-hd",               // 语音合成 02 高清版
	"speech-02-turbo",            // 语音合成 02 快速版
	"speech-01-hd",               // 语音合成 01 高清版
	"speech-01-turbo",            // 语音合成 01 快速版
	"MiniMax-M2.1",               // MiniMax M2.1 对话模型
	"MiniMax-M2.1-highspeed",     // MiniMax M2.1 高速版
	"MiniMax-M2",                 // MiniMax M2 对话模型
	"MiniMax-M2.5",               // MiniMax M2.5 对话模型
	"MiniMax-M2.5-highspeed",     // MiniMax M2.5 高速版
	"image-01",                   // 图片生成模型 01
	"image-01-live",              // 图片生成模型 01 实时版
}

// ChannelName 通道名称，用于标识 MiniMax 通道
var ChannelName = "minimax"
