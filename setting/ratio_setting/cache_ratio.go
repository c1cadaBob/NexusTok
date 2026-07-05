// cache_ratio.go — 缓存读取/创建缓存比率配置
// 职责：管理各 AI 模型的缓存相关比率配置，包括：
//   - 缓存读取比率（Cache Ratio）：命中缓存时的 token 计费折扣率
//   - 创建缓存比率（Create Cache Ratio）：写入缓存时的额外计费倍率
//
// 支持线程安全的并发读写，并在更新时自动刷新暴露数据缓存。

package ratio_setting

import (
	"github.com/c1cada/NexusTok/types"
)

// defaultCacheRatio 缓存读取比率的默认配置
// 值为 1 表示与正常价格相同，0.5 表示正常价格的 50%，0.1 表示 10%
var defaultCacheRatio = map[string]float64{
	"gemini-3-flash-preview":              0.1,
	"gemini-3-pro-preview":                0.1,
	"gemini-3.1-pro-preview":              0.1,
	"gpt-4":                               0.5,
	"o1":                                  0.5,
	"o1-2024-12-17":                       0.5,
	"o1-preview-2024-09-12":               0.5,
	"o1-preview":                          0.5,
	"o1-mini-2024-09-12":                  0.5,
	"o1-mini":                             0.5,
	"o3-mini":                             0.5,
	"o3-mini-2025-01-31":                  0.5,
	"gpt-4o-2024-11-20":                   0.5,
	"gpt-4o-2024-08-06":                   0.5,
	"gpt-4o":                              0.5,
	"gpt-4o-mini-2024-07-18":              0.5,
	"gpt-4o-mini":                         0.5,
	"gpt-4o-realtime-preview":             0.5,
	"gpt-4o-mini-realtime-preview":        0.5,
	"gpt-4.5-preview":                     0.5,
	"gpt-4.5-preview-2025-02-27":          0.5,
	"gpt-4.1":                             0.25,
	"gpt-4.1-mini":                        0.25,
	"gpt-4.1-nano":                        0.25,
	"gpt-5":                               0.1,
	"gpt-5-2025-08-07":                    0.1,
	"gpt-5-chat-latest":                   0.1,
	"gpt-5-mini":                          0.1,
	"gpt-5-mini-2025-08-07":               0.1,
	"gpt-5-nano":                          0.1,
	"gpt-5-nano-2025-08-07":               0.1,
	"deepseek-chat":                       0.25,
	"deepseek-reasoner":                   0.25,
	"deepseek-coder":                      0.25,
	"claude-3-sonnet-20240229":            0.1,
	"claude-3-opus-20240229":              0.1,
	"claude-3-haiku-20240307":             0.1,
	"claude-3-5-haiku-20241022":           0.1,
	"claude-haiku-4-5-20251001":           0.1,
	"claude-3-5-sonnet-20240620":          0.1,
	"claude-3-5-sonnet-20241022":          0.1,
	"claude-3-7-sonnet-20250219":          0.1,
	"claude-3-7-sonnet-20250219-thinking": 0.1,
	"claude-sonnet-4-20250514":            0.1,
	"claude-sonnet-4-20250514-thinking":   0.1,
	"claude-opus-4-20250514":              0.1,
	"claude-opus-4-20250514-thinking":     0.1,
	"claude-opus-4-1-20250805":            0.1,
	"claude-opus-4-1-20250805-thinking":   0.1,
	"claude-sonnet-4-5-20250929":          0.1,
	"claude-sonnet-4-5-20250929-thinking": 0.1,
	"claude-opus-4-5-20251101":            0.1,
	"claude-opus-4-5-20251101-thinking":   0.1,
	"claude-opus-4-6":                     0.1,
	"claude-opus-4-6-thinking":            0.1,
	"claude-opus-4-6-max":                 0.1,
	"claude-opus-4-6-high":                0.1,
	"claude-opus-4-6-medium":              0.1,
	"claude-opus-4-6-low":                 0.1,
	"claude-opus-4-7":                     0.1,
	"claude-opus-4-7-thinking":            0.1,
	"claude-opus-4-7-max":                 0.1,
	"claude-opus-4-7-xhigh":               0.1,
	"claude-opus-4-7-high":                0.1,
	"claude-opus-4-7-medium":              0.1,
	"claude-opus-4-7-low":                 0.1,
}

