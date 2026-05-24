// Package controller - group.go
// 该文件实现了用户组管理的 API 控制器
//
// 用户组用于控制用户的配额倍率和可用模型
// 不同用户组可以有不同的计费策略
//
// 主要 API：
// - GetGroups：获取所有用户组列表
// - GetUserGroups：获取当前用户可用的用户组
package controller

import (
	"net/http"

	"github.com/c1cada/NexusTok/model"
	"github.com/c1cada/NexusTok/service"
	"github.com/c1cada/NexusTok/setting"
	"github.com/c1cada/NexusTok/setting/ratio_setting"

	"github.com/gin-gonic/gin"
)

// GetGroups 获取所有用户组列表
//
// 从配比配置中获取所有定义的用户组名称
//
// 参数：
//   - c: Gin 上下文
func GetGroups(c *gin.Context) {
	groupNames := make([]string, 0)
	for groupName := range ratio_setting.GetGroupRatioCopy() {
		groupNames = append(groupNames, groupName)
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    groupNames,
	})
}

// GetUserGroups 获取当前用户可用的用户组
//
// 返回用户可用的用户组及其倍率信息
// 包括：
// - 用户组名称
// - 配额倍率
// - 用户组描述
//
// 特殊用户组 "auto" 表示自动选择最优用户组
//
// 参数：
//   - c: Gin 上下文
func GetUserGroups(c *gin.Context) {
	usableGroups := make(map[string]map[string]interface{})
	userGroup := ""
	userId := c.GetInt("id")
	userGroup, _ = model.GetUserGroup(userId, false)
	userUsableGroups := service.GetUserUsableGroups(userGroup)
	for groupName, _ := range ratio_setting.GetGroupRatioCopy() {
		// 检查用户是否可以使用该用户组
		if desc, ok := userUsableGroups[groupName]; ok {
			usableGroups[groupName] = map[string]interface{}{
				"ratio": service.GetUserGroupRatio(userGroup, groupName),
				"desc":  desc,
			}
		}
	}
	// 添加 "auto" 用户组选项
	if _, ok := userUsableGroups["auto"]; ok {
		usableGroups["auto"] = map[string]interface{}{
			"ratio": "自动",
			"desc":  setting.GetUsableGroupDescription("auto"),
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    usableGroups,
	})
}
