// Package dto - user_settings.go
// 该文件定义了用户设置相关的数据传输对象
//
// 用户设置存储在 user 表的 setting 字段中（JSON 格式）
// 包含通知方式、额度预警、语言偏好等配置
package dto

// UserSetting 用户设置结构体
type UserSetting struct {
	NotifyType                       string  `json:"notify_type,omitempty"`                          // 通知类型（email/webhook/bark/gotify）
	QuotaWarningThreshold            float64 `json:"quota_warning_threshold,omitempty"`              // 额度预警阈值
	WebhookUrl                       string  `json:"webhook_url,omitempty"`                          // Webhook URL
	WebhookSecret                    string  `json:"webhook_secret,omitempty"`                       // Webhook 密钥
	NotificationEmail                string  `json:"notification_email,omitempty"`                   // 通知邮箱地址
	BarkUrl                          string  `json:"bark_url,omitempty"`                             // Bark 推送 URL
	GotifyUrl                        string  `json:"gotify_url,omitempty"`                           // Gotify 服务器地址
	GotifyToken                      string  `json:"gotify_token,omitempty"`                         // Gotify 应用令牌
	GotifyPriority                   int     `json:"gotify_priority"`                                // Gotify 消息优先级
	UpstreamModelUpdateNotifyEnabled bool    `json:"upstream_model_update_notify_enabled,omitempty"` // 是否接收上游模型更新定时检测通知（仅管理员）
	AcceptUnsetRatioModel            bool    `json:"accept_unset_model_ratio_model,omitempty"`       // 是否接受未设置价格的模型
	RecordIpLog                      bool    `json:"record_ip_log,omitempty"`                        // 是否记录请求和错误日志 IP
	SidebarModules                   string  `json:"sidebar_modules,omitempty"`                      // 左侧边栏模块配置（JSON 格式）
	BillingPreference                string  `json:"billing_preference,omitempty"`                   // 扣费策略（subscription/wallet）
	Language                         string  `json:"language,omitempty"`                             // 用户语言偏好（zh/en）
}

// 通知类型常量
var (
	NotifyTypeEmail   = "email"   // 邮件通知
	NotifyTypeWebhook = "webhook" // Webhook 通知
	NotifyTypeBark    = "bark"    // Bark 推送通知
	NotifyTypeGotify  = "gotify"  // Gotify 推送通知
)
