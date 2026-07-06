// types.go 定义了账号认证模块的核心数据类型。
// 包括登录流程相关结构体（请求、结果、会话）、账号凭证、运行时视图、
// 配额状态、模型状态、提供者错误等。
package accountauth

import "time"

// 账号运行时状态常量
const (
	StatusUnknown    = "unknown"    // 未知状态
	StatusReady      = "ready"      // 就绪，可接受请求
	StatusRefreshing = "refreshing" // 正在刷新凭证
	StatusCooling    = "cooling"    // 冷却期，暂时不可用
	StatusDisabled   = "disabled"   // 已禁用
	StatusError      = "error"      // 出错，不可用
)

// LoginOptions 登录选项，控制登录行为和连接配置
type LoginOptions struct {
	NoBrowser    bool              `json:"no_browser"`    // 是否跳过自动打开浏览器（Device 流程使用）
	ProjectID    string            `json:"project_id"`    // 项目 ID（部分提供者需要）
	CallbackPort int               `json:"callback_port"` // OAuth 回调监听端口
	Proxy        string            `json:"proxy"`         // 代理地址
	Metadata     map[string]string `json:"metadata"`      // 附加元数据
}

// LoginStartRequest 登录启动请求
type LoginStartRequest struct {
	PoolGroupID int          `json:"pool_group_id"` // 账号池分组 ID
	Name        string       `json:"name"`          // 账号名称
	Options     LoginOptions `json:"options"`       // 登录选项
}

// LoginStartResult 登录启动结果，返回给前端用于引导用户完成认证
type LoginStartResult struct {
	SessionID       string            `json:"session_id"`                 // 登录会话 ID
	Provider        string            `json:"provider"`                   // 提供者名称
	Mode            string            `json:"mode"`                       // 登录模式（oauth/device）
	AuthorizeURL    string            `json:"authorize_url,omitempty"`    // OAuth 授权页面 URL
	VerificationURL string            `json:"verification_url,omitempty"` // Device 验证页面 URL
	UserCode        string            `json:"user_code,omitempty"`        // Device 用户验证码
	ExpiresAt       int64             `json:"expires_at,omitempty"`       // 会话过期时间（Unix 秒）
	PollInterval    int64             `json:"poll_interval,omitempty"`    // Device 轮询间隔（秒）
	Metadata        map[string]string `json:"metadata,omitempty"`         // 附加元数据
}

// LoginSessionStatus 登录会话状态类型
type LoginSessionStatus string

// 登录会话状态常量
const (
	LoginSessionPending   LoginSessionStatus = "pending"   // 等待用户操作
	LoginSessionCompleted LoginSessionStatus = "completed" // 认证成功
	LoginSessionFailed    LoginSessionStatus = "failed"    // 认证失败
	LoginSessionCancelled LoginSessionStatus = "cancelled" // 已取消
)

// LoginSessionView 登录会话的视图对象，用于向前端展示会话详情
type LoginSessionView struct {
	SessionID       string             `json:"session_id"`                 // 会话 ID
	AccountID       int                `json:"account_id,omitempty"`       // 关联的账号 ID（完成后填充）
	Provider        string             `json:"provider"`                   // 提供者名称
	Mode            string             `json:"mode"`                       // 登录模式
	Status          LoginSessionStatus `json:"status"`                     // 会话状态
	StatusMessage   string             `json:"status_message,omitempty"`   // 状态附加消息
	PoolGroupID     int                `json:"pool_group_id"`              // 账号池分组 ID
	Name            string             `json:"name,omitempty"`             // 账号名称
	AuthorizeURL    string             `json:"authorize_url,omitempty"`    // OAuth 授权 URL
	VerificationURL string             `json:"verification_url,omitempty"` // Device 验证 URL
	UserCode        string             `json:"user_code,omitempty"`        // Device 用户码
	ExpiresAt       int64              `json:"expires_at,omitempty"`       // 过期时间
	PollInterval    int64              `json:"poll_interval,omitempty"`    // 轮询间隔
	CreatedAt       int64              `json:"created_at"`                 // 创建时间
	UpdatedAt       int64              `json:"updated_at"`                 // 更新时间
	Account         *AccountCredential `json:"account,omitempty"`          // 完成后的账号凭证
}

// LoginCompleteRequest 登录完成请求
type LoginCompleteRequest struct {
	SessionID   string       `json:"session_id"`    // 登录会话 ID
	PoolGroupID int          `json:"pool_group_id"` // 账号池分组 ID
	Name        string       `json:"name"`          // 账号名称
	Input       string       `json:"input"`         // 用户输入（回调 URL 或授权码等）
	Options     LoginOptions `json:"options"`       // 登录选项
}

