// Package common - topup-ratio.go
// 该文件实现了充值分组倍率管理功能
//
// 充值分组倍率用于不同用户组的充值金额换算：
// - default：默认用户组倍率（默认 1）
// - vip：VIP 用户组倍率（默认 1）
// - svip：SVIP 用户组倍率（默认 1）
//
// 倍率含义：实际到账金额 = 充值金额 × 倍率
// 例如：VIP 用户充值 100 元，倍率为 1.2，则实际到账 120 额度
//
// 线程安全：
// - 使用 sync.RWMutex 保护倍率数据的并发读写
// - 读操作（Get/ToJSON）使用读锁，写操作（Update）使用写锁
package common

import (
	"encoding/json"
	"sync"
)

// topupGroupRatio 存储各用户组的充值倍率
//
// 键：用户组名称（default, vip, svip 等）
// 值：充值倍率（浮点数，默认为 1）
var topupGroupRatio = map[string]float64{
	"default": 1,
	"vip":     1,
	"svip":    1,
}

// topupGroupRatioMutex 保护 topupGroupRatio 的并发读写
var topupGroupRatioMutex sync.RWMutex

// TopupGroupRatio2JSONString 将充值分组倍率序列化为 JSON 字符串
//
// 用于配置持久化和 API 响应
//
// 返回值：
//   - string: JSON 格式的倍率配置
func TopupGroupRatio2JSONString() string {
	topupGroupRatioMutex.RLock()
	defer topupGroupRatioMutex.RUnlock()
	jsonBytes, err := json.Marshal(topupGroupRatio)
	if err != nil {
		SysError("error marshalling topup group ratio: " + err.Error())
	}
	return string(jsonBytes)
}

// UpdateTopupGroupRatioByJSONString 从 JSON 字符串更新充值分组倍率
//
// 用于从数据库或配置文件加载倍率配置
// 注意：此操作会完全替换现有配置
//
// 参数：
//   - jsonStr: JSON 格式的倍率配置
//
// 返回值：
//   - error: JSON 解析错误
func UpdateTopupGroupRatioByJSONString(jsonStr string) error {
	topupGroupRatioMutex.Lock()
	defer topupGroupRatioMutex.Unlock()
	topupGroupRatio = make(map[string]float64)
	return json.Unmarshal([]byte(jsonStr), &topupGroupRatio)
}

// GetTopupGroupRatio 获取指定用户组的充值倍率
//
// 如果用户组不存在，记录错误日志并返回默认倍率 1
//
// 参数：
//   - name: 用户组名称
//
// 返回值：
//   - float64: 充值倍率
func GetTopupGroupRatio(name string) float64 {
	topupGroupRatioMutex.RLock()
	defer topupGroupRatioMutex.RUnlock()
	ratio, ok := topupGroupRatio[name]
	if !ok {
		SysError("topup group ratio not found: " + name)
		return 1
	}
	return ratio
}
