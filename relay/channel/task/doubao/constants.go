// doubao - constants.go
// 豆包（Doubao / Seedance）视频生成任务的常量定义。
// 包含支持的模型列表、渠道名称标识，以及视频输入折扣比率配置。
// 视频输入折扣用于在检测到请求中包含视频输入时，自动降低计费倍率。
package doubao

// ModelList 豆包 Seedance 支持的视频生成模型列表。
// 包含不同版本和模式的模型：文生视频（t2v）、图生视频（i2v）等。
var ModelList = []string{
	"doubao-seedance-1-0-pro-250528",
	"doubao-seedance-1-0-lite-t2v",
	"doubao-seedance-1-0-lite-i2v",
	"doubao-seedance-1-5-pro-251215",
	"doubao-seedance-2-0-260128",
	"doubao-seedance-2-0-fast-260128",
}

// ChannelName 豆包视频生成渠道的唯一标识名称，用于路由和识别。
var ChannelName = "doubao-video"

// videoInputRatioMap 视频输入折扣比率映射表。
// 键为模型名称，值为折扣系数（含视频单价 / 不含视频单价）。
// 管理员应将 ModelRatio 设置为"不含视频"的较高费率，
// 系统在检测到视频输入时自动乘以此折扣比率，实现含视频的较低费率计费。
var videoInputRatioMap = map[string]float64{
	"doubao-seedance-2-0-260128":      28.0 / 46.0, // ~0.6087
	"doubao-seedance-2-0-fast-260128": 22.0 / 37.0, // ~0.5946
}

// GetVideoInputRatio 获取指定模型的视频输入折扣比率。
// 如果该模型支持视频输入折扣，返回折扣系数和 true；否则返回 0 和 false。
//
// 参数：
//   - modelName: 模型名称
//
// 返回：
//   - float64: 折扣系数
//   - bool: 该模型是否支持视频输入折扣
func GetVideoInputRatio(modelName string) (float64, bool) {
	r, ok := videoInputRatioMap[modelName]
	return r, ok
}
