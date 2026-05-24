// auto_group.go — 自动分组（Auto Group）配置管理
// 职责：管理系统自动生成的分组列表，用于在用户注册或创建资源时
// 自动为其分配默认分组。提供分组的序列化/反序列化和查询功能。

package setting

import (
	// 项目公共工具包，提供 JSON 序列化等工具函数
	"github.com/c1cada/NexusTok/common"
)

// autoGroups 存储所有自动分组的名称列表
var autoGroups = []string{
	"default",
}

// DefaultUseAutoGroup 控制是否默认启用自动分组功能
var DefaultUseAutoGroup = false

// ContainsAutoGroup 判断指定的分组名是否在自动分组列表中
// 参数：
//   - group: 待查询的分组名称
// 返回值：如果分组存在于列表中则返回 true
func ContainsAutoGroup(group string) bool {
	for _, autoGroup := range autoGroups {
		if autoGroup == group {
			return true
		}
	}
	return false
}

// UpdateAutoGroupsByJsonString 从 JSON 字符串解析并更新自动分组列表
// 参数：
//   - jsonString: JSON 格式的分组名数组字符串，如 '["default","vip"]'
// 返回值：解析失败时返回错误
func UpdateAutoGroupsByJsonString(jsonString string) error {
	autoGroups = make([]string, 0)
	return common.Unmarshal([]byte(jsonString), &autoGroups)
}

// AutoGroups2JsonString 将当前自动分组列表序列化为 JSON 字符串
// 返回值：JSON 字符串，序列化失败时返回 "[]"
func AutoGroups2JsonString() string {
	jsonBytes, err := common.Marshal(autoGroups)
	if err != nil {
		return "[]"
	}
	return string(jsonBytes)
}

// GetAutoGroups 获取当前所有自动分组的切片副本
// 返回值：自动分组名称列表
func GetAutoGroups() []string {
	return autoGroups
}