// LoginCompleteResult 登录完成结果
type LoginCompleteResult struct {
	AccountID int                 `json:"account_id"`        // 新建或关联的账号 ID
	Account   *AccountCredential  `json:"account,omitempty"` // 账号凭证
	Runtime   *AccountRuntimeView `json:"runtime,omitempty"` // 账号运行时视图
}

// AccountCredential 账号凭证对象，存储认证信息和元数据
type AccountCredential struct {
	Provider        string            `json:"provider"`                    // 提供者名称（如 codex）
	AuthType        string            `json:"auth_type"`                   // 认证类型（如 official_oauth）
	Label           string            `json:"label"`                       // 显示标签
	Credentials     string            `json:"credentials"`                 // 加密存储的凭证 JSON
	Summary         string            `json:"summary"`                     // 凭证摘要（用于界面展示）
	Metadata        map[string]any    `json:"metadata,omitempty"`          // 元数据（邮箱、账号 ID 等）
	Attributes      map[string]string `json:"attributes,omitempty"`        // 属性键值对
	ExpiresAt       time.Time         `json:"expires_at,omitempty"`        // 令牌过期时间
	LastRefreshedAt time.Time         `json:"last_refreshed_at,omitempty"` // 上次刷新时间
	NextRefreshAt   time.Time         `json:"next_refresh_at,omitempty"`   // 下次计划刷新时间
}

// AccountRuntimeView 账号运行时视图，汇总展示账号的完整运行状态
type AccountRuntimeView struct {
	Status            string                    `json:"status"`                        // 运行时状态
	StatusMessage     string                    `json:"status_message,omitempty"`      // 状态附加消息
	Unavailable       bool                      `json:"unavailable"`                   // 是否不可用
	Quota             QuotaState                `json:"quota"`                         // 配额状态
	ModelStates       map[string]*ModelState    `json:"model_states,omitempty"`        // 各模型的状态
	RecentRequests    []RecentRequestBucket     `json:"recent_requests,omitempty"`     // 最近请求统计
	LastError         *ProviderError            `json:"last_error,omitempty"`          // 最近一次错误
	LastRefreshedTime int64                     `json:"last_refreshed_time,omitempty"` // 上次刷新时间
	NextRefreshTime   int64                     `json:"next_refresh_time,omitempty"`   // 下次刷新时间
	NextRetryTime     int64                     `json:"next_retry_time,omitempty"`     // 下次重试时间
	SuccessCount      int64                     `json:"success_count"`                 // 成功请求总数
	FailedCount       int64                     `json:"failed_count"`                  // 失败请求总数
	Extra             map[string]map[string]any `json:"extra,omitempty"`               // 扩展字段
}

// QuotaState 配额状态，记录配额是否超限及相关信息
type QuotaState struct {
	Exceeded      bool      `json:"exceeded"`                  // 是否已超限
	Reason        string    `json:"reason,omitempty"`          // 超限原因
	NextRecoverAt time.Time `json:"next_recover_at,omitempty"` // 下次配额恢复时间
	BackoffLevel  int       `json:"backoff_level,omitempty"`   // 退避等级
}

// ModelState 单个模型的运行状态
type ModelState struct {
	Status         string         `json:"status"`                     // 模型状态
	StatusMessage  string         `json:"status_message,omitempty"`   // 状态附加消息
	Unavailable    bool           `json:"unavailable"`                // 是否不可用
	NextRetryAfter time.Time      `json:"next_retry_after,omitempty"` // 下次可重试时间
	LastError      *ProviderError `json:"last_error,omitempty"`       // 最近错误
	Quota          QuotaState     `json:"quota"`                      // 模型级别配额状态
	UpdatedAt      time.Time      `json:"updated_at,omitempty"`       // 最后更新时间
}

// ProviderError 上游提供者返回的错误信息
type ProviderError struct {
	Code       string `json:"code,omitempty"`        // 错误码
	Message    string `json:"message,omitempty"`     // 错误消息
	Retryable  bool   `json:"retryable"`             // 是否可重试
	HTTPStatus int    `json:"http_status,omitempty"` // HTTP 状态码
}

// RecentRequestBucket 最近请求统计的时间桶
type RecentRequestBucket struct {
	Time    string `json:"time"`    // 时间范围标签（如 "14:20-14:30"）
	Success int64  `json:"success"` // 成功请求数
	Failed  int64  `json:"failed"`  // 失败请求数
}

// ProviderInfo 提供者信息，描述提供者支持的认证方式
type ProviderInfo struct {
	Name            string `json:"name"`             // 提供者标识
	DisplayName     string `json:"display_name"`     // 显示名称
	SupportsOAuth   bool   `json:"supports_oauth"`   // 是否支持 OAuth
	SupportsDevice  bool   `json:"supports_device"`  // 是否支持 Device 流程
	SupportsRefresh bool   `json:"supports_refresh"` // 是否支持 Token 刷新
}
