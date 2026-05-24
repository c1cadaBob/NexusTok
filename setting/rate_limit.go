// rate_limit.go — 模型请求频率限制配置
// 职责：管理基于分组的模型请求频率限制（Rate Limit）功能。
// 支持按分组设定总请求次数限制和成功请求次数限制，
// 并提供线程安全的读写操作和数据校验功能。

package setting

import (
	"encoding/json"
	"fmt"
	"math"
	"sync"

	"github.com/c1cada/NexusTok/common"
)

// --- 全局频率限制配置变量 ---

// ModelRequestRateLimitEnabled 控制是否启用模型请求频率限制
var ModelRequestRateLimitEnabled = false

// ModelRequestRateLimitDurationMinutes 频率限制的统计时间窗口（单位：分钟）
var ModelRequestRateLimitDurationMinutes = 1

// ModelRequestRateLimitCount 全局总请求次数限制（0 表示不限制）
var ModelRequestRateLimitCount = 0

// ModelRequestRateLimitSuccessCount 全局成功请求次数限制
var ModelRequestRateLimitSuccessCount = 1000

// ModelRequestRateLimitGroup 按分组设定的频率限制配置
// 键为分组名，值为 [2]int 数组，含义为 [总请求限制, 成功请求限制]
var ModelRequestRateLimitGroup = map[string][2]int{}

// ModelRequestRateLimitMutex 保护频率限制配置的读写互斥锁
var ModelRequestRateLimitMutex sync.RWMutex

// ModelRequestRateLimitGroup2JSONString 将分组频率限制配置序列化为 JSON 字符串
// 使用读锁保证并发安全
// 返回值：JSON 格式的配置字符串
func ModelRequestRateLimitGroup2JSONString() string {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	jsonBytes, err := json.Marshal(ModelRequestRateLimitGroup)
	if err != nil {
		common.SysLog("error marshalling model ratio: " + err.Error())
	}
	return string(jsonBytes)
}

// UpdateModelRequestRateLimitGroupByJSONString 从 JSON 字符串解析并更新分组频率限制配置
// 参数：
//   - jsonStr: JSON 格式的配置字符串
//
// 返回值：解析失败时返回错误
func UpdateModelRequestRateLimitGroupByJSONString(jsonStr string) error {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	ModelRequestRateLimitGroup = make(map[string][2]int)
	return json.Unmarshal([]byte(jsonStr), &ModelRequestRateLimitGroup)
}

// GetGroupRateLimit 获取指定分组的频率限制配置
// 参数：
//   - group: 分组名称
//
// 返回值：
//   - totalCount: 该分组的总请求限制
//   - successCount: 该分组的成功请求限制
//   - found: 是否找到该分组的配置
func GetGroupRateLimit(group string) (totalCount, successCount int, found bool) {
	ModelRequestRateLimitMutex.RLock()
	defer ModelRequestRateLimitMutex.RUnlock()

	if ModelRequestRateLimitGroup == nil {
		return 0, 0, false
	}

	limits, found := ModelRequestRateLimitGroup[group]
	if !found {
		return 0, 0, false
	}
	return limits[0], limits[1], true
}

// CheckModelRequestRateLimitGroup 校验分组频率限制配置的合法性
// 检查规则：
//   - 总请求限制不允许为负数
//   - 成功请求限制不允许小于 1
//   - 所有值不允许超过 int32 最大值
//
// 参数：
//   - jsonStr: JSON 格式的配置字符串
//
// 返回值：校验失败时返回错误
func CheckModelRequestRateLimitGroup(jsonStr string) error {
	checkModelRequestRateLimitGroup := make(map[string][2]int)
	err := json.Unmarshal([]byte(jsonStr), &checkModelRequestRateLimitGroup)
	if err != nil {
		return err
	}
	for group, limits := range checkModelRequestRateLimitGroup {
		if limits[0] < 0 || limits[1] < 1 {
			return fmt.Errorf("group %s has negative rate limit values: [%d, %d]", group, limits[0], limits[1])
		}
		if limits[0] > math.MaxInt32 || limits[1] > math.MaxInt32 {
			return fmt.Errorf("group %s [%d, %d] has max rate limits value 2147483647", group, limits[0], limits[1])
		}
	}

	return nil
}
