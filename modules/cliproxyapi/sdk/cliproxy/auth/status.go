// auth - status.go
// 定义认证条目的生命周期状态常量，用于标识认证信息在不同阶段的状态。
package auth

// Status 表示认证条目的生命周期状态类型。
type Status string

const (
	// StatusUnknown 表示认证状态无法确定。
	StatusUnknown Status = "unknown"
	// StatusActive 表示认证有效且可用于执行请求。
	StatusActive Status = "active"
	// StatusPending 表示认证正在等待外部操作（如多因素认证）。
	StatusPending Status = "pending"
	// StatusRefreshing 表示认证正在进行刷新流程。
	StatusRefreshing Status = "refreshing"
	// StatusError 表示认证因错误暂时不可用。
	StatusError Status = "error"
	// StatusDisabled 表示认证已被有意禁用。
	StatusDisabled Status = "disabled"
)
