// Package constant - cache_key.go
// 该文件定义了 Redis 缓存键的格式常量
//
// 缓存键用于存储用户相关的数据：
// - 用户组信息
// - 用户配额
// - 用户启用状态
// - 用户名
//
// 缓存键格式说明：
// - 使用 fmt.Sprintf 格式化，%d 为用户 ID
// - 例如：user_group:12345
package constant

// 用户缓存键格式常量
const (
	// UserGroupKeyFmt 用户组缓存键格式
	// 存储用户所属的用户组（default, vip, svip 等）
	UserGroupKeyFmt = "user_group:%d"

	// UserQuotaKeyFmt 用户配额缓存键格式
	// 存储用户的剩余额度
	UserQuotaKeyFmt = "user_quota:%d"

	// UserEnabledKeyFmt 用户启用状态缓存键格式
	// 存储用户账户是否启用
	UserEnabledKeyFmt = "user_enabled:%d"

	// UserUsernameKeyFmt 用户名缓存键格式
	// 存储用户的显示名称
	UserUsernameKeyFmt = "user_name:%d"
)

// Token 字段名常量
// 用于 Token 缓存中的字段标识
const (
	// TokenFiledRemainQuota Token 剩余额度字段名
	TokenFiledRemainQuota = "RemainQuota"
	// TokenFieldGroup Token 用户组字段名
	TokenFieldGroup = "Group"
)
