// qwen.go — Qwen（通义千问）模型配置管理
// 职责：定义和管理 Qwen 模型的专属配置，包括同步图像生成模型列表等设置。
// 通过 config.GlobalConfig 注册实现持久化存储。

package model_setting

import (
	"strings"

	"github.com/c1cada/NexusTok/setting/config"
)

// QwenSettings defines Qwen model configuration. 注意bool要以enabled结尾才可以生效编辑
type QwenSettings struct {
	SyncImageModels []string `json:"sync_image_models"`
}

// 默认配置
var defaultQwenSettings = QwenSettings{
	SyncImageModels: []string{
		"z-image",
		"qwen-image",
		"wan2.6",
		"wan2.7",
		"qwen-image-edit",
		"qwen-image-edit-max",
		"qwen-image-edit-max-2026-01-16",
		"qwen-image-edit-plus",
		"qwen-image-edit-plus-2025-12-15",
		"qwen-image-edit-plus-2025-10-30",
	},
}

// 全局实例
var qwenSettings = defaultQwenSettings

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("qwen", &qwenSettings)
}

// GetQwenSettings 获取当前 Qwen 配置的指针。
func GetQwenSettings() *QwenSettings {
	return &qwenSettings
}

// IsSyncImageModel 判断指定模型是否为同步图像生成模型。
// 通过子串匹配检查模型名称是否包含列表中的任一关键词。
func IsSyncImageModel(model string) bool {
	for _, m := range qwenSettings.SyncImageModels {
		if strings.Contains(model, m) {
			return true
		}
	}
	return false
}