// defaultCreateCacheRatio 创建缓存（写入缓存）比率的默认配置
// 值为 1.25 表示写入缓存的 token 价格为正常价格的 125%
var defaultCreateCacheRatio = map[string]float64{
	"claude-3-sonnet-20240229":            1.25,
	"claude-3-opus-20240229":              1.25,
	"claude-3-haiku-20240307":             1.25,
	"claude-3-5-haiku-20241022":           1.25,
	"claude-haiku-4-5-20251001":           1.25,
	"claude-3-5-sonnet-20240620":          1.25,
	"claude-3-5-sonnet-20241022":          1.25,
	"claude-3-7-sonnet-20250219":          1.25,
	"claude-3-7-sonnet-20250219-thinking": 1.25,
	"claude-sonnet-4-20250514":            1.25,
	"claude-sonnet-4-20250514-thinking":   1.25,
	"claude-opus-4-20250514":              1.25,
	"claude-opus-4-20250514-thinking":     1.25,
	"claude-opus-4-1-20250805":            1.25,
	"claude-opus-4-1-20250805-thinking":   1.25,
	"claude-sonnet-4-5-20250929":          1.25,
	"claude-sonnet-4-5-20250929-thinking": 1.25,
	"claude-opus-4-5-20251101":            1.25,
	"claude-opus-4-5-20251101-thinking":   1.25,
	"claude-opus-4-6":                     1.25,
	"claude-opus-4-6-thinking":            1.25,
	"claude-opus-4-6-max":                 1.25,
	"claude-opus-4-6-high":                1.25,
	"claude-opus-4-6-medium":              1.25,
	"claude-opus-4-6-low":                 1.25,
	"claude-opus-4-7":                     1.25,
	"claude-opus-4-7-thinking":            1.25,
	"claude-opus-4-7-max":                 1.25,
	"claude-opus-4-7-xhigh":               1.25,
	"claude-opus-4-7-high":                1.25,
	"claude-opus-4-7-medium":              1.25,
	"claude-opus-4-7-low":                 1.25,
}

//var defaultCreateCacheRatio = map[string]float64{}

// cacheRatioMap 线程安全的缓存读取比率 Map
var cacheRatioMap = types.NewRWMap[string, float64]()

// createCacheRatioMap 线程安全的创建缓存比率 Map
var createCacheRatioMap = types.NewRWMap[string, float64]()

// GetCacheRatioMap 返回缓存读取比率 Map 的副本
// 返回值：模型名到缓存比率的映射副本
func GetCacheRatioMap() map[string]float64 {
	return cacheRatioMap.ReadAll()
}

// CacheRatio2JSONString 将缓存读取比率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func CacheRatio2JSONString() string {
	return cacheRatioMap.MarshalJSONString()
}

// CreateCacheRatio2JSONString 将创建缓存比率 Map 序列化为 JSON 字符串
// 返回值：JSON 字符串
func CreateCacheRatio2JSONString() string {
	return createCacheRatioMap.MarshalJSONString()
}

// GetDefaultCacheRatioMap 返回内置缓存读取倍率配置的副本。
// 该函数用于区分“系统默认值”和“管理员持久化覆盖值”，避免上游价格同步
// 把内置默认价格误判为手动确认价。
func GetDefaultCacheRatioMap() map[string]float64 {
	return copyFloatMap(defaultCacheRatio)
}

// GetDefaultCreateCacheRatioMap 返回内置缓存写入倍率配置的副本。
// 调用方只能读取副本，不能修改包内默认值。
func GetDefaultCreateCacheRatioMap() map[string]float64 {
	return copyFloatMap(defaultCreateCacheRatio)
}

// UpdateCacheRatioByJSONString 从 JSON 字符串更新缓存读取比率配置
// 更新后会自动刷新暴露数据缓存
// 参数：
//   - jsonStr: JSON 格式的比率配置字符串
//
// 返回值：解析失败时返回错误
func UpdateCacheRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(cacheRatioMap, jsonStr, InvalidateExposedDataCache)
}

// UpdateCreateCacheRatioByJSONString 从 JSON 字符串更新创建缓存比率配置
// 更新后会自动刷新暴露数据缓存
// 参数：
//   - jsonStr: JSON 格式的比率配置字符串
//
// 返回值：解析失败时返回错误
func UpdateCreateCacheRatioByJSONString(jsonStr string) error {
	return types.LoadFromJsonStringWithCallback(createCacheRatioMap, jsonStr, InvalidateExposedDataCache)
}

// GetCacheRatio 获取指定模型的缓存读取比率
// 参数：
//   - name: 模型名称
//
// 返回值：
//   - float64: 缓存比率，未找到时默认返回 1
//   - bool: 是否在配置中找到了该模型
func GetCacheRatio(name string) (float64, bool) {
	ratio, ok := cacheRatioMap.Get(name)
	if !ok {
		return 1, false // 默认为 1（与正常价格相同）
	}
	return ratio, true
}

// GetCreateCacheRatio 获取指定模型的创建缓存比率
// 参数：
//   - name: 模型名称
//
// 返回值：
//   - float64: 创建缓存比率，未找到时默认返回 1.25
//   - bool: 是否在配置中找到了该模型
func GetCreateCacheRatio(name string) (float64, bool) {
	ratio, ok := createCacheRatioMap.Get(name)
	if !ok {
		return 1.25, false // 默认为 1.25（写入缓存额外收费 25%）
	}
	return ratio, true
}

// GetCacheRatioCopy 获取缓存读取比率 Map 的副本
// 返回值：模型名到缓存比率的映射副本
func GetCacheRatioCopy() map[string]float64 {
	return cacheRatioMap.ReadAll()
}

// GetCreateCacheRatioCopy 获取创建缓存比率 Map 的副本
// 返回值：模型名到创建缓存比率的映射副本
func GetCreateCacheRatioCopy() map[string]float64 {
	return createCacheRatioMap.ReadAll()
}
