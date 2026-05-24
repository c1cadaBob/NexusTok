// Package types - channel_error.go
// 该文件定义了渠道错误（ChannelError）数据结构
//
// 主要结构体：
// - ChannelError：渠道错误信息，包含渠道详情和错误上下文
//
// 用途：
// - 记录渠道请求失败时的详细信息
// - 用于日志记录和错误分析
// - 支持多 Key 模式和账号池模式的错误追踪
package types

// ChannelError 渠道错误信息
// 记录渠道请求失败时的详细上下文信息
// 用于日志记录、错误分析和问题排查
// 支持多 Key 模式和账号池模式的错误追踪
type ChannelError struct {
	ChannelId           int    `json:"channel_id"`                     // 渠道 ID
	ChannelType         int    `json:"channel_type"`                   // 渠道类型（对应 constant 中的渠道类型常量）
	ChannelName         string `json:"channel_name"`                   // 渠道名称
	IsMultiKey          bool   `json:"is_multi_key"`                   // 是否为多密钥模式
	AutoBan             bool   `json:"auto_ban"`                       // 是否启用自动禁用
	UsingKey            string `json:"using_key"`                      // 当前使用的 API 密钥（已脱敏）
	CredentialMode      string `json:"credential_mode,omitempty"`      // 凭证模式（single_key/multi_key/account_pool 等）
	AccountPool         bool   `json:"account_pool,omitempty"`         // 是否使用账号池
	ChannelAccountId    int    `json:"channel_account_id,omitempty"`   // 渠道账号 ID
	ChannelAccountName  string `json:"channel_account_name,omitempty"` // 渠道账号名称
	PoolGroupId         int    `json:"pool_group_id,omitempty"`        // 账号池分组 ID
	PoolGroupName       string `json:"pool_group_name,omitempty"`      // 账号池分组名称
	PoolAccountId       int    `json:"pool_account_id,omitempty"`      // 账号池账号 ID
	PoolAccountName     string `json:"pool_account_name,omitempty"`    // 账号池账号名称
	PoolAccountAuthType string `json:"pool_account_auth_type,omitempty"` // 账号池账号认证类型
}

// NewChannelError 创建新的渠道错误
//
// 参数：
//   - channelId: 渠道 ID
//   - channelType: 渠道类型
//   - channelName: 渠道名称
//   - isMultiKey: 是否为多密钥模式
//   - usingKey: 当前使用的 API 密钥
//   - autoBan: 是否启用自动禁用
//
// 返回值：
//   - *ChannelError: 渠道错误对象
func NewChannelError(channelId int, channelType int, channelName string, isMultiKey bool, usingKey string, autoBan bool) *ChannelError {
	return &ChannelError{
		ChannelId:   channelId,
		ChannelType: channelType,
		ChannelName: channelName,
		IsMultiKey:  isMultiKey,
		AutoBan:     autoBan,
		UsingKey:    usingKey,
	}
}
