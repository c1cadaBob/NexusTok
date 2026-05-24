// Package model - model_extra.go
// 该文件提供了模型的扩展查询功能
//
// 核心功能：
// - GetModelEnableGroups：获取指定模型启用的分组列表
// - GetModelQuotaTypes：获取指定模型的计费类型集合
//
// 数据来源：
// - 从定价缓存（pricing cache）中读取
package model

// GetModelEnableGroups 获取指定模型启用的分组列表
// 从定价缓存中读取数据
//
// 参数：
//   - modelName: 模型名称
//
// 返回值：
//   - []string: 启用的分组列表
func GetModelEnableGroups(modelName string) []string {
	// 确保缓存最新
	GetPricing()

	if modelName == "" {
		return make([]string, 0)
	}

	modelEnableGroupsLock.RLock()
	groups, ok := modelEnableGroups[modelName]
	modelEnableGroupsLock.RUnlock()
	if !ok {
		return make([]string, 0)
	}
	return groups
}

// GetModelQuotaTypes 返回指定模型的计费类型集合（来自缓存）
func GetModelQuotaTypes(modelName string) []int {
	GetPricing()

	modelEnableGroupsLock.RLock()
	quota, ok := modelQuotaTypeMap[modelName]
	modelEnableGroupsLock.RUnlock()
	if !ok {
		return []int{}
	}
	return []int{quota}
}
