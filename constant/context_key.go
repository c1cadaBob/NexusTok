// Package constant - context_key.go
// 该文件定义了 Gin Context 中使用的键常量
//
// 上下文键用于在请求处理链中传递数据：
// - 中间件设置数据到 Context
// - Controller/Service 从 Context 读取数据
//
// 键的分类：
// - Token 相关：API Token 的属性（ID、密钥、配额、用户组等）
// - Channel 相关：渠道的属性（ID、类型、URL、密钥、模型映射等）
// - User 相关：用户的属性（ID、邮箱、配额、状态等）
// - 请求相关：请求的元数据（开始时间、是否流式等）
package constant

// ContextKey 上下文键类型
// 使用自定义类型避免与其他包的键冲突
type ContextKey string

// Token 计数相关键
const (
	// ContextKeyTokenCountMeta Token 计数元数据
	ContextKeyTokenCountMeta ContextKey = "token_count_meta"
	// ContextKeyPromptTokens 提示词 Token 数量
	ContextKeyPromptTokens ContextKey = "prompt_tokens"
	// ContextKeyEstimatedTokens 预估 Token 数量
	ContextKeyEstimatedTokens ContextKey = "estimated_tokens"
)

// 请求相关键
const (
	// ContextKeyOriginalModel 原始请求的模型名称
	// 在模型映射前保存原始名称
	ContextKeyOriginalModel ContextKey = "original_model"
	// ContextKeyRequestStartTime 请求开始时间
	// 用于计算请求处理耗时
	ContextKeyRequestStartTime ContextKey = "request_start_time"
)

// Token 相关键
const (
	// ContextKeyTokenUnlimited Token 是否有无限配额
	ContextKeyTokenUnlimited ContextKey = "token_unlimited_quota"
	// ContextKeyTokenKey Token 密钥
	ContextKeyTokenKey ContextKey = "token_key"
	// ContextKeyTokenId Token ID
	ContextKeyTokenId ContextKey = "token_id"
	// ContextKeyTokenGroup Token 用户组
	ContextKeyTokenGroup ContextKey = "token_group"
	// ContextKeyTokenSpecificChannelId Token 指定的渠道 ID
	ContextKeyTokenSpecificChannelId ContextKey = "specific_channel_id"
	// ContextKeyTokenModelLimitEnabled Token 模型限制是否启用
	ContextKeyTokenModelLimitEnabled ContextKey = "token_model_limit_enabled"
	// ContextKeyTokenModelLimit Token 模型限制列表
	ContextKeyTokenModelLimit ContextKey = "token_model_limit"
	// ContextKeyTokenCrossGroupRetry Token 是否允许跨用户组重试
	ContextKeyTokenCrossGroupRetry ContextKey = "token_cross_group_retry"
)

