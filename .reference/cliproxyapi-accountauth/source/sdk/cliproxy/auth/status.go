// 包 auth - status.go
// 该文件定义了认证条目的生命周期状态枚举。
// Status 类型表示 Auth 条目在其生命周期中的各种状态。
package auth

// Status 表示 Auth 条目的生命周期状态。
type Status string

const (
	// StatusUnknown 表示无法确定认证状态。
	StatusUnknown Status = "unknown"
	// StatusActive 表示认证有效且可执行。
	StatusActive Status = "active"
	// StatusPending 表示认证正在等待外部操作（如 MFA）。
	StatusPending Status = "pending"
	// StatusRefreshing 表示认证正在进行刷新流程。
	StatusRefreshing Status = "refreshing"
	// StatusError 表示认证因错误暂时不可用。
	StatusError Status = "error"
	// StatusDisabled 标记认证已被有意禁用。
	StatusDisabled Status = "disabled"
)
