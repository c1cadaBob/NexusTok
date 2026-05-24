// Package dto - notify.go
// 该文件定义了系统通知相关的数据传输对象
//
// 主要结构体：
// - Notify：系统通知结构体（类型、标题、内容、值列表）
//
// 通知类型：
// - NotifyTypeQuotaExceed：配额超限通知
// - NotifyTypeChannelUpdate：渠道更新通知
// - NotifyTypeChannelTest：渠道测试通知
//
// 内容模板：
// - ContentValueParam ({{value}})：内容中的值占位符，运行时替换为实际值
package dto

// Notify 系统通知结构体
// Type：通知类型（quota_exceed/channel_update/channel_test 等）
// Title：通知标题
// Content：通知内容（可包含 {{value}} 占位符）
// Values：值列表（用于替换内容中的占位符）
type Notify struct {
	Type    string        `json:"type"`
	Title   string        `json:"title"`
	Content string        `json:"content"`
	Values  []interface{} `json:"values"`
}

// ContentValueParam 内容值占位符
// 在通知内容模板中使用，运行时会被替换为 Values 数组中的实际值
const ContentValueParam = "{{value}}"

// 通知类型常量
const (
	NotifyTypeQuotaExceed   = "quota_exceed"   // 配额超限通知
	NotifyTypeChannelUpdate = "channel_update"  // 渠道更新通知
	NotifyTypeChannelTest   = "channel_test"    // 渠道测试通知
)

// NewNotify 创建新的通知实例
// t：通知类型
// title：通知标题
// content：通知内容（可包含占位符）
// values：值列表（用于替换占位符）
func NewNotify(t string, title string, content string, values []interface{}) Notify {
	return Notify{
		Type:    t,
		Title:   title,
		Content: content,
		Values:  values,
	}
}
