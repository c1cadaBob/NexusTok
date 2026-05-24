// Package model - channel_satisfy.go
// 该文件提供了渠道模型匹配的辅助函数
//
// 核心功能：
// - IsChannelEnabledForGroupModel：检查指定渠道是否支持指定分组和模型
// - IsChannelEnabledForAnyGroupModel：检查渠道是否支持任意分组的指定模型
//
// 查询策略：
// 1. 内存缓存启用时从缓存查询（O(1) 复杂度）
// 2. 内存缓存未启用时从数据库查询
// 3. 支持模型名称标准化匹配（如 gpt-4o 匹配 gpt-4o-2024-05-13）
package model

import (
	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
)

// IsChannelEnabledForGroupModel 检查指定渠道是否支持指定分组和模型
// 优先从内存缓存查询，缓存未启用时回退到数据库查询
// 支持模型名称标准化匹配
func IsChannelEnabledForGroupModel(group string, modelName string, channelID int) bool {
	if group == "" || modelName == "" || channelID <= 0 {
		return false
	}
	if !common.MemoryCacheEnabled {
		return isChannelEnabledForGroupModelDB(group, modelName, channelID)
	}

	channelSyncLock.RLock()
	defer channelSyncLock.RUnlock()

	if group2model2channels == nil {
		return false
	}

	if isChannelIDInList(group2model2channels[group][modelName], channelID) {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized != "" && normalized != modelName {
		return isChannelIDInList(group2model2channels[group][normalized], channelID)
	}
	return false
}

// IsChannelEnabledForAnyGroupModel 检查渠道是否支持任意分组的指定模型
// 遍历所有分组，任一分组支持即返回 true
func IsChannelEnabledForAnyGroupModel(groups []string, modelName string, channelID int) bool {
	if len(groups) == 0 {
		return false
	}
	for _, g := range groups {
		if IsChannelEnabledForGroupModel(g, modelName, channelID) {
			return true
		}
	}
	return false
}

// isChannelEnabledForGroupModelDB 从数据库查询渠道是否支持指定分组和模型
// 支持模型名称标准化匹配
func isChannelEnabledForGroupModelDB(group string, modelName string, channelID int) bool {
	var count int64
	err := DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, modelName, channelID, true).
		Count(&count).Error
	if err == nil && count > 0 {
		return true
	}
	normalized := ratio_setting.FormatMatchingModelName(modelName)
	if normalized == "" || normalized == modelName {
		return false
	}
	count = 0
	err = DB.Model(&Ability{}).
		Where(commonGroupCol+" = ? and model = ? and channel_id = ? and enabled = ?", group, normalized, channelID, true).
		Count(&count).Error
	return err == nil && count > 0
}

// isChannelIDInList 检查渠道 ID 是否在列表中
func isChannelIDInList(list []int, channelID int) bool {
	for _, id := range list {
		if id == channelID {
			return true
		}
	}
	return false
}
