// midjourney.go — Midjourney 图像生成服务的配置开关
// 职责：管理 Midjourney 渠道的各项功能开关，包括通知回调、
// 账号筛选、模式清除、URL 转发和动作状态检查等。

package setting

// MjNotifyEnabled 控制是否启用 Midjourney 的异步通知回调功能
var MjNotifyEnabled = false

// MjAccountFilterEnabled 控制是否启用 Midjourney 的账号过滤功能
var MjAccountFilterEnabled = false

// MjModeClearEnabled 控制是否启用 Midjourney 模式清除功能
var MjModeClearEnabled = false

// MjForwardUrlEnabled 控制是否启用 Midjourney 请求 URL 转发功能
var MjForwardUrlEnabled = true

// MjActionCheckSuccessEnabled 控制是否启用 Midjourney 动作执行成功状态检查
var MjActionCheckSuccessEnabled = true
