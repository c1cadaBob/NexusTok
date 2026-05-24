// Package setting - user_usable_group.go
// 该文件管理用户可用分组的配置
//
// 功能：
// - 维护用户可用的分组列表（分组名称 -> 分组描述）
// - 提供线程安全的分组查询和更新操作
// - 支持 JSON 序列化/反序列化用于数据库存储
package setting

import (
	"encoding/json"
	"sync"

	"github.com/c1cada/NexusTok/common"
)

// userUsableGroups 用户可用分组映射（分组名称 -> 分组描述）
var userUsableGroups = map[string]string{
	"default": "默认分组",
	"vip":     "vip分组",
}

// userUsableGroupsMutex 保护 userUsableGroups 的读写锁
var userUsableGroupsMutex sync.RWMutex

// GetUserUsableGroupsCopy 获取用户可用分组的副本
// 返回副本而非引用，确保外部修改不影响内部状态
//
// 返回值：
//   - map[string]string: 分组名称到描述的映射副本
func GetUserUsableGroupsCopy() map[string]string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	copyUserUsableGroups := make(map[string]string)
	for k, v := range userUsableGroups {
		copyUserUsableGroups[k] = v
	}
	return copyUserUsableGroups
}

// UserUsableGroups2JSONString 将用户可用分组序列化为 JSON 字符串
//
// 返回值：
//   - string: JSON 格式的分组配置
func UserUsableGroups2JSONString() string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	jsonBytes, err := json.Marshal(userUsableGroups)
	if err != nil {
		common.SysLog("error marshalling user groups: " + err.Error())
	}
	return string(jsonBytes)
}

// UpdateUserUsableGroupsByJSONString 从 JSON 字符串更新用户可用分组
//
// 参数：
//   - jsonStr: JSON 格式的分组配置
//
// 返回值：
//   - error: 解析错误
func UpdateUserUsableGroupsByJSONString(jsonStr string) error {
	userUsableGroupsMutex.Lock()
	defer userUsableGroupsMutex.Unlock()

	userUsableGroups = make(map[string]string)
	return json.Unmarshal([]byte(jsonStr), &userUsableGroups)
}

// GetUsableGroupDescription 获取分组的描述信息
// 如果分组不存在，返回分组名称本身
//
// 参数：
//   - groupName: 分组名称
//
// 返回值：
//   - string: 分组描述
func GetUsableGroupDescription(groupName string) string {
	userUsableGroupsMutex.RLock()
	defer userUsableGroupsMutex.RUnlock()

	if desc, ok := userUsableGroups[groupName]; ok {
		return desc
	}
	return groupName
}