// Channel 相关键
const (
	// ContextKeyChannelId 渠道 ID
	ContextKeyChannelId ContextKey = "channel_id"
	// ContextKeyChannelName 渠道名称
	ContextKeyChannelName ContextKey = "channel_name"
	// ContextKeyChannelCreateTime 渠道创建时间
	ContextKeyChannelCreateTime ContextKey = "channel_create_time"
	// ContextKeyChannelBaseUrl 渠道基础 URL
	ContextKeyChannelBaseUrl ContextKey = "base_url"
	// ContextKeyChannelType 渠道类型
	ContextKeyChannelType ContextKey = "channel_type"
	// ContextKeyChannelSetting 渠道配置
	ContextKeyChannelSetting ContextKey = "channel_setting"
	// ContextKeyChannelOtherSetting 渠道其他配置
	ContextKeyChannelOtherSetting ContextKey = "channel_other_setting"
	// ContextKeyChannelParamOverride 渠道参数覆盖
	ContextKeyChannelParamOverride ContextKey = "param_override"
	// ContextKeyChannelHeaderOverride 渠道请求头覆盖
	ContextKeyChannelHeaderOverride ContextKey = "header_override"
	// ContextKeyChannelOrganization 渠道组织
	ContextKeyChannelOrganization ContextKey = "channel_organization"
	// ContextKeyChannelAutoBan 渠道是否自动禁用
	ContextKeyChannelAutoBan ContextKey = "auto_ban"
	// ContextKeyChannelModelMapping 渠道模型映射
	ContextKeyChannelModelMapping ContextKey = "model_mapping"
	// ContextKeyChannelStatusCodeMapping 渠道状态码映射
	ContextKeyChannelStatusCodeMapping ContextKey = "status_code_mapping"
	// ContextKeyChannelIsMultiKey 渠道是否为多密钥模式
	ContextKeyChannelIsMultiKey ContextKey = "channel_is_multi_key"
	// ContextKeyChannelMultiKeyIndex 多密钥模式的当前密钥索引
	ContextKeyChannelMultiKeyIndex ContextKey = "channel_multi_key_index"
	// ContextKeyChannelKey 渠道 API 密钥
	ContextKeyChannelKey ContextKey = "channel_key"
	// ContextKeyChannelCredentialMode 渠道凭证模式
	ContextKeyChannelCredentialMode ContextKey = "channel_credential_mode"
	// ContextKeyChannelAccountPool 渠道账号池
	ContextKeyChannelAccountPool ContextKey = "channel_account_pool"
	// ContextKeyChannelAccountId 渠道账号 ID
	ContextKeyChannelAccountId ContextKey = "channel_account_id"
	// ContextKeyChannelAccountName 渠道账号名称
	ContextKeyChannelAccountName ContextKey = "channel_account_name"
	// ContextKeyChannelAccountExcludedIds 渠道账号排除 ID 列表
	ContextKeyChannelAccountExcludedIds ContextKey = "channel_account_excluded_ids"
	// ContextKeyChannelAccountReserved 渠道账号保留标记
	ContextKeyChannelAccountReserved ContextKey = "channel_account_reserved"
	// ContextKeyChannelAccountRetryChannelId 渠道账号重试渠道 ID
	ContextKeyChannelAccountRetryChannelId ContextKey = "channel_account_retry_channel_id"

	// 账号池相关键
	// ContextKeyPoolGroupId 账号池分组 ID
	ContextKeyPoolGroupId ContextKey = "pool_group_id"
	// ContextKeyPoolGroupName 账号池分组名称
	ContextKeyPoolGroupName ContextKey = "pool_group_name"
	// ContextKeyPoolAccountId 账号池账号 ID
	ContextKeyPoolAccountId ContextKey = "pool_account_id"
	// ContextKeyPoolAccountName 账号池账号名称
	ContextKeyPoolAccountName ContextKey = "pool_account_name"
	// ContextKeyPoolAccountAuthType 账号池账号认证类型
	ContextKeyPoolAccountAuthType ContextKey = "pool_account_auth_type"
	// ContextKeyPoolAccountExcludedIds 账号池账号排除 ID 列表
	ContextKeyPoolAccountExcludedIds ContextKey = "pool_account_excluded_ids"
	// ContextKeyPoolAccountReserved 账号池账号保留标记
	ContextKeyPoolAccountReserved ContextKey = "pool_account_reserved"
	// ContextKeyPoolGroupReserved 账号池分组并发槽位保留标记
	ContextKeyPoolGroupReserved ContextKey = "pool_group_reserved"
)

// 自动分组相关键
const (
	// ContextKeyAutoGroup 自动分组名称
	ContextKeyAutoGroup ContextKey = "auto_group"
	// ContextKeyAutoGroupIndex 自动分组索引
	ContextKeyAutoGroupIndex ContextKey = "auto_group_index"
	// ContextKeyAutoGroupRetryIndex 自动分组重试索引
	ContextKeyAutoGroupRetryIndex ContextKey = "auto_group_retry_index"
)

// User 相关键
const (
	// ContextKeyUserId 用户 ID
	ContextKeyUserId ContextKey = "id"
	// ContextKeyUserSetting 用户配置
	ContextKeyUserSetting ContextKey = "user_setting"
	// ContextKeyUserQuota 用户配额
	ContextKeyUserQuota ContextKey = "user_quota"
	// ContextKeyUserStatus 用户状态
	ContextKeyUserStatus ContextKey = "user_status"
	// ContextKeyUserEmail 用户邮箱
	ContextKeyUserEmail ContextKey = "user_email"
	// ContextKeyUserGroup 用户组
	ContextKeyUserGroup ContextKey = "user_group"
	// ContextKeyUsingGroup 当前使用的用户组
	ContextKeyUsingGroup ContextKey = "group"
	// ContextKeyUserName 用户名
	ContextKeyUserName ContextKey = "username"
)

// 其他键
const (
	// ContextKeyLocalCountTokens 是否使用本地 Token 计数
	ContextKeyLocalCountTokens ContextKey = "local_count_tokens"
	// ContextKeySystemPromptOverride 系统提示词覆盖
	ContextKeySystemPromptOverride ContextKey = "system_prompt_override"
	// ContextKeyFileSourcesToCleanup 请求结束后需要清理的文件源列表
	ContextKeyFileSourcesToCleanup ContextKey = "file_sources_to_cleanup"
	// ContextKeyAdminRejectReason 管理员拒绝原因（仅管理员可见，不返回给用户）
	// 用于调试，可持久化到消费/错误日志中
	ContextKeyAdminRejectReason ContextKey = "admin_reject_reason"
	// ContextKeyLanguage 用户语言偏好（用于国际化）
	ContextKeyLanguage ContextKey = "language"
	// ContextKeyIsStream 是否为流式请求
	ContextKeyIsStream ContextKey = "is_stream"
)
