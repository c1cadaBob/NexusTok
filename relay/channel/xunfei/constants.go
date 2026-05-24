// Package xunfei 讯飞星火大模型渠道常量定义。
package xunfei

// ModelList 讯飞星火支持的模型列表。
// 包含从 v1.1 到 v4.0 的各版本星火大模型。
var ModelList = []string{
	"SparkDesk",       // 星火大模型默认版本
	"SparkDesk-v1.1",  // 星火 v1.1 版本
	"SparkDesk-v2.1",  // 星火 v2.1 版本
	"SparkDesk-v3.1",  // 星火 v3.1 版本
	"SparkDesk-v3.5",  // 星火 v3.5 版本
	"SparkDesk-v4.0",  // 星火 v4.0 版本（Ultra）
}

// ChannelName 讯飞星火渠道的唯一标识名称。
var ChannelName = "xunfei"
