// Package model - pricing_refresh.go
// 该文件提供了定价缓存的强制刷新功能
//
// 核心功能：
// - RefreshPricing：强制立即重新计算与定价相关的缓存
// - 绕过默认的 1 分钟延迟刷新
// - 用于需要最新数据的内部管理 API
package model

// RefreshPricing 强制立即重新计算与定价相关的缓存。
// 该方法用于需要最新数据的内部管理 API，
// 因此会绕过默认的 1 分钟延迟刷新。
func RefreshPricing() {
	updatePricingLock.Lock()
	defer updatePricingLock.Unlock()

	modelSupportEndpointsLock.Lock()
	defer modelSupportEndpointsLock.Unlock()

	updatePricing()
}
