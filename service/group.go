// group.go - 用户分组管理服务
// 本文件提供用户分组相关的查询和计算功能。
// 包括用户可用分组列表获取、分组归属判断、自动分组配置、
// 以及分组倍率查询等。支持特殊分组的添加和移除操作。
package service

import (
	"strings"

	"github.com/c1cada/NexusTok/setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"
)

// GetUserUsableGroups 获取指定用户可使用的分组列表。
// 基于系统全局可用分组列表，结合用户分组的特殊配置（添加/移除分组），
// 返回该用户最终可用的分组映射表。
// 参数:
//   - userGroup: 用户所属分组名称
// 返回值:
//   - map[string]string: 分组名称 -> 分组描述 的映射
func GetUserUsableGroups(userGroup string) map[string]string {
	groupsCopy := setting.GetUserUsableGroupsCopy()
	if userGroup != "" {
		specialSettings, b := ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)
		if b {
			// 处理特殊可用分组
			for specialGroup, desc := range specialSettings {
				if strings.HasPrefix(specialGroup, "-:") {
					// 移除分组
					groupToRemove := strings.TrimPrefix(specialGroup, "-:")
					delete(groupsCopy, groupToRemove)
				} else if strings.HasPrefix(specialGroup, "+:") {
					// 添加分组
					groupToAdd := strings.TrimPrefix(specialGroup, "+:")
					groupsCopy[groupToAdd] = desc
				} else {
					// 直接添加分组
					groupsCopy[specialGroup] = desc
				}
			}
		}
		// 如果userGroup不在UserUsableGroups中，返回UserUsableGroups + userGroup
		if _, ok := groupsCopy[userGroup]; !ok {
			groupsCopy[userGroup] = "用户分组"
		}
	}
	return groupsCopy
}

// GroupInUserUsableGroups 判断指定分组是否在用户的可用分组列表中。
// 参数:
//   - userGroup: 用户所属分组名称
//   - groupName: 待检查的分组名称
// 返回值:
//   - bool: true 表示该分组在用户可用列表中
func GroupInUserUsableGroups(userGroup, groupName string) bool {
	_, ok := GetUserUsableGroups(userGroup)[groupName]
	return ok
}

// GetUserAutoGroup 根据用户分组获取自动分组设置
func GetUserAutoGroup(userGroup string) []string {
	groups := GetUserUsableGroups(userGroup)
	autoGroups := make([]string, 0)
	for _, group := range setting.GetAutoGroups() {
		if _, ok := groups[group]; ok {
			autoGroups = append(autoGroups, group)
		}
	}
	return autoGroups
}

// GetUserGroupRatio 获取用户使用某个分组的倍率
// userGroup 用户分组
// group 需要获取倍率的分组
func GetUserGroupRatio(userGroup, group string) float64 {
	ratio, ok := ratio_setting.GetGroupGroupRatio(userGroup, group)
	if ok {
		return ratio
	}
	return ratio_setting.GetGroupRatio(group)
}
