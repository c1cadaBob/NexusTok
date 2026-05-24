// 智谱 GLM-4V 多模态模型渠道常量定义。
package zhipu_4v

// ModelList 智谱 GLM-4V 支持的模型列表。
// 包含 GLM-4 系列的各版本模型，支持文本、视觉、工具调用等多种能力。
var ModelList = []string{
	"glm-4",           // GLM-4 基础版
	"glm-4v",          // GLM-4V 视觉版（支持图片输入）
	"glm-3-turbo",     // GLM-3 Turbo 快速版
	"glm-4-alltools",  // GLM-4 全工具版（支持所有工具调用）
	"glm-4-plus",      // GLM-4 Plus 增强版
	"glm-4-0520",      // GLM-4 0520 版本
	"glm-4-air",       // GLM-4 Air 轻量版
	"glm-4-airx",      // GLM-4 AirX 超轻量版
	"glm-4-long",      // GLM-4 Long 长上下文版
	"glm-4-flash",     // GLM-4 Flash 极速版
	"glm-4v-plus",     // GLM-4V Plus 视觉增强版
	"glm-4.6",         // GLM-4.6 版本
	"glm-4.6v",        // GLM-4.6V 视觉版本
	"glm-4.7",         // GLM-4.7 版本
	"glm-4.7-flash",   // GLM-4.7 Flash 极速版
	"glm-5",           // GLM-5 最新版本
}

// ChannelName 智谱 GLM-4V 渠道的唯一标识名称。
var ChannelName = "zhipu_4v"
