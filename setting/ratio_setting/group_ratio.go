// group_ratio.go — 用户分组比率配置管理
// 职责：管理用户分组的计费比率配置，支持三层分组体系：
//   - GroupRatio：用户所属分组的基础比率（如 vip 用户享受折扣）
//   - GroupGroupRatio：用户分组对特定使用分组的交叉比率（如 vip 使用 edit_this 分组的折扣）
//   - GroupSpecialUsableGroup：用户分组的特殊可用分组配置（支持追加/移除操作）
//
// 通过 config.GlobalConfig 注册实现持久化存储。

package ratio_setting

import (
	"encoding/json"
	"errors"

	"github.com/c1cada/NexusTok/common"
	"github.com/c1cada/NexusTok/setting/config"
	"github.com/c1cada/NexusTok/types"
)

// defaultGroupRatio 默认的分组比率配置
// 值为 1 表示无折扣/加价
var defaultGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

// groupRatioMap 线程安全的分组比率 Map
var groupRatioMap = types.NewRWMap[string, float64]()

// defaultGroupGroupRatio 默认的分组交叉比率配置
// 键为用户分组，值为该分组对各使用分组的比率
var defaultGroupGroupRatio = map[string]map[string]float64{
	"vip": {
		"edit_this": 0.9, // vip 用户使用 edit_this 分组享受 9 折
	},
}

// groupGroupRatioMap 线程安全的分组交叉比率 Map
var groupGroupRatioMap = types.NewRWMap[string, map[string]float64]()

// defaultGroupSpecialUsableGroup 默认的分组特殊可用分组配置
// 键格式：
//   - "append_1"：追加名为 "vip_special_group_1" 的可用分组
//   - "-:remove_1"：移除名为 "vip_removed_group_1" 的可用分组
var defaultGroupSpecialUsableGroup = map[string]map[string]string{
	"vip": {
		"append_1":   "vip_special_group_1",
		"-:remove_1": "vip_removed_group_1",
	},
}

// GroupRatioSetting 分组比率配置的聚合结构体
type GroupRatioSetting struct {
	// GroupRatio 用户分组的基础比率
	GroupRatio *types.RWMap[string, float64] `json:"group_ratio"`
	// GroupGroupRatio 用户分组对使用分组的交叉比率
	GroupGroupRatio *types.RWMap[string, map[string]float64] `json:"group_group_ratio"`
	// GroupSpecialUsableGroup 用户分组的特殊可用分组配置
	GroupSpecialUsableGroup *types.RWMap[string, map[string]string] `json:"group_special_usable_group"`
}

// groupRatioSetting 是全局分组比率配置实例
var groupRatioSetting GroupRatioSetting

// init 初始化分组比率配置并注册到全局配置管理系统
func init() {
	groupSpecialUsableGroup := types.NewRWMap[string, map[string]string]()
	groupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)

	groupRatioMap.AddAll(defaultGroupRatio)
	groupGroupRatioMap.AddAll(defaultGroupGroupRatio)

	groupRatioSetting = GroupRatioSetting{
		GroupSpecialUsableGroup: groupSpecialUsableGroup,
		GroupRatio:              groupRatioMap,
		GroupGroupRatio:         groupGroupRatioMap,
	}

	config.GlobalConfig.Register("group_ratio_setting", &groupRatioSetting)
}

// GetGroupRatioSetting 获取当前分组比率配置的指针
// 如果特殊可用分组为空则初始化默认值
// 返回值：指向当前配置的指针
func GetGroupRatioSetting() *GroupRatioSetting {
	if groupRatioSetting.GroupSpecialUsableGroup == nil {
		groupRatioSetting.GroupSpecialUsableGroup = types.NewRWMap[string, map[string]string]()
		groupRatioSetting.GroupSpecialUsableGroup.AddAll(defaultGroupSpecialUsableGroup)
	}
	return &groupRatioSetting
}

// GetGroupRatioCopy 获取所有分组比率的副本
// 返回值：分组名到比率的映射副本
func GetGroupRatioCopy() map[string]float64 {
	return groupRatioMap.ReadAll()
}

// ContainsGroupRatio 判断指定分组是否在比率配置中
// 参数：
//   - name: 分组名称
//
// 返回值：存在则返回 true
func ContainsGroupRatio(name string) bool {
	_, ok := groupRatioMap.Get(name)
	return ok
}

// GroupRatio2JSONString 将分组比率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func GroupRatio2JSONString() string {
	return groupRatioMap.MarshalJSONString()
}

// UpdateGroupRatioByJSONString 从 JSON 字符串更新分组比率配置
// 参数：
//   - jsonStr: JSON 格式的分组比率配置
//
// 返回值：解析失败时返回错误
func UpdateGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupRatioMap, jsonStr)
}

// GetGroupRatio 获取指定分组的比率
// 参数：
//   - name: 分组名称
//
// 返回值：分组比率，未找到时记录日志并返回默认值 1
func GetGroupRatio(name string) float64 {
	ratio, ok := groupRatioMap.Get(name)
	if !ok {
		common.SysLog("group ratio not found: " + name)
		return 1
	}
	return ratio
}

// GetGroupGroupRatio 获取用户分组对特定使用分组的交叉比率
// 参数：
//   - userGroup: 用户所属分组
//   - usingGroup: 当前使用的分组
//
// 返回值：
//   - float64: 交叉比率
//   - bool: 是否找到对应的配置
func GetGroupGroupRatio(userGroup, usingGroup string) (float64, bool) {
	gp, ok := groupGroupRatioMap.Get(userGroup)
	if !ok {
		return -1, false
	}
	ratio, ok := gp[usingGroup]
	if !ok {
		return -1, false
	}
	return ratio, true
}

// GroupGroupRatio2JSONString 将分组交叉比率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func GroupGroupRatio2JSONString() string {
	return groupGroupRatioMap.MarshalJSONString()
}

// UpdateGroupGroupRatioByJSONString 从 JSON 字符串更新分组交叉比率配置
// 参数：
//   - jsonStr: JSON 格式的分组交叉比率配置
//
// 返回值：解析失败时返回错误
func UpdateGroupGroupRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonString(groupGroupRatioMap, jsonStr)
}

// CheckGroupRatio 校验分组比率配置的合法性
// 规则：所有分组比率不允许为负数
// 参数：
//   - jsonStr: JSON 格式的分组比率配置
//
// 返回值：校验失败时返回错误
func CheckGroupRatio(jsonStr string) error {
	checkGroupRatio := make(map[string]float64)
	err := json.Unmarshal([]byte(jsonStr), &checkGroupRatio)
	if err != nil {
		return err
	}
	for name, ratio := range checkGroupRatio {
		if ratio < 0 {
			return errors.New("group ratio must be not less than 0: " + name)
		}
	}
	return nil
}
