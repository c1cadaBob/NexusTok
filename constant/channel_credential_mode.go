// Package constant - channel_credential_mode.go
// 该文件定义了渠道凭证模式相关的常量
//
// 渠道凭证模式决定了如何管理和使用渠道的 API 密钥：
// - single_key：单密钥模式，渠道只有一个 API 密钥
// - multi_key：多密钥模式，渠道有多个 API 密钥轮换使用
// - account_pool：账号池模式，从账号池中选择账号
// - global_account_pool：全局账号池模式，使用全局共享的账号池
package constant

// 渠道凭证模式常量
const (
	// ChannelCredentialModeSingleKey 单密钥模式
	// 渠道只配置一个 API 密钥，所有请求使用同一密钥
	ChannelCredentialModeSingleKey = "single_key"

	// ChannelCredentialModeMultiKey 多密钥模式
	// 渠道配置多个 API 密钥，支持轮换和负载均衡
	ChannelCredentialModeMultiKey = "multi_key"

	// ChannelCredentialModeAccountPool 账号池模式
	// 从专用账号池中选择账号进行请求
	ChannelCredentialModeAccountPool = "account_pool"

	// ChannelCredentialModeGlobalAccountPool 全局账号池模式
	// 使用全局共享的账号池，多个渠道可共享同一账号池
	ChannelCredentialModeGlobalAccountPool = "global_account_pool"
)

// 账号池选择模式常量
const (
	// ChannelAccountPoolModePolling 轮询模式
	// 按顺序依次使用账号池中的账号
	ChannelAccountPoolModePolling = "polling"

	// ChannelAccountPoolModeRandom 随机模式
	// 从账号池中随机选择账号
	ChannelAccountPoolModeRandom = "random"

	// ChannelAccountPoolModeFillFirst 优先填充模式
	// 优先使用未满负载的账号
	ChannelAccountPoolModeFillFirst = "fill_first"
)
