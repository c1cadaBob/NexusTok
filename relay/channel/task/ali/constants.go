// ali - constants.go
// 阿里云通义万相视频生成任务的常量定义。
// 包含支持的模型列表和渠道名称标识。
package ali

// ModelList 阿里云通义万相支持的视频生成模型列表。
// 包含不同版本和模式的模型：文生视频（t2v）、图生视频（i2v）、首尾帧生视频（kf2v）等。
var ModelList = []string{
	"wan2.5-i2v-preview", // 万相2.5 preview（有声视频）推荐
	"wan2.2-i2v-flash",   // 万相2.2极速版（无声视频）
	"wan2.2-i2v-plus",    // 万相2.2专业版（无声视频）
	"wanx2.1-i2v-plus",   // 万相2.1专业版（无声视频）
	"wanx2.1-i2v-turbo",  // 万相2.1极速版（无声视频）
}

// ChannelName 阿里云通义万相渠道的唯一标识名称，用于路由和识别。
var ChannelName = "ali"
