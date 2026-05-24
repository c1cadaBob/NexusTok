// Package suno 实现 Suno AI 音乐生成渠道的模型定义
package suno

// ModelList 支持的 Suno 模型列表
// suno_music: 音乐生成，suno_lyrics: 歌词生成
var ModelList = []string{
	"suno_music", "suno_lyrics",
}

// ChannelName 渠道名称标识
var ChannelName = "suno"
