// Package vertex 实现 Google Vertex AI 渠道的模型区域解析
package vertex

import "github.com/c1cada/NexusTok/common" // 公共工具包

// GetModelRegion 获取模型对应的区域配置
// 支持 JSON 格式的区域映射配置，可以为不同模型指定不同区域
//
// 参数：
//   - other: 区域配置字符串（可以是普通字符串或 JSON 格式）
//   - localModelName: 本地模型名称
//
// 返回值：
//   - string: 模型对应的区域名称
func GetModelRegion(other string, localModelName string) string {
	// 如果是 JSON 字符串，解析区域映射
	if common.IsJsonObject(other) {
		m, err := common.StrToMap(other)
		if err != nil {
			return other // 解析失败时返回原始值
		}
		if m[localModelName] != nil {
			return m[localModelName].(string)
		} else {
			if v, ok := m["default"]; ok {
				return v.(string)
			}
			return "global"
		}
	}
	return other
}
