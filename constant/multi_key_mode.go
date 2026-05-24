// Package constant - multi_key_mode.go
// 该文件定义了多密钥模式相关的常量
//
// 多密钥模式决定了当渠道配置了多个 API 密钥时，如何选择使用哪个密钥
package constant

// MultiKeyMode 多密钥选择模式类型
type MultiKeyMode string

const (
	// MultiKeyModeRandom 随机模式
	// 从可用密钥中随机选择一个
	MultiKeyModeRandom MultiKeyMode = "random"

	// MultiKeyModePolling 轮询模式
	// 按顺序依次使用每个密钥
	MultiKeyModePolling MultiKeyMode = "polling"
)
