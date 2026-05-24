// Package types - price_data.go
// 该文件定义了价格数据相关类型
//
// 主要类型：
// - GroupRatioInfo：分组倍率信息
//
// 用途：
// - 在计费计算中传递分组倍率
// - 支持特殊倍率和默认倍率
package types

import "fmt"

// GroupRatioInfo 分组倍率信息
// 存储用户组的计费倍率配置
type GroupRatioInfo struct {
	GroupRatio        float64 // 用户组默认倍率
	GroupSpecialRatio float64 // 用户组特殊倍率（如特定模型的优惠倍率）
	HasSpecialRatio   bool    // 是否有特殊倍率配置
}

// PriceData 价格数据结构
// 包含模型计费所需的所有价格和倍率信息
// 用于计算 API 调用的实际费用
type PriceData struct {
	FreeModel            bool               // 是否为免费模型
	ModelPrice           float64            // 模型基础价格（按次计费时使用）
	ModelRatio           float64            // 模型倍率（相对基准模型的倍数）
	CompletionRatio      float64            // 补全 Token 倍率（相对于提示 Token 的倍数）
	CacheRatio           float64            // 缓存命中倍率（缓存 Token 的折扣倍数）
	CacheCreationRatio   float64            // 缓存创建倍率（创建缓存的额外费用倍数）
	CacheCreation5mRatio float64            // 5 分钟缓存创建倍率
	CacheCreation1hRatio float64            // 1 小时缓存创建倍率
	ImageRatio           float64            // 图片倍率
	AudioRatio           float64            // 音频倍率
	AudioCompletionRatio float64            // 音频补全倍率
	OtherRatios          map[string]float64 // 其他自定义倍率（如视频、文件等）
	UsePrice             bool               // 是否使用固定价格计费（而非按 Token 计费）
	Quota                int                // 按次计费的最终额度（如 Midjourney、Task 类型）
	QuotaToPreConsume    int                // 按量计费的预消耗额度
	GroupRatioInfo       GroupRatioInfo     // 用户组倍率信息
}

// AddOtherRatio 添加自定义倍率
// 用于添加非标准的计费倍率（如视频生成、特殊处理等）
//
// 参数：
//   - key: 倍率键名（如 "video_ratio"、"file_ratio"）
//   - ratio: 倍率值，必须大于 0
func (p *PriceData) AddOtherRatio(key string, ratio float64) {
	if p.OtherRatios == nil {
		p.OtherRatios = make(map[string]float64)
	}
	if ratio <= 0 {
		return
	}
	p.OtherRatios[key] = ratio
}

// ToSetting 将价格数据转换为设置字符串
// 用于日志记录和调试，输出所有价格和倍率的格式化字符串
//
// 返回值：
//   - string: 格式化的价格设置字符串
func (p *PriceData) ToSetting() string {
	return fmt.Sprintf("ModelPrice: %f, ModelRatio: %f, CompletionRatio: %f, CacheRatio: %f, GroupRatio: %f, UsePrice: %t, CacheCreationRatio: %f, CacheCreation5mRatio: %f, CacheCreation1hRatio: %f, QuotaToPreConsume: %d, ImageRatio: %f, AudioRatio: %f, AudioCompletionRatio: %f", p.ModelPrice, p.ModelRatio, p.CompletionRatio, p.CacheRatio, p.GroupRatioInfo.GroupRatio, p.UsePrice, p.CacheCreationRatio, p.CacheCreation5mRatio, p.CacheCreation1hRatio, p.QuotaToPreConsume, p.ImageRatio, p.AudioRatio, p.AudioCompletionRatio)
}
