package common

import "github.com/c1cada/NexusTok/constant"

// GetAnonymousRequestBodyLimitBytes 返回匿名入口请求体限制的字节数。
//
// 该限制面向未认证的 POST 接口，例如登录、注册、密码重置和支付 Webhook。
// 这些接口本身不需要承载大文件，因此默认 512KB 足以覆盖正常 JSON/Form/Webhook
// 负载，同时能在进入 controller 前拦截异常大请求。配置值小于等于 0 时表示禁用，
// 便于特殊部署在上游网关已经完成限制时主动关闭本层保护。
func GetAnonymousRequestBodyLimitBytes() int64 {
	limitKB := constant.AnonymousRequestBodyLimitKB
	if limitKB <= 0 {
		return 0
	}
	return int64(limitKB) << 10
}
