// exposed_cache.go — 暴露数据缓存管理
// 职责：为前端 API 提供定价数据（模型比率、完成比率、缓存比率、模型价格等）
// 的带 TTL 缓存层。使用 atomic.Value 实现无锁读取，双检锁（Double-Check Locking）
// 防止并发重建，并在返回时克隆数据避免外部修改影响缓存。

package ratio_setting

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// exposedDataTTL 暴露数据缓存的过期时间，30 秒
const exposedDataTTL = 30 * time.Second

// exposedCache 缓存条目结构，包含数据和过期时间
type exposedCache struct {
	data      gin.H     // 缓存的定价数据
	expiresAt time.Time // 缓存过期时间
}

var (
	// exposedData 原子存储的缓存条目指针
	exposedData atomic.Value
	// rebuildMu 重建缓存的互斥锁
	rebuildMu sync.Mutex
)

// InvalidateExposedDataCache 使暴露数据缓存立即失效
// 在定价配置更新时调用，触发下次读取时重建缓存
func InvalidateExposedDataCache() {
	exposedData.Store((*exposedCache)(nil))
}

// cloneGinH 浅拷贝 gin.H Map，避免外部修改影响缓存中的数据
// 参数：
//   - src: 源 Map
//
// 返回值：拷贝后的新 Map
func cloneGinH(src gin.H) gin.H {
	dst := make(gin.H, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// GetExposedData 获取暴露给前端的定价数据
// 使用双检锁模式：先无锁检查缓存是否有效，无效则加锁重建
// 返回值：包含各项定价数据的 gin.H Map 副本
func GetExposedData() gin.H {
	// 第一次无锁检查：缓存有效则直接返回克隆数据
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	// 缓存无效，加锁重建
	rebuildMu.Lock()
	defer rebuildMu.Unlock()
	// 第二次检查（double-check）：防止多个 goroutine 同时重建
	if c, ok := exposedData.Load().(*exposedCache); ok && c != nil && time.Now().Before(c.expiresAt) {
		return cloneGinH(c.data)
	}
	// 收集所有定价数据
	newData := gin.H{
		"model_ratio":        GetModelRatioCopy(),
		"completion_ratio":   GetCompletionRatioCopy(),
		"cache_ratio":        GetCacheRatioCopy(),
		"create_cache_ratio": GetCreateCacheRatioCopy(),
		"model_price":        GetModelPriceCopy(),
	}
	// 存储新缓存并设置过期时间
	exposedData.Store(&exposedCache{
		data:      newData,
		expiresAt: time.Now().Add(exposedDataTTL),
	})
	return cloneGinH(newData)
}
