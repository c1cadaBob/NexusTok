// expose_ratio.go — 比率暴露开关管理
// 职责：控制是否将模型定价比率数据暴露给前端 API。
// 使用 atomic.Bool 实现线程安全的开关状态读写。

package ratio_setting

import "sync/atomic"

// exposeRatioEnabled 原子布尔值，控制比率数据是否对外暴露
var exposeRatioEnabled atomic.Bool

// init 初始化时默认关闭比率暴露
func init() {
	exposeRatioEnabled.Store(false)
}

// SetExposeRatioEnabled 设置比率暴露开关状态
// 参数：
//   - enabled: true 表示启用暴露，false 表示禁用
func SetExposeRatioEnabled(enabled bool) {
	exposeRatioEnabled.Store(enabled)
}

// IsExposeRatioEnabled 获取当前比率暴露开关状态
// 返回值：true 表示已启用，false 表示已禁用
func IsExposeRatioEnabled() bool {
	return exposeRatioEnabled.Load()
}
